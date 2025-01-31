package main

import (
	"log/slog"

	"github.com/rewired-gh/ichigo-bot/internal/app"
	"github.com/rewired-gh/ichigo-bot/internal/util"
	"github.com/tillberg/autorestart"
)

func main() {
	config, err := util.LoadConfig()

	slog.Info("Loaded config", "config", config)

	if err != nil {
		slog.Error(err.Error())
		return
	}

	go autorestart.RestartOnChange()

	for {
		app.StartBotService(&config)
	}
}
