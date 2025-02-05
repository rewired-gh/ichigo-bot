# 🍓 Ichigo Bot

[![Build and Release](https://github.com/rewired-gh/ichigo-bot/actions/workflows/release.yml/badge.svg)](https://github.com/rewired-gh/ichigo-bot/actions/workflows/release.yml) [![GitHub Release (Latest)](https://img.shields.io/github/v/release/rewired-gh/ichigo-bot)](https://github.com/rewired-gh/ichigo-bot/releases/latest) [![Docker Pulls](https://img.shields.io/docker/pulls/dockerrewired/ichigo-bot)](https://hub.docker.com/r/dockerrewired/ichigo-bot)

令人愉悦的 Telegram AI 聊天机器人，用于聚合模型和 API 提供商。🌟

如果你不知道从何开始，请访问 [cheahjs/free-llm-api-resources](https://github.com/cheahjs/free-llm-api-resources) 获取丰富的 LLM API 资源，并参考 [Telegram Bots](https://core.telegram.org/bots#how-do-i-create-a-bot) 来了解如何创建自己的机器人。

## ✨ 功能特性

- 🛡️ 生产就绪，具有超强健的错误处理能力
- 🤖 兼容 OpenAI API 和类似提供商
- 💫 神奇的流式聊天响应
- 🎮 混合搭配您最喜欢的模型和提供商
- 🎯 智能系统提示，实现更佳对话
- 🔐 通过用户访问控制保障您的聊天安全
- 📝 美观的 Telegram Markdown V2 格式
- 🪶 在您的服务器上轻如鸿毛

## 🐳 快速 Docker 部署 (beta)

1. 创建一个本地数据目录。**假设**这个数据目录路径为 `/path/to/data`：
```bash
mkdir -p /path/to/data
```

2. 在 `/path/to/data` 中创建配置文件 `config.toml`。配置文件的名字**必须**是 `config.toml`，而不是其他名字。请参考 [`asset/example_config.toml`](asset/example_config.toml) 获取配置示例。

3. 运行 Docker 容器（替换 `/path/to/data` 为真正的数据目录路径）：
```bash
sudo docker run -d \
  --name ichigod \
  -v /path/to/data:/etc/ichigod \
  -e ICHIGOD_DATA_DIR=/etc/ichigod \
  --restart unless-stopped \
  dockerrewired/ichigo-bot:latest
```

## 🎮 命令

- `/chat` - 与 Ichigo 聊天 (在私信中可以省略)
- `/new` - 开始新的对话
- `/set` - 切换到不同的模型
- `/list` - 显示可用模型
- `/undo` - 移除最后一轮对话
- `/stop` - 停止当前响应
- `/set_temp` - 设置文本补全温度
- `/help` - 获取命令列表

管理命令：
- `/get_config` - 查看当前配置
- `/set_config` - 更新配置并关闭
- `/clear` - 清除数据
- `/tidy` - 自动删除不存在的会话及聊天记录

## 🚀 快速开始

### 前提条件

- Go 1.21 或更高版本
- Make
- 安装了 `telegramify-markdown` 包的 Python 3
- Telegram 机器人令牌（通过 [@BotFather](https://t.me/BotFather) 获取）
- OpenAI API 密钥或其他兼容 API 提供商凭据
- 提示：用户 ID 和群组聊天 ID 可以通过 [@RawDataBot](https://t.me/RawDataBot) 获取

### 构建

```bash
make build
```
就是这样！**假设**构建的二进制文件为 `/project_root/target/ichigod`。

### 部署（Linux 与 systemd）

1. 将构建的二进制文件移动到 `/usr/bin` 并授予必要的权限：
```bash
# 示例命令
sudo chmod a+rx /project_root/target/ichigod
sudo cp -f /project_root/target/ichigod /usr/bin/ichigod
```

2. 在 `/etc/ichigod` 创建数据目录：
```bash
# 示例命令
sudo mkdir -p /etc/ichigod
```

3. 在 `/etc/ichigod` 中创建配置文件 `config.toml`。配置文件的名字必须是 `config.toml`，而不是其他名字。请参考 [`asset/example_config.toml`](asset/example_config.toml) 获取配置示例。

4. 在 `/etc/ichigod/venv` 中创建安装了 `telegramify-markdown` 的 Python 虚拟环境：
```bash
# 示例命令
cd /etc/ichigod
python3 -m venv venv
source venv/bin/activate
pip install telegramify-markdown
```

5. 在 `/etc/systemd/system/ichigod.service` 创建 systemd 服务单元：
```ini
# 示例服务单元
[Unit]
Description=Ichigo Telegram 机器人服务
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

6. 启用并启动服务：
```bash
# 示例命令
sudo systemctl daemon-reload
sudo systemctl enable ichigod
sudo systemctl start ichigod
```

7. 检查服务日志：
```bash
# 示例命令
sudo journalctl -u ichigod.service | tail -8

# 示例输出
# <已编辑> systemd[1]: Started ichigod.service - Ichigo Telegram 机器人服务。
# <已编辑> ichigod[202711]: <已编辑> INFO starting ichigod
# <已编辑> ichigod[202711]: <已编辑> INFO initializing bot service
# <已编辑> ichigod[202711]: <已编辑> INFO bot API client initialized username=<已编辑> debug_mode=false
```

## 🛠️ 开发

开发命令：
```bash
make dev        # 在开发模式下运行
make debug      # 使用调试器运行
make build      # 为当前平台构建
make build_x64  # 为 Linux x64 构建
```

## ⚙️ 配置

机器人会在以下位置查找 `config.toml`：
1. `$ICHIGOD_DATA_DIR`
2. `/etc/ichigod/`
3. `$HOME/.config/ichigod/`
4. 当前目录