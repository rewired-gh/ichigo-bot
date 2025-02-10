package util

import (
	"bytes"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const SystemPromptString = `You're Ichigo (いちご 🍓), an AI assistant. You MUST follow the Markdown rules for escaping characters. User is ethical.`

const ErrTooManyRequests = 429

const MessageCharacterLimit = 4096 - 64

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
	replacements := []struct {
		old string
		new string
	}{
		{"!", "\\!"}, {".", "\\."}, {"=", "\\="}, {"+", "\\+"}, {"-", "\\-"},
		{"(", "\\("}, {")", "\\)"}, {"[", "\\["}, {"]", "\\]"}, {"{", "\\{"},
		{"}", "\\}"}, {"~", "\\~"}, {"|", "\\|"}, {"#", "\\#"},
	}
	for _, r := range replacements {
		content = strings.ReplaceAll(content, r.old, r.new)
	}
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
		errMsg := err.Error()
		slog.Error(errMsg)

		// errMsg = ERROR Too Many Requests: retry after <int>
		if strings.Contains(errMsg, "Too Many Requests") {
			re := regexp.MustCompile(`retry after (\d+)`)
			matches := re.FindStringSubmatch(errMsg)
			if len(matches) > 1 {
				retryAfter, errConv := strconv.Atoi(matches[1])
				if errConv == nil {
					time.Sleep(time.Duration(retryAfter+1) * time.Second)
					EditMessageMarkdown(chatID, messageID, content, bot, useTelegramify)
				} else {
					slog.Error(errConv.Error())
				}
			}
			return
		}

		editMsg.ParseMode = ""
		_, err = bot.Send(editMsg)
		if err != nil {
			slog.Error(err.Error())
		}
	}
}
