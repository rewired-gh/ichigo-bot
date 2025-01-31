package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/pelletier/go-toml/v2"
	"github.com/rewired-gh/ichigo-bot/internal/util"
	"github.com/sashabaranov/go-openai"

	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func StartBotService(config *util.Config) {
	botState := NewState(config)
	bot, err := botapi.NewBotAPI(config.Token)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	botState.Bot = bot
	botState.Bot.Debug = config.Debug == true
	u := botapi.NewUpdate(0)
	u.Timeout = 60
	updates := botState.Bot.GetUpdatesChan(u)
	defer botState.Bot.StopReceivingUpdates()

	for update := range updates {
		session, exists := botState.SessionMap[update.Message.From.ID]
		if !exists {
			slog.Warn("Session not found", "userID", update.Message.From.ID)
			continue
		}

		inMsg := update.Message
		if inMsg == nil {
			slog.Warn("Nil message")
			continue
		}

		if !inMsg.IsCommand() {
			select {
			case content := <-session.ResponseChannel:
				session.ChatRecords = append(session.ChatRecords, ChatRecord{
					Role:    RoleBot,
					Content: content,
				})
				session.State = StateIdle
			default:
			}
			if session.State == StateResponding {
				slog.Warn("Ignoring message while responding", "userID", inMsg.From.ID)
				continue
			}

			handleChatAction(botState, inMsg, session)
		}

		switch inMsg.Command() {
		case "new":
			session.ChatRecords = make([]ChatRecord, 16)
			util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
		case "set":
			model := inMsg.CommandArguments()

			modelInfo, exists := botState.CachedModelMap[model]
			if !exists {
				slog.Warn("Model not found in CachedModelMap", "model", model)
				continue
			}

			session.Model = model
			util.SendMessageQuick(
				inMsg.Chat.ID,
				fmt.Sprintf("Model set to %s (%s) by %s", modelInfo.Name, model, modelInfo.Provider),
				botState.Bot,
			)
		case "undo":
			if len(session.ChatRecords) > 0 && session.ChatRecords[len(session.ChatRecords)-1].Role == RoleBot {
				session.ChatRecords = session.ChatRecords[:len(session.ChatRecords)-1]
			}
			if len(session.ChatRecords) > 0 && session.ChatRecords[len(session.ChatRecords)-1].Role == RoleUser {
				session.ChatRecords = session.ChatRecords[:len(session.ChatRecords)-1]
			}
			util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
		case "stop":
			session.StopChannel <- struct{}{}
		}

		if slices.Contains(botState.Config.Admins, inMsg.From.ID) {
			switch inMsg.Command() {
			case "get_config":
				configString, err := toml.Marshal(botState.Config)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				util.SendMessageQuick(inMsg.Chat.ID, string(configString), botState.Bot)
			case "set_config":
				configString := inMsg.CommandArguments()
				var config util.Config
				err := toml.Unmarshal([]byte(configString), &config)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				botState.Config = &config
				dataDir := util.GetDataDir()
				configPath := filepath.Join(dataDir, util.ConfigName+"."+util.ConfigType)
				err = os.WriteFile(configPath, []byte(configString), 0644)
				if err != nil {
					slog.Error(err.Error())
					continue
				}
				util.SendMessageQuick(inMsg.Chat.ID, "Roger. Please restart the daemon manually.", botState.Bot)
			case "clear":
				for _, session := range botState.SessionMap {
					session.ChatRecords = make([]ChatRecord, 16)
				}
				util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
			}
		}
	}
}

func handleChatAction(botState *State, inMsg *botapi.Message, session *Session) {
	model, modelExists := botState.CachedModelMap[session.Model]
	if !modelExists {
		slog.Error("Model not found in CachedModelMap", "model", session.Model)
		return
	}
	client, clientExists := botState.CachedProviderMap[model.Provider]
	if !clientExists {
		slog.Error("Provider not found in CachedProviderMap", "provider", model.Provider)
		return
	}

	session.ChatRecords = append(session.ChatRecords, ChatRecord{
		Role:    RoleUser,
		Content: inMsg.Text,
	})
	session.State = StateResponding
	go handleStreamingResponse(botState, inMsg, session, model, client)
}

func wrapMessage(responding bool, content string, session *Session) string {
	respondingBanner := fmt.Sprintf("💭 *%s*\n", session.Model)
	finishedBanner := fmt.Sprintf("🤗 *%s*\n", session.Model)
	if responding {
		return respondingBanner + content
	}
	return finishedBanner + content
}

func handleStreamingResponse(botState *State, inMsg *botapi.Message, session *Session,
	model *util.Model, client *openai.Client) {
	stopChan := session.StopChannel
	responseContent := ""

	outMsg, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(true, responseContent, session), botState.Bot)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	openaiMsgs := make([]openai.ChatCompletionMessage, len(session.ChatRecords))
	for i, record := range session.ChatRecords {
		openaiMsgs[i] = record.ToOpenAIChatMessage()
	}

	req := openai.ChatCompletionRequest{
		Messages:  openaiMsgs,
		Model:     model.Name,
		MaxTokens: botState.Config.MaxTokensPerResponse,
		Stream:    true,
	}
	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer stream.Close()
	defer func() {
		session.ResponseChannel <- responseContent
	}()

	for {
		select {
		case <-stopChan:
			return
		default:
			response, err := stream.Recv()

			if errors.Is(err, io.EOF) {
				util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(false, responseContent, session), botState.Bot)
				return
			}

			if err != nil {
				slog.Error(err.Error())
				return
			}

			responseContent += response.Choices[0].Delta.Content
			util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(true, responseContent, session), botState.Bot)
		}
	}
}
