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
	cmd := inMsg.Command()
	slog.Info("processing command",
		"command", cmd,
		"user_id", inMsg.From.ID)

	switch cmd {
	case "chat":
		handleChatAction(botState, inMsg, session)
	case "new":
		session.ChatRecords = make([]ChatRecord, 0, 16)
		tryStoppingResponse(session)
		select {
		case <-session.ResponseChannel:
		default:
		}
		util.SendMessageQuick(inMsg.Chat.ID, "New conversation started.", botState.Bot)
	case "set":
		modelAlias := inMsg.CommandArguments()
		model, exists := botState.CachedModelMap[modelAlias]
		if !exists {
			slog.Warn("model not found", "model", modelAlias)
			util.SendMessageQuick(inMsg.Chat.ID, "Model not found.", botState.Bot)
			return
		}
		if !session.AvailableModels.ContainsAny(modelAlias) {
			slog.Warn("model not available", "model", modelAlias, "user_id", inMsg.From.ID, "chat_id", inMsg.Chat.ID)
			util.SendMessageQuick(inMsg.Chat.ID, "Model not available.", botState.Bot)
			return
		}
		session.Model = modelAlias
		util.SendMessageQuick(inMsg.Chat.ID, fmt.Sprintf("Current model: %s (%s) by %s", model.Name, modelAlias, model.Provider), botState.Bot)
	case "list":
		modelList := "Available models:\n"
		for alias, model := range botState.CachedModelMap {
			if !session.AvailableModels.ContainsAny(alias) {
				continue
			}
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
		util.SendMessageQuick(inMsg.Chat.ID, "Last round of conversation undone.", botState.Bot)
	case "stop":
		tryStoppingResponse(session)
		util.SendMessageQuick(inMsg.Chat.ID, "Tried stopping the last response.", botState.Bot)
	default:
		if isAdmin(botState.Config.Admins, inMsg.From.ID) {
			handleAdminCommand(botState, inMsg)
		}
	}
}

// handleAdminCommand executes admin-only commands.
func handleAdminCommand(botState *State, inMsg *botapi.Message) {
	cmd := inMsg.Command()
	slog.Info("processing admin command",
		"command", cmd,
		"user_id", inMsg.From.ID)

	switch cmd {
	case "get_config":
		configString, err := toml.Marshal(botState.Config)
		if err != nil {
			slog.Error("failed to marshal config", "error", err)
			util.SendMessageQuick(inMsg.Chat.ID, "Failed to retrieve configuration.", botState.Bot)
			return
		}
		util.SendMessageQuick(inMsg.Chat.ID, string(configString), botState.Bot)
	case "set_config":
		configString := inMsg.CommandArguments()
		var config util.Config
		err := toml.Unmarshal([]byte(configString), &config)
		if err != nil {
			slog.Error(err.Error())
			util.SendMessageQuick(inMsg.Chat.ID, "Failed to update configuration.", botState.Bot)
			return
		}
		botState.Config = &config
		dataDir := util.GetDataDir()
		configPath := filepath.Join(dataDir, util.ConfigName+"."+util.ConfigType)
		err = os.WriteFile(configPath, []byte(configString), 0644)
		if err != nil {
			slog.Error(err.Error())
			util.SendMessageQuick(inMsg.Chat.ID, "Failed to update configuration.", botState.Bot)
			return
		}
		util.SendMessageQuick(inMsg.Chat.ID, "Configuration updated. The bot will now shut down or restart.", botState.Bot)
		os.Exit(0)
	case "clear":
		for _, sess := range botState.SessionMap {
			sess.ChatRecords = make([]ChatRecord, 0, 16)
		}
		util.SendMessageQuick(inMsg.Chat.ID, "All chat sessions have been reset.", botState.Bot)
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
