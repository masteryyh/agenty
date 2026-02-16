# Agenty CLI

Agenty CLI 是一个命令行客户端，用于与 Agenty 后端服务进行交互。

## 功能特性

- ✨ 精美的 ASCII Art 启动界面
- 📝 管理模型供应商（Provider）
- 🤖 管理AI模型（Model）
- 💬 管理聊天会话（Session）
- 🗨️ 交互式聊天界面
- 🎨 彩色输出和表格展示
- 🔧 完整的工具调用展示

## 安装

### 构建

```bash
# 构建CLI客户端
go build -o agenty-cli ./cmd/cli

# 构建后端服务器
go build -o agenty-server ./cmd/server.go
```

## 使用说明

### 全局参数

```bash
--url string      # 后端API地址 (默认: http://localhost:8080)
--config string   # 配置文件路径 (默认: ./config.yaml)
```

### 启动后端服务

首先需要启动后端服务：

```bash
./agenty-server
```

### Provider 管理

```bash
# 列出所有供应商
./agenty-cli provider list --page 1 --page-size 10

# 创建供应商
./agenty-cli provider create \
  --name "OpenAI" \
  --type openai \
  --base-url "https://api.openai.com/v1" \
  --api-key "sk-xxx"

# 删除供应商
./agenty-cli provider delete <provider-id> [--force]
```

### Model 管理

```bash
# 列出所有模型
./agenty-cli model list --page 1 --page-size 10

# 创建模型
./agenty-cli model create \
  --name "gpt-4" \
  --provider-id <provider-id>

# 删除模型
./agenty-cli model delete <model-id>
```

### Session 管理

```bash
# 列出所有会话
./agenty-cli session list --page 1 --page-size 10

# 创建会话
./agenty-cli session create

# 查看会话详情
./agenty-cli session view <session-id>
```

### 交互式聊天

```bash
# 开始聊天（自动使用最近的会话和默认模型）
./agenty-cli chat

# 指定特定会话
./agenty-cli chat --session <session-id>

# 指定特定模型
./agenty-cli chat --model <model-id>

# 指定会话和模型
./agenty-cli chat --session <session-id> --model <model-id>
```

在聊天界面中可用的命令：
- 直接输入消息并按回车发送
- `/new` - 开始新的聊天会话（清空屏幕）
- `/status` - 查看当前会话状态（ID、token消耗、消息数等）
- `/model provider-name/model-name` - 切换到不同的模型
- `/help` - 显示帮助信息
- `exit` - 退出聊天

#### 高级编辑功能

CLI使用readline库提供强大的输入编辑功能：

**中文支持**
- ✅ 完整的Unicode/UTF-8支持，包括中文、日文、emoji等
- ✅ Backspace正确删除多字节字符
- ✅ 左右箭头键在字符间正确移动

**命令补全**
- 按`Tab`键自动补全斜杠命令
- 输入`/`后按`Tab`显示所有可用命令

**历史记录**
- 使用上下箭头浏览历史输入
- 历史记录跨会话保持

**编辑快捷键**
- `Ctrl+A` - 移动到行首
- `Ctrl+E` - 移动到行尾
- `Ctrl+K` - 删除到行尾
- `Ctrl+U` - 删除到行首
- `Ctrl+W` - 删除前一个单词
- `Ctrl+C` - 中断当前输入

## 消息展示

CLI会以不同颜色和图标展示不同类型的消息：

- 👤 **用户消息** - 青色
- 🤖 **AI助手消息** - 绿色
  - 💭 **Reasoning（推理过程）** - 蓝色/灰色（Kimi模型专属）
- 🔧 **工具调用** - 黄色，显示工具名称和参数
- 🛠️ **工具结果** - 紫色
  - ✅ 成功 - 绿色
  - ❌ 错误 - 红色

### Reasoning 消息展示

对于支持推理过程的模型（如Kimi），CLI会单独展示推理内容：

```
🤖 Assistant (moonshot-v1-128k) [14:23:45]:
  💭 Reasoning:
  用户想要了解天气情况，我需要调用天气API获取信息...
  
  根据您的位置，今天天气晴朗，温度约为25°C。
```

## 示例工作流

```bash
# 1. 启动后端服务
./agenty-server

# 2. 创建供应商
./agenty-cli provider create \
  --name "OpenAI" \
  --type openai \
  --base-url "https://api.openai.com/v1" \
  --api-key "sk-xxx"

# 3. 记录provider-id，创建模型
./agenty-cli model create \
  --name "gpt-4" \
  --provider-id <provider-id>

# 4. 开始聊天（自动使用默认模型和最近会话）
./agenty-cli chat

# 在聊天中使用斜杠命令
You: /status
📊 Session Status
  Session ID: xxx
  Token Consumed: 1234
  Messages: 10
  
You: /model OpenAI/gpt-3.5-turbo
✓ Switched to model: gpt-3.5-turbo (from OpenAI)

You: /new
✓ Started new session: yyy
```

## 开发

### 项目结构

```
cmd/
  cli/main.go          # CLI入口
  server.go            # 服务器入口
pkg/
  cli/
    client/client.go   # API客户端
    cmd/               # CLI命令
      root.go          # 根命令
      provider.go      # Provider管理
      model.go         # Model管理
      session.go       # Session管理
      chat.go          # 聊天命令
```

### 依赖

- [cobra](https://github.com/spf13/cobra) - CLI框架
- [pterm](https://github.com/pterm/pterm) - 终端美化
- [sonic](https://github.com/bytedance/sonic) - JSON解析

## 注意事项

1. CLI需要后端服务运行才能工作
2. 默认后端地址是 `http://localhost:8080`，可通过 `--url` 参数修改
3. 工具调用需要后端正确配置tools注册
4. 聊天超时时间为120秒

## 许可证

Apache License 2.0
