package main

import (
	"log/slog"

	"github.com/rewired-gh/ichigo-bot/internal/app"
	"github.com/rewired-gh/ichigo-bot/internal/util"
)

func main() {
	slog.Info("starting ichigod")
	config, err := util.LoadConfig()

	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		return
	}

	for {
		app.StartBotService(&config)
		slog.Warn("bot service terminated, restarting")
	}
}
