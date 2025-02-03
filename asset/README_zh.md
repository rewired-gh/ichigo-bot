# 🍓 Ichigo Bot

[![Build and Release](https://github.com/rewired-gh/ichigo-bot/actions/workflows/release.yml/badge.svg)](https://github.com/rewired-gh/ichigo-bot/actions/workflows/release.yml)

一个由 OpenAI 兼容 API 驱动的令人愉悦的 Telegram 机器人，用于有趣友好的聊天互动！🌟

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

1. 在本地目录中创建配置文件 `config.toml`。配置文件的名字必须是 `config.toml`，而不是其他名字。**假设**您的本地目录为 `/path/to/data`。请参考 [`asset/example_config.toml`](asset/example_config.toml) 获取配置示例。

2. 运行 Docker 容器：
```bash
docker run -d \
  --name ichigod \
  -v /path/to/data:/etc/ichigod \
  -e ICHIGOD_DATA_DIR=/etc/ichigod \
  --restart unless-stopped \
  dockerrewired/ichigo-bot:latest
```

3. 管理 Docker 容器：
```bash
# 停止容器
docker stop ichigod

# 启动容器
docker start ichigod

# 重启容器
docker restart ichigod

# 移除容器
docker rm ichigod
```

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

## 🎮 命令

- `/chat` - 与 Ichigo 聊天 (在私信中可以省略)
- `/new` - 开始新的对话
- `/set` - 切换到不同的模型
- `/list` - 显示可用模型
- `/undo` - 移除最后一轮对话
- `/stop` - 停止当前响应

管理命令：
- `/get_config` - 查看当前配置
- `/set_config` - 更新配置并关闭
- `/clear` - 清除数据

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