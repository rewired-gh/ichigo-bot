package util

import (
	"log/slog"
	"strings"

	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func convertToTelegramMarkdown(content string) string {
	content = strings.ReplaceAll(content, "!", "\\!")
	content = strings.ReplaceAll(content, ".", "\\.")
	content = strings.ReplaceAll(content, "=", "\\=")
	content = strings.ReplaceAll(content, "+", "\\+")
	content = strings.ReplaceAll(content, "-", "\\-")
	content = strings.ReplaceAll(content, "(", "\\(")
	content = strings.ReplaceAll(content, ")", "\\)")
	content = strings.ReplaceAll(content, "[", "\\[")
	content = strings.ReplaceAll(content, "]", "\\]")
	content = strings.ReplaceAll(content, "{", "\\{")
	content = strings.ReplaceAll(content, "}", "\\}")
	content = strings.ReplaceAll(content, "~", "\\~")
	content = strings.ReplaceAll(content, "|", "\\|")
	content = strings.ReplaceAll(content, "#", "\\#")
	return content
}

func SendMessageQuick(chatID int64, content string, bot *botapi.BotAPI) {
	msg := botapi.NewMessage(chatID, content)
	_, err := bot.Send(msg)
	if err != nil {
		slog.Error(err.Error())
	}
}

func SendMessageMarkdown(chatID int64, content string, bot *botapi.BotAPI) (botapi.Message, error) {
	msg := botapi.NewMessage(chatID, convertToTelegramMarkdown(content))
	msg.ParseMode = botapi.ModeMarkdownV2
	return bot.Send(msg)
}

func EditMessageMarkdown(chatID int64, messageID int, content string, bot *botapi.BotAPI) {
	editMsg := botapi.NewEditMessageText(chatID, messageID, convertToTelegramMarkdown(content))
	editMsg.ParseMode = botapi.ModeMarkdownV2
	_, err := bot.Send(editMsg)
	if err != nil {
		slog.Error(err.Error())
		editMsg.ParseMode = botapi.ModeMarkdown
		_, err = bot.Send(editMsg)
		if err != nil {
			slog.Error(err.Error())
		}
	}
}
