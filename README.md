# 🍓 Ichigo Bot

[![Build and Release](https://github.com/rewired-gh/ichigo-bot/actions/workflows/release.yml/badge.svg)](https://github.com/rewired-gh/ichigo-bot/actions/workflows/release.yml) [![GitHub Release (Latest)](https://img.shields.io/github/v/release/rewired-gh/ichigo-bot)](https://github.com/rewired-gh/ichigo-bot/releases/latest) [![Docker Pulls](https://img.shields.io/docker/pulls/dockerrewired/ichigo-bot)](https://hub.docker.com/r/dockerrewired/ichigo-bot)

[English](README.md) | [简体中文](asset/README_zh.md)

A delightful Telegram AI chat bot powered for aggregating models and API providers. 🌟

If you're not sure where to start, check out [cheahjs/free-llm-api-resources](https://github.com/cheahjs/free-llm-api-resources) for a wealth of LLM API resources, and visit [Telegram Bots](https://core.telegram.org/bots#how-do-i-create-a-bot) for guidance on creating your own bot.

## ✨ Features

- 🛡️ Production-ready with super-robust error handling
- 💫 Magical streaming chat responses
- 🖼️ Supports image in chat for multimodal LLM
- 🤖 Compatible with almost any API providers
- 🎮 Mix and match your favorite models and providers
- 🔐 Keeps your chats and models safe with user access control
- 📝 Beautiful Telegram Markdown V2 formatting
- 🎯 Smart system prompts for better conversations
- 🪶 Light as a feather on your server

## 🐳 Quick Docker Deployment (beta)

1. Create a local data directory. **Assume** the data directory path is `/path/to/data`:
```bash
mkdir -p /path/to/data
```

2. Create a configuration file `config.toml` in `/path/to/data`. The configuration file's name **must** be `config.toml`, not any other name. Please refer to [`asset/example_config.toml`](asset/example_config.toml) for an example configuration.

3. Run the Docker container (replace `/path/to/data` with the real data directory path):
```bash
sudo docker run -d \
  --name ichigod \
  -v /path/to/data:/etc/ichigod \
  -e ICHIGOD_DATA_DIR=/etc/ichigod \
  --restart unless-stopped \
  dockerrewired/ichigo-bot:latest
```

## 🎮 Commands

- `/chat` - Chat with Ichigo (Can be omitted in private messages)
- `/new` - Start a new conversation
- `/set` - Switch to a different model
- `/list` - Show available models
- `/list_prompts` - List available system prompts
- `/undo` - Remove last conversation round
- `/stop` - Stop the current response
- `/set_temp` - Set text completion temperature
- `/set_prompt` - Set system prompt
- `/help` - Get the list of commands

Admin Commands:
- `/get_config` - View current configuration
- `/set_config` - Update configuration and shutdown
- `/clear` - Clear data
- `/tidy` - Auto delete sessions and chat records whose IDs no longer exist

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or later
- Make
- Python 3 with `telegramify-markdown` package installed
- A Telegram bot token (obtained via [@BotFather](https://t.me/BotFather))
- OpenAI API key or other compatible API provider credentials
- Tip: User IDs and group chat IDs can be retrieved via [@RawDataBot](https://t.me/RawDataBot)

### Build

```bash
make build
```
That's it! **Assume** the built binary is `/project_root/target/ichigod`.

### Deploy (Linux with systemd)

1. Move the built binary to `/usr/bin` and grant necessary permissions:
```bash
# Example commands
sudo chmod a+rx /project_root/target/ichigod
sudo cp -f /project_root/target/ichigod /usr/bin/ichigod
```

2. Create a data directory at `/etc/ichigod`:
```bash
# Example commands
sudo mkdir -p /etc/ichigod
```

3. Create a configuration file `config.toml` in `/etc/ichigod`. The configuration file's name must be `config.toml`, not any other name. Please refer to [`asset/example_config.toml`](asset/example_config.toml) for an example configuration.

4. Create a Python virtual environment with `telegramify-markdown` installed in `/etc/ichigod/venv`:
```bash
# Example commands
cd /etc/ichigod
python3 -m venv venv
source venv/bin/activate
pip install telegramify-markdown
```

5. Create a systemd service unit at `/etc/systemd/system/ichigod.service`:
```ini
# Example service unit
[Unit]
Description=Ichigo Telegram Bot Service
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/bin/ichigod
Restart=always
RestartSec=5
Environment="ICHIGOD_DATA_DIR=/etc/ichigod"
Environment="PATH=/etc/ichigod/venv/bin"

[Install]
WantedBy=multi-user.target
```

6. Enable and start the service:
```bash
# Example commands
sudo systemctl daemon-reload
sudo systemctl enable ichigod
sudo systemctl start ichigod
```

7. Check the service log:
```bash
# Example commands
sudo journalctl -u ichigod.service | tail -8

# Example outputs
# <redacted> systemd[1]: Started ichigod.service - Ichigo Telegram Bot Service.
# <redacted> ichigod[202711]: <redacted> INFO starting ichigod
# <redacted> ichigod[202711]: <redacted> INFO initializing bot service
# <redacted> ichigod[202711]: <redacted> INFO bot API client initialized username=<redacted> debug_mode=false
```

## 🛠️ Development

Development commands:
```bash
make dev        # Run in development mode
make debug      # Run with debugger
make build      # Build for current platform
make build_x64  # Build for Linux x64
```

## ⚙️ Configuration

The bot looks for `config.toml` in these locations:
1. `$ICHIGOD_DATA_DIR`
2. `/etc/ichigod/`
3. `$HOME/.config/ichigod/`
4. Current directory
