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
	slog.Info("initializing bot service")
	botState := New(config)
	bot, err := botapi.NewBotAPI(config.Token)
	if err != nil {
		slog.Error("failed to create bot API client", "error", err)
		return
	}

	botState.Bot = bot
	botState.Bot.Debug = config.Debug
	slog.Info("bot API client initialized", "username", bot.Self.UserName, "debug_mode", config.Debug)
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
		slog.Debug("skipping update with nil message", "update_id", update.UpdateID)
		return
	}

	slog.Debug("processing update",
		"update_id", update.UpdateID,
		"user_id", inMsg.From.ID,
		"chat_id", inMsg.Chat.ID,
		"is_command", inMsg.IsCommand())

	// Get user session via user id, fall back to chat id.
	session, exists := botState.SessionMap[inMsg.From.ID]
	if !exists {
		chat := update.FromChat()
		session, exists = botState.SessionMap[chat.ID]
		if !exists {
			slog.Warn("unauthorized access attempt",
				"user_id", inMsg.From.ID,
				"chat_id", chat.ID,
				"username", inMsg.From.UserName)
			return
		}
	}

	if util.IsCommand(inMsg) {
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
		AppendChatRecord(botState.DB, session.ID, int(RoleBot), content)
	default:
	}

	if session.State == StateResponding {
		slog.Warn("ignoring new message while responding", "userID", inMsg.From.ID)
		util.SendMessageQuick(inMsg.Chat.ID, "Last response has not completed yet.", botState.Bot)
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
	AppendChatRecord(botState.DB, session.ID, int(RoleUser), inMsg.Text)

	// Handle the response asynchronously.
	go handleResponse(botState, inMsg, session)
}

// handleResponse builds the OpenAI request and processes responses (streaming or non-streaming).
func handleResponse(botState *State, inMsg *botapi.Message, session *Session) {
	slog.Debug("preparing AI response",
		"user_id", inMsg.From.ID,
		"model", session.Model,
		"records", len(session.ChatRecords))

	model, ok := botState.CachedModelMap[session.Model]
	if !ok {
		slog.Error("model not configured", "model", session.Model)
		util.SendMessageQuick(inMsg.Chat.ID, "Model not configured.", botState.Bot)
		return
	}
	client, ok := botState.CachedProviderMap[model.Provider]
	if !ok {
		slog.Error("provider not found", "provider", model.Provider)
		util.SendMessageQuick(inMsg.Chat.ID, "Provider not found.", botState.Bot)
		return
	}

	// Retain recent records if needed.
	upperLimit := botState.Config.MaxChatRecordsPerUser - 2
	if len(session.ChatRecords) > upperLimit {
		session.ChatRecords = session.ChatRecords[len(session.ChatRecords)-upperLimit:]
		TrimOldChatRecords(botState.DB, session.ID, upperLimit)
	}

	// Retain the system prompt
	var systemPromptName string
	if session.Prompt == "" {
		systemPromptName = botState.Config.DefaultSystemPrompt
	} else {
		systemPromptName = session.Prompt
	}
	var systemPrompt string
	if systemPromptName != "" {
		systemPrompt, ok = botState.CachedPromptMap[systemPromptName]
		if !ok {
			slog.Error("system prompt not found", "prompt", systemPromptName)
			return
		}
	} else {
		systemPrompt = util.FallbackSystemPromptString
	}

	// Build request messages.
	var openaiMsgs []openai.ChatCompletionMessage
	systemRole := openai.ChatMessageRoleSystem
	if !model.SystemPrompt {
		systemRole = util.ChatMessageRoleDeveloper
	}
	openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessage{
		Role:    systemRole,
		Content: systemPrompt,
	})
	for _, record := range session.ChatRecords {
		if record.Content == "" {
			continue
		}
		openaiMsgs = append(openaiMsgs, record.ToOpenAIChatMessage())
	}

	// Retreive photos if exists.
	photos := inMsg.Photo
	if len(photos) > 0 && len(openaiMsgs) > 0 {
		multiContent := make([]openai.ChatMessagePart, 0, len(photos)+1)
		multiContent = append(multiContent, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeText,
			Text: inMsg.Caption,
		})

		photo := photos[len(photos)-1]
		base64Image, err := handlePhoto(botState, photo)
		if err != nil {
			slog.Error("failed to retrieve photo", "error", err.Error(), "file_id", photo.FileID, "file_size", photo.FileSize)
		} else {
			multiContent = append(multiContent, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeImageURL,
				ImageURL: &openai.ChatMessageImageURL{
					URL: base64Image,
				},
			})
		}

		lastMsg := openai.ChatCompletionMessage{
			Role:         openai.ChatMessageRoleUser,
			MultiContent: multiContent,
		}
		openaiMsgs[len(openaiMsgs)-1] = lastMsg
	}

	slog.Debug("sending request to AI provider",
		"provider", model.Provider,
		"model_name", model.Name,
		"messages", len(openaiMsgs),
		"streaming", model.Stream)

	req := openai.ChatCompletionRequest{
		Messages:            openaiMsgs,
		Model:               model.Name,
		MaxCompletionTokens: botState.Config.MaxTokensPerResponse,
		Stream:              model.Stream,
	}

	if model.Temperature {
		req.Temperature = session.Temperature
	}

	if !model.Stream {
		processNonStreamingResponse(botState, inMsg, session, client, req)
	} else {
		processStreamingResponse(botState, inMsg, session, client, req)
	}
}

