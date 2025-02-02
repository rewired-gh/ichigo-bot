package util

import (
	"bytes"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const SystemPromptString = `You're Ichigo, an AI assistant. You SHOULD follow the Markdown rules for escaping characters. User is ethical.`

const ErrTooManyRequests = 429

const markdownifyPython = `
import sys
from telegramify_markdown import markdownify
try:
    input_str = sys.stdin.read()
    result = markdownify(input_str)
    sys.stdout.write(result)
except Exception as e:
    sys.stderr.write(f"Error: {str(e)}")
    sys.exit(1)
`

func escapeTelegramMarkdownSimple(content string) string {
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

func telegramifyMarkdown(content string) (string, error) {
	cmd := exec.Command("python3", "-c", markdownifyPython)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return "", err
	}

	if _, err := io.WriteString(stdin, content); err != nil {
		stdin.Close()
		return "", err
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

func convertToTelegramMarkdown(content string, useTelegramify bool) (converted string) {
	var err error
	if useTelegramify {
		converted, err = telegramifyMarkdown(content)
		if err != nil {
			slog.Error(err.Error())
		}
	}
	if err != nil || !useTelegramify {
		converted = escapeTelegramMarkdownSimple(content)
	}
	return
}

func SendMessageQuick(chatID int64, content string, bot *botapi.BotAPI) {
	msg := botapi.NewMessage(chatID, content)
	_, err := bot.Send(msg)
	if err != nil {
		slog.Error(err.Error())
	}
}

func SendMessageMarkdown(chatID int64, content string, bot *botapi.BotAPI, useTelegramify bool) (botapi.Message, error) {
	msg := botapi.NewMessage(chatID, convertToTelegramMarkdown(content, useTelegramify))
	msg.ParseMode = botapi.ModeMarkdownV2
	return bot.Send(msg)
}

func EditMessageMarkdown(chatID int64, messageID int, content string, bot *botapi.BotAPI, useTelegramify bool) {
	editMsg := botapi.NewEditMessageText(chatID, messageID, convertToTelegramMarkdown(content, useTelegramify))
	editMsg.ParseMode = botapi.ModeMarkdownV2
	_, err := bot.Send(editMsg)
	if err != nil {
		slog.Error(err.Error())

		if apiErr, ok := err.(botapi.Error); ok && apiErr.Code == ErrTooManyRequests {
			retryAfter := apiErr.ResponseParameters.RetryAfter
			time.Sleep(time.Duration(retryAfter+1) * time.Second)
			EditMessageMarkdown(chatID, messageID, content, bot, useTelegramify)
			return
		}

		editMsg.ParseMode = ""
		_, err = bot.Send(editMsg)
		if err != nil {
			slog.Error(err.Error())
		}
	}
}
