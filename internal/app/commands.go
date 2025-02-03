package app

import (
	"fmt"
	"os"
	"path/filepath"

	"log/slog"

	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pelletier/go-toml/v2"
	"github.com/rewired-gh/ichigo-bot/internal/util"
)

// handleCommand interprets incoming bot commands.
func handleCommand(botState *State, inMsg *botapi.Message, session *Session) {
	switch inMsg.Command() {
	case "chat":
		handleChatAction(botState, inMsg, session)
	case "new":
		session.ChatRecords = make([]ChatRecord, 0, 16)
		util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
	case "set":
		modelAlias := inMsg.CommandArguments()
		model, exists := botState.CachedModelMap[modelAlias]
		if !exists {
			slog.Warn("Model not found", "model", modelAlias)
			return
		}
		session.Model = modelAlias
		util.SendMessageQuick(inMsg.Chat.ID, fmt.Sprintf("Current Model: %s (%s) by %s", model.Name, modelAlias, model.Provider), botState.Bot)
	case "list":
		var modelList string
		for alias, model := range botState.CachedModelMap {
			modelList += fmt.Sprintf("%s: %s by %s\n", alias, model.Name, model.Provider)
		}
		util.SendMessageQuick(inMsg.Chat.ID, modelList, botState.Bot)
	case "undo":
		if len(session.ChatRecords) > 0 {
			if session.ChatRecords[len(session.ChatRecords)-1].Role == RoleBot {
				session.ChatRecords = session.ChatRecords[:len(session.ChatRecords)-1]
			}
			if len(session.ChatRecords) > 0 && session.ChatRecords[len(session.ChatRecords)-1].Role == RoleUser {
				session.ChatRecords = session.ChatRecords[:len(session.ChatRecords)-1]
			}
		}
		util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
	case "stop":
		if session.State == StateResponding {
			go func() { session.StopChannel <- struct{}{} }()
			session.State = StateIdle
		}
	default:
		if isAdmin(botState.Config.Admins, inMsg.From.ID) {
			handleAdminCommand(botState, inMsg)
		}
	}
}

// handleAdminCommand executes admin-only commands.
func handleAdminCommand(botState *State, inMsg *botapi.Message) {
	switch inMsg.Command() {
	case "get_config":
		configString, err := toml.Marshal(botState.Config)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		util.SendMessageQuick(inMsg.Chat.ID, string(configString), botState.Bot)
	case "set_config":
		configString := inMsg.CommandArguments()
		var config util.Config
		err := toml.Unmarshal([]byte(configString), &config)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		botState.Config = &config
		dataDir := util.GetDataDir()
		configPath := filepath.Join(dataDir, util.ConfigName+"."+util.ConfigType)
		err = os.WriteFile(configPath, []byte(configString), 0644)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		util.SendMessageQuick(inMsg.Chat.ID, "Roger. This bot will shutdown soon.", botState.Bot)
		os.Exit(0)
	case "clear":
		for _, sess := range botState.SessionMap {
			sess.ChatRecords = make([]ChatRecord, 0, 16)
		}
		util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
	}
}

func isAdmin(admins []int64, userID int64) bool {
	for _, a := range admins {
		if a == userID {
			return true
		}
	}
	return false
}