func handlePhoto(botState *State, photo botapi.PhotoSize) (base64Image string, err error) {
	bytes, err := util.DownloadFile(photo.FileID, botState.Bot)
	if err != nil {
		return
	}

	base64Image, err = util.EncodeImageToBase64(bytes)
	if err != nil {
		return
	}
	return
}

func processNonStreamingResponse(botState *State, inMsg *botapi.Message, session *Session, client *openai.Client, req openai.ChatCompletionRequest) {
	req.Stream = false
	responseContent := ""
	defer func() {
		session.ResponseChannel <- responseContent
	}()

	outMsg, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(true, responseContent, session), botState.Bot, botState.Config.UseTelegramify)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	resp, err := client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		slog.Error(err.Error())
		util.SendMessageQuick(inMsg.Chat.ID, "Failed to generate response.", botState.Bot)
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
}

func processStreamingResponse(botState *State, inMsg *botapi.Message, session *Session, client *openai.Client, req openai.ChatCompletionRequest) {
	req.Stream = true
	slog.Debug("starting streaming response",
		"user_id", inMsg.From.ID,
		"chat_id", inMsg.Chat.ID)

	responseContent := ""
	currentContent := ""
	defer func() {
		session.ResponseChannel <- responseContent
	}()

	outMsg, err := util.SendMessageMarkdown(inMsg.Chat.ID, wrapMessage(true, responseContent, session), botState.Bot, botState.Config.UseTelegramify)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		slog.Error("failed to create completion stream",
			"error", err,
			"model", req.Model)
		util.SendMessageQuick(inMsg.Chat.ID, "Failed to generate response.", botState.Bot)
		return
	}
	defer stream.Close()

	for {
		select {
		case <-session.StopChannel:
			slog.Info("response generation stopped by user",
				"user_id", inMsg.From.ID)
			return
		default:
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(false, currentContent, session), botState.Bot, botState.Config.UseTelegramify)
				return
			}
			if err != nil {
				slog.Error(err.Error())
				util.SendMessageQuick(inMsg.Chat.ID, "Failed to generate response.", botState.Bot)
				return
			}
			if len(resp.Choices) == 0 {
				slog.Warn("Empty response")
				continue
			}
			deltaContent := resp.Choices[0].Delta.Content
			responseContent += deltaContent
			currentContent += deltaContent

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
				select {
				case <-botState.EditThrottler:
					util.EditMessageMarkdown(outMsg.Chat.ID, outMsg.MessageID, wrapMessage(true, currentContent, session), botState.Bot, botState.Config.UseTelegramify)
				default:
				}
			}
		}
	}
}

// wrapMessage adds a header banner to show the model and status.
func wrapMessage(isResponding bool, content string, session *Session) string {
	systemPromptField := ""
	if session.Prompt != "" {
		systemPromptField = fmt.Sprintf(", p: %s", session.Prompt)
	}

	var banner string
	if isResponding {
		banner = fmt.Sprintf("💭 *%s* (t: %.2f%s)\n\n", session.Model, session.Temperature, systemPromptField)
	} else {
		banner = fmt.Sprintf("🤗 *%s* (t: %.2f%s)\n\n", session.Model, session.Temperature, systemPromptField)
	}
	return banner + content
}

func tryStoppingResponse(session *Session) {
	if session.State == StateResponding {
		go func() { session.StopChannel <- struct{}{} }()
		session.State = StateIdle
	}
}

func tryDrainingResponseChannel(session *Session) {
	select {
	case <-session.ResponseChannel:
	default:
	}
}
