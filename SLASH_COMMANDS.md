# CLI Slash Commands Demo

本文档演示了新添加的斜杠命令功能。

## 功能概述

### 1. 自动Session选择
用户启动chat时不需要指定session ID，CLI会：
- 自动选择最近使用的session
- 如果没有session，自动创建新的
- 显示恢复的历史消息

### 2. 自动Model选择
用户启动chat时不需要指定model ID，CLI会：
- 自动选择第一个可用的模型
- 显示正在使用的模型名称和Provider
- 可通过 `/model` 命令动态切换

### 3. 斜杠命令

#### `/new` - 开始新对话
```
You: /new
✓ Started new session: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /model [provider/model] - Switch to a different model
  • exit - Quit the chat
```

清空屏幕，显示新session ID，准备开始新对话。

#### `/status` - 查看会话状态
```
You: /status

📊 Session Status
  Session ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  Token Consumed: 1234
  Messages: 10
  Created: 2026-02-16 12:30:45
  Updated: 2026-02-16 12:35:20
```

显示：
- Session UUID
- 累计消耗的token数量
- 消息总数
- 创建和更新时间

#### `/model` - 切换模型
```
You: /model OpenAI/gpt-4
✓ Switched to model: gpt-4 (from OpenAI)

You: /model Anthropic/claude-3-opus
✓ Switched to model: claude-3-opus (from Anthropic)
```

格式：`/model provider-name/model-name`
- Provider名称和Model名称不区分大小写
- 自动查找匹配的provider和model
- 显示切换成功的确认信息

#### `/help` - 显示帮助
```
You: /help
ℹ Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /model [provider/model] - Switch to a different model
  • exit - Quit the chat
```

## 使用示例

### 场景1：首次使用
```bash
$ ./agenty-cli chat
ℹ Created new session: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
ℹ Using model: gpt-4 (from OpenAI)

Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /model [provider/model] - Switch to a different model
  • exit - Quit the chat

You: Hello!
🤖 Assistant (gpt-4) [12:30:45]:
  Hi! How can I help you today?
```

### 场景2：恢复上次对话
```bash
$ ./agenty-cli chat
ℹ Resuming last session: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
ℹ Using model: gpt-4 (from OpenAI)

Previous Messages
👤 User [12:30:45]:
  Hello!

🤖 Assistant (gpt-4) [12:30:46]:
  Hi! How can I help you today?

Available commands:
  ...

You: Tell me more about AI
```

### 场景3：使用斜杠命令
```bash
You: /status
📊 Session Status
  Session ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  Token Consumed: 150
  Messages: 2
  Created: 2026-02-16 12:30:45
  Updated: 2026-02-16 12:30:46

You: /model Anthropic/claude-3-opus
✓ Switched to model: claude-3-opus (from Anthropic)

You: What's the weather like?
🤖 Assistant (claude-3-opus) [12:31:00]:
  I don't have access to real-time weather information...

You: /new
✓ Started new session: yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy

You: Start fresh conversation...
```

## 技术实现细节

### 命令解析
- 在输入循环中检测以 `/` 开头的输入
- 使用 `strings.Fields()` 分割命令和参数
- 每个命令有独立的处理函数

### 状态管理
- 在聊天循环中维护 `currentSessionID` 和 `currentModelID`
- 命令处理函数返回新的ID值
- 动态更新当前使用的session和model

### 屏幕清除
- 使用ANSI转义序列 `\033[2J\033[H`
- 兼容大多数现代终端
- `/new` 命令执行清屏操作

### Provider/Model查找
- 使用 `strings.EqualFold()` 实现不区分大小写匹配
- 遍历所有providers和models查找匹配项
- 提供友好的错误提示

## 错误处理

### 命令格式错误
```
You: /model gpt-4
Command error: invalid format, use: provider-name/model-name
```

### Provider不存在
```
You: /model UnknownProvider/model
Command error: provider 'UnknownProvider' not found
```

### Model不存在
```
You: /model OpenAI/unknown-model
Command error: model 'unknown-model' not found in provider 'OpenAI'
```

## 用户体验提升

1. **无需记忆ID**: 用户不需要复制粘贴UUID
2. **快速切换**: 使用友好的名称而不是UUID切换模型
3. **实时反馈**: 所有操作都有清晰的确认信息
4. **自然对话**: 减少中断，专注于对话本身
5. **灵活控制**: 需要时可以使用命令，不需要时完全透明
