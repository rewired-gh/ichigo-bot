package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/rewired-gh/ichigo-bot/internal/util"
	"github.com/sashabaranov/go-openai"

	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StartBotService initializes the bot state and update loop.
func StartBotService(config *util.Config) {
	botState := NewState(config)
	bot, err := botapi.NewBotAPI(config.Token)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	botState.Bot = bot
	botState.Bot.Debug = config.Debug
	u := botapi.NewUpdate(0)
	u.Timeout = 60
	updates := botState.Bot.GetUpdatesChan(u)
	defer botState.Bot.StopReceivingUpdates()

	for update := range updates {
		processUpdate(botState, update)
	}
}

// processUpdate processes a single update from Telegram.
func processUpdate(botState *State, update botapi.Update) {
	inMsg := update.Message
	if inMsg == nil {
		slog.Warn("Nil message")
		return
	}

	// Get user session via user id, fall back to chat id.
	session, exists := botState.SessionMap[inMsg.From.ID]
	if !exists {
		chat := update.FromChat()
		session, exists = botState.SessionMap[chat.ID]
		if !exists {
			slog.Warn("Session not found", "userID", inMsg.From.ID, "chatID", chat.ID)
			return
		}
	}

	if inMsg.IsCommand() {
		handleCommand(botState, inMsg, session)
	} else if inMsg.Chat.IsPrivate() {
		handleChatAction(botState, inMsg, session)
	}
}

// handleChatAction sends a user message to the AI and invokes response handling.
func handleChatAction(botState *State, inMsg *botapi.Message, session *Session) {
	// Drain any pending response.
	select {
	case content := <-session.ResponseChannel:
		session.ChatRecords = append(session.ChatRecords, ChatRecord{Role: RoleBot, Content: content})
		session.State = StateIdle
	default:
	}

	if session.State == StateResponding {
		slog.Warn("Ignoring new message while responding", "userID", inMsg.From.ID)
		return
	}

	// Clear stale stop signal.
	select {
	case <-session.StopChannel:
	default:
	}

	// Append this user message to the session.
	session.ChatRecords = append(session.ChatRecords, ChatRecord{Role: RoleUser, Content: inMsg.Text})
	session.State = StateResponding

	// Handle the response asynchronously.
	go handleResponse(botState, inMsg, session)
}

// handleResponse builds the OpenAI request and processes responses (streaming or non-streaming).
func handleResponse(botState *State, inMsg *botapi.Message, session *Session) {
	model, ok := botState.CachedModelMap[session.Model]
	if !ok {
		slog.Error("Model not found", "model", session.Model)
		return
	}
	client, ok := botState.CachedProviderMap[model.Provider]
	if !ok {
		slog.Error("Provider not found", "provider", model.Provider)
		return
	}

	// Retain recent records if needed.
	upperLimit := botState.Config.MaxChatRecordsPerUser - 2
	if len(session.ChatRecords) > upperLimit {
		session.ChatRecords = session.ChatRecords[len(session.ChatRecords)-upperLimit:]
	}

	// Build request messages.
	var openaiMsgs []openai.ChatCompletionMessage
	systemRole := openai.ChatMessageRoleSystem
	if !model.SystemPrompt {
		systemRole = openai.ChatMessageRoleUser
	}
	openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessage{
		Role:    systemRole,
		Content: util.SystemPromptString,
	})
	for _, record := range session.ChatRecords {
		if record.Role == RoleBot && record.Content == "" {
			continue
		}
		openaiMsgs = append(openaiMsgs, record.ToOpenAIChatMessage())
	}

	req := openai.ChatCompletionRequest{
		Messages:            openaiMsgs,
		Model:               model.Name,
		MaxCompletionTokens: botState.Config.MaxTokensPerResponse,
		Stream:              model.Stream,
	}

	if !model.Stream {
		processNonStreamingResponse(botState, inMsg, session, client, req)
	} else {
		processStreamingResponse(botState, inMsg, session, client, req)
	}
}

func processNonStreamingResponse(botState *State, inMsg *botapi.Message, session *Session, client *openai.Client, req openai.ChatCompletionRequest) {
	resp, err := client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	responseContent := resp.Choices[0].Message.Content
	sendLongMessage(botState, inMsg, session, responseContent, false)
}

func processStreamingResponse(botState *State, inMsg *botapi.Message, session *Session, client *openai.Client, req openai.ChatCompletionRequest) {
	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	defer stream.Close()

	var responseContent, currentContent string
	outMsg, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(true, "", session), botState.Bot, botState.Config.UseTelegramify)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	lastEditLen := 0

	for {
		select {
		case <-session.StopChannel:
			return
		default:
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(false, currentContent, session), botState.Bot, botState.Config.UseTelegramify)
				return
			}
			if err != nil {
				slog.Error(err.Error())
				return
			}
			if len(resp.Choices) == 0 {
				slog.Warn("Empty response")
				continue
			}
			deltaContent := resp.Choices[0].Delta.Content
			responseContent += deltaContent
			currentContent += deltaContent
			if len(responseContent)-lastEditLen < 16 {
				continue
			}

			select {
			case <-botState.EditThrottler:
				lastEditLen = len(responseContent)
				if len(currentContent) > util.MessageCharacterLimit {
					chunk := currentContent[:util.MessageCharacterLimit]
					currentContent = currentContent[util.MessageCharacterLimit:]
					util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(false, chunk, session), botState.Bot, botState.Config.UseTelegramify)
					outMsg, err = util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(true, currentContent, session), botState.Bot, botState.Config.UseTelegramify)
					if err != nil {
						slog.Error(err.Error())
						return
					}
				} else {
					util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(true, currentContent, session), botState.Bot, botState.Config.UseTelegramify)
				}
			default:
			}
		}
	}
}

// wrapMessage adds a header banner to show the model and status.
func wrapMessage(isResponding bool, content string, session *Session) string {
	var banner string
	if isResponding {
		banner = fmt.Sprintf("💭 *%s*\n\n", session.Model)
	} else {
		banner = fmt.Sprintf("🤗 *%s*\n\n", session.Model)
	}
	return banner + content
}

// sendLongMessage breaks content into chunks if it exceeds the Telegram message limit.
func sendLongMessage(botState *State, inMsg *botapi.Message, session *Session, content string, isStreaming bool) {
	if len(content) > util.MessageCharacterLimit {
		firstChunk := content[:util.MessageCharacterLimit]
		remaining := content[util.MessageCharacterLimit:]
		if !isStreaming {
			util.EditMessageMarkdown(inMsg.Chat.ID, inMsg.MessageID, wrapMessage(false, firstChunk, session), botState.Bot, botState.Config.UseTelegramify)
		} else {
			_, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(false, firstChunk, session), botState.Bot, botState.Config.UseTelegramify)
			if err != nil {
				slog.Error(err.Error())
				return
			}
		}
		for len(remaining) > 0 {
			var chunk string
			if len(remaining) > util.MessageCharacterLimit {
				chunk = remaining[:util.MessageCharacterLimit]
				remaining = remaining[util.MessageCharacterLimit:]
			} else {
				chunk = remaining
				remaining = ""
			}
			_, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(false, chunk, session), botState.Bot, botState.Config.UseTelegramify)
			if err != nil {
				slog.Error(err.Error())
				return
			}
		}
	} else {
		util.EditMessageMarkdown(inMsg.Chat.ID, inMsg.MessageID, wrapMessage(false, content, session), botState.Bot, botState.Config.UseTelegramify)
	}
}
