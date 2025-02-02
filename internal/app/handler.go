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
				go func() { session.StopChannel <- struct{}{} }()
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

	select {
	case <-session.StopChannel:
	default:
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
	go handleResponse(botState, inMsg, session, model, client)
}

func wrapMessage(responding bool, content string, session *Session) string {
	respondingBanner := fmt.Sprintf("💭 *%s*\n\n", session.Model)
	finishedBanner := fmt.Sprintf("🤗 *%s*\n\n", session.Model)
	if responding {
		return respondingBanner + content
	}
	return finishedBanner + content
}

func handleResponse(botState *State, inMsg *botapi.Message, session *Session,
	model *util.Model, client *openai.Client) {
	stopChan := session.StopChannel
	responseContent := ""
	currentContent := ""

	defer func() {
		session.ResponseChannel <- responseContent
	}()

	outMsg, err := util.SendMessageMarkdown(inMsg.Chat.ID,
		wrapMessage(true, currentContent, session),
		botState.Bot, botState.Config.UseTelegramify)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	var openaiMsgs []openai.ChatCompletionMessage
	systemPromptRole := openai.ChatMessageRoleSystem
	if !model.SystemPrompt {
		systemPromptRole = openai.ChatMessageRoleUser
	}
	openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessage{
		Role:    systemPromptRole,
		Content: util.SystemPromptString,
	})
	for _, record := range session.ChatRecords {
		msg := record.ToOpenAIChatMessage()
		if msg.Role == openai.ChatMessageRoleAssistant && msg.Content == "" {
			continue
		}
		openaiMsgs = append(openaiMsgs, msg)
	}

	req := openai.ChatCompletionRequest{
		Messages:            openaiMsgs,
		Model:               model.Name,
		MaxCompletionTokens: botState.Config.MaxTokensPerResponse,
		Stream:              model.Stream,
	}

	if !model.Stream {
		resp, err := client.CreateChatCompletion(context.Background(), req)
		if err != nil {
			slog.Error(err.Error())
			return
		}
		responseContent = resp.Choices[0].Message.Content

		if len(responseContent) > util.MessageCharacterLimit {
			leftContent := responseContent[:util.MessageCharacterLimit]
			rightContent := responseContent[util.MessageCharacterLimit:]
			util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID,
				wrapMessage(false, leftContent, session),
				botState.Bot, botState.Config.UseTelegramify)

			for len(rightContent) > 0 {
				var chunk string
				if len(rightContent) > util.MessageCharacterLimit {
					chunk = rightContent[:util.MessageCharacterLimit]
					rightContent = rightContent[util.MessageCharacterLimit:]
				} else {
					chunk = rightContent
					rightContent = ""
				}
				outMsg, err = util.SendMessageMarkdown(inMsg.Chat.ID,
					wrapMessage(false, chunk, session),
					botState.Bot, botState.Config.UseTelegramify)
				if err != nil {
					slog.Error(err.Error())
					return
				}
			}
		} else {
			util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID,
				wrapMessage(false, responseContent, session),
				botState.Bot, botState.Config.UseTelegramify)
		}

		return
	}

	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer stream.Close()

	lastEditLen := len(responseContent)
	for {
		select {
		case <-stopChan:
			return
		default:
			response, err := stream.Recv()

			if errors.Is(err, io.EOF) {
				util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID,
					wrapMessage(false, currentContent, session),
					botState.Bot, botState.Config.UseTelegramify)
				return
			}

			if err != nil {
				slog.Error(err.Error())
				return
			}

			if len(response.Choices) == 0 {
				slog.Warn("Empty response")
				continue
			}

			deltaContent := response.Choices[0].Delta.Content
			responseContent += deltaContent
			currentContent += deltaContent
			currentEditLen := len(responseContent)
			if currentEditLen-lastEditLen < 16 {
				continue
			}

			select {
			case <-botState.EditThrottler:
				lastEditLen = currentEditLen

				if len(currentContent) > util.MessageCharacterLimit {
					leftContent := currentContent[:util.MessageCharacterLimit]
					rightContent := currentContent[util.MessageCharacterLimit:]
					currentContent = rightContent
					util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID,
						wrapMessage(false, leftContent, session),
						botState.Bot, botState.Config.UseTelegramify)
					outMsg, err = util.SendMessageMarkdown(inMsg.Chat.ID,
						wrapMessage(true, rightContent, session),
						botState.Bot, botState.Config.UseTelegramify)
					if err != nil {
						slog.Error(err.Error())
						return
					}
				} else {
					util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID,
						wrapMessage(true, currentContent, session),
						botState.Bot, botState.Config.UseTelegramify)
				}
			default:
			}
		}
	}
}
