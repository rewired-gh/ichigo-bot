# Ichigo Bot

A Telegram bot powered by OpenAI-compatible APIs for chat interactions.

## Features

- 🛡️ Production-ready with robust error handling
- 🤖 Compatible with OpenAI API and similar providers
- 💬 Streaming chat responses
- 🔄 Configurable models and providers
- 🎯 Support for system prompts
- 🔐 User access control
- 📝 Telegram Markdown V2 formatting support
- 🪶 Minimal performance footprint

## Quick Start

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

2. Create data directory at `/etc/ichigod`:
```bash
# Example commands
sudo mkdir -p /etc/ichigod
```

3. Create a configuration file `config.toml` in `/etc/ichigod`:
```toml
# Example configuration
token = "YOUR_TELEGRAM_BOT_TOKEN"
admins = [123456789]  # Your Telegram user ID
users = []  # Allowed user IDs
groups = []  # Allowed group chat IDs
default_model = "o3m" # Alias of the default model
max_tokens_per_response = 4000
max_chat_records_per_user = 32
use_telegramify = true # telegramify-markdown must be installed
debug = false

[[providers]]
name = "openai"
base_url = "https://api.openai.com/v1"
api_key = "YOUR_OPENAI_API_KEY"

[[models]]
alias = "o3m"
name = "o3-mini"
provider = "openai"
stream = false # o3-mini doesn't support streaming response
system_prompt = false # o3-mini doesn't support system prompt

[[models]]
alias = "4o"
name = "4o"
provider = "openai"
stream = true
system_prompt = true
```

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

## Commands

- `/chat` - Chat with Ichigo (Can be omitted in private messages)
- `/new` - Start a new conversation
- `/set` - Switch to a different model
- `/list` - Show available models
- `/undo` - Remove last conversation round
- `/stop` - Stop the current response

Admin Commands:
- `/get_config` - View current configuration
- `/set_config` - Update configuration and shutdown
- `/clear` - Clear data

## Development

Development commands:
```bash
make dev        # Run in development mode
make debug      # Run with debugger
make build      # Build for current platform
make build_x64  # Build for Linux x64
```

## Configuration

The bot looks for `config.toml` in these locations:
1. `$ICHIGOD_DATA_DIR`
2. `/etc/ichigod/`
3. `$HOME/.config/ichigod/`
4. Current directory
