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
		inMsg := update.Message
		inChat := update.FromChat()
		if inMsg == nil {
			slog.Warn("Nil message")
			continue
		}

		session, exists := botState.SessionMap[inMsg.From.ID]
		if !exists {
			session, exists = botState.SessionMap[inChat.ID]
			if !exists {
				slog.Warn("Session not found", "userID", inMsg.From.ID, "chatID", inChat.ID)
				continue
			}
		}

		if !inMsg.IsCommand() && inChat.IsPrivate() {
			handleChatAction(botState, inMsg, session)
			continue
		}

		switch inMsg.Command() {
		case "chat":
			handleChatAction(botState, inMsg, session)
		case "new":
			session.ChatRecords = make([]ChatRecord, 0, 16)
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
				fmt.Sprintf("Current Model: %s (%s) by %s", modelInfo.Name, model, modelInfo.Provider),
				botState.Bot,
			)
		case "list":
			var modelList string
			for alias, model := range botState.CachedModelMap {
				modelList += fmt.Sprintf("%s: %s by %s\n", alias, model.Name, model.Provider)
			}
			util.SendMessageQuick(inMsg.Chat.ID, modelList, botState.Bot)
		case "undo":
			if len(session.ChatRecords) > 0 && session.ChatRecords[len(session.ChatRecords)-1].Role == RoleBot {
				session.ChatRecords = session.ChatRecords[:len(session.ChatRecords)-1]
			}
			if len(session.ChatRecords) > 0 && session.ChatRecords[len(session.ChatRecords)-1].Role == RoleUser {
				session.ChatRecords = session.ChatRecords[:len(session.ChatRecords)-1]
			}
			util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
		case "stop":
			if session.State == StateResponding {
				session.StopChannel <- struct{}{}
				session.State = StateIdle
			}
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
					session.ChatRecords = make([]ChatRecord, 0, 16)
				}
				util.SendMessageQuick(inMsg.Chat.ID, "Roger.", botState.Bot)
			}
		}
	}
}

func handleChatAction(botState *State, inMsg *botapi.Message, session *Session) {
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
		return
	}

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

	upperLimit := botState.Config.MaxChatRecordsPerUser - 2
	if len(session.ChatRecords) > upperLimit {
		delta := len(session.ChatRecords) - upperLimit
		session.ChatRecords = session.ChatRecords[delta:]
	}

	session.ChatRecords = append(session.ChatRecords, ChatRecord{
		Role:    RoleUser,
		Content: inMsg.Text,
	})
	session.State = StateResponding
	go handleStreamingResponse(botState, inMsg, session, model, client)
}

func wrapMessage(responding bool, content string, session *Session) string {
	respondingBanner := fmt.Sprintf("💭 *%s*\n\n", session.Model)
	finishedBanner := fmt.Sprintf("🤗 *%s*\n\n", session.Model)
	if responding {
		return respondingBanner + content
	}
	return finishedBanner + content
}

func handleStreamingResponse(botState *State, inMsg *botapi.Message, session *Session,
	model *util.Model, client *openai.Client) {
	stopChan := session.StopChannel
	responseContent := ""

	defer func() {
		session.ResponseChannel <- responseContent
	}()

	outMsg, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(true, responseContent, session),
		botState.Bot, botState.Config.UseTelegramify)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	openaiMsgs := make([]openai.ChatCompletionMessage, len(session.ChatRecords)+1)
	openaiMsgs[0] = openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: util.SystemPromptString,
	}
	for i, record := range session.ChatRecords {
		openaiMsgs[i+1] = record.ToOpenAIChatMessage()
	}

	req := openai.ChatCompletionRequest{
		Messages:            openaiMsgs,
		Model:               model.Name,
		MaxCompletionTokens: botState.Config.MaxTokensPerResponse,
		Stream:              true,
	}
	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer stream.Close()

	for {
		select {
		case <-stopChan:
			return
		default:
			response, err := stream.Recv()

			if errors.Is(err, io.EOF) {
				util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(false, responseContent, session),
					botState.Bot, botState.Config.UseTelegramify)
				return
			}

			if err != nil {
				slog.Error(err.Error())
				return
			}

			responseContent += response.Choices[0].Delta.Content
			select {
			case <-botState.EditThrottler.ReadyChannel:
				util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(true, responseContent, session),
					botState.Bot, botState.Config.UseTelegramify)
				botState.EditThrottler.ResetChannel <- struct{}{}
			default:
			}

		}
	}
}
