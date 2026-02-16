# CLI 使用演示

本文档展示优化后CLI的实际使用效果。

## 演示 1: 首次使用 - 零配置启动

```
$ ./agenty-cli chat

▄████████    ▄██████▄     ▄████████ ███▄▄▄▄       ███     ▄██   ▄        
  ███    ███   ███    ███   ███    ███ ███▀▀▀██▄ ▀█████████▄ ███   ██▄      
  ███    ███   ███    █▀    ███    █▀  ███   ███    ▀███▀▀██ ███▄▄▄███      
  ███    ███  ▄███          ███        ███   ███     ███   ▀ ▀▀▀▀▀▀███      
▀███████████ ▀▀███ ████▄  ▀███████████ ███   ███     ███     ▄██   ███      
  ███    ███   ███    ███          ███ ███   ███     ███     ███   ███      
  ███    ███   ███    ███    ▄█    ███ ███   ███     ███     ███   ███      
  ███    █▀    ████████▀   ▄████████▀   ▀█   █▀     ▄████▀    ▀█████▀       

ℹ Created new session: 550e8400-e29b-41d4-a716-446655440000
ℹ Using model: gpt-4 (from OpenAI)

ℹ Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /model [provider/model] - Switch to a different model
  • exit - Quit the chat

You: Hello! Tell me about yourself
⠋ Thinking...
🤖 Assistant (gpt-4) [14:23:45]:
  Hello! I'm an AI assistant powered by GPT-4. I'm designed to help with a 
  wide variety of tasks including answering questions, helping with analysis,
  writing, coding, and much more. How can I assist you today?

You: What's 2+2?
⠋ Thinking...
🤖 Assistant (gpt-4) [14:23:52]:
  2 + 2 = 4

You: exit
ℹ Goodbye!
```

## 演示 2: 恢复上次对话

```
$ ./agenty-cli chat

ℹ Resuming last session: 550e8400-e29b-41d4-a716-446655440000
ℹ Using model: gpt-4 (from OpenAI)

Previous Messages
👤 User [14:23:45]:
  Hello! Tell me about yourself

🤖 Assistant (gpt-4) [14:23:46]:
  Hello! I'm an AI assistant powered by GPT-4...

👤 User [14:23:50]:
  What's 2+2?

🤖 Assistant (gpt-4) [14:23:52]:
  2 + 2 = 4

ℹ Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /model [provider/model] - Switch to a different model
  • exit - Quit the chat

You: Can you help me with Python code?
⠋ Thinking...
🤖 Assistant (gpt-4) [14:25:10]:
  Of course! I'd be happy to help you with Python code. What would you 
  like assistance with?
```

## 演示 3: 使用斜杠命令

```
You: /status

📊 Session Status
  Session ID: 550e8400-e29b-41d4-a716-446655440000
  Token Consumed: 350
  Messages: 6
  Created: 2026-02-16 14:23:45
  Updated: 2026-02-16 14:25:10

You: /model Anthropic/claude-3-opus
✓ Switched to model: claude-3-opus (from Anthropic)

You: What's the capital of France?
⠋ Thinking...
🤖 Assistant (claude-3-opus) [14:26:30]:
  The capital of France is Paris.

You: /status

📊 Session Status
  Session ID: 550e8400-e29b-41d4-a716-446655440000
  Token Consumed: 425
  Messages: 8
  Created: 2026-02-16 14:23:45
  Updated: 2026-02-16 14:26:30

You: /new
✓ Started new session: 660f9511-f3ac-52e5-b827-557766551111

ℹ Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /model [provider/model] - Switch to a different model
  • exit - Quit the chat

You: Fresh start!
⠋ Thinking...
🤖 Assistant (claude-3-opus) [14:27:00]:
  Hello! Yes, we're starting fresh. How can I help you today?
```

## 演示 4: 工具调用展示

```
You: Can you read the file /etc/hosts?
⠋ Thinking...
🤖 Assistant (gpt-4) [14:28:00]:

  🔧 Tool Calls:
    • read_file
      {
        "path": "/etc/hosts"
      }

🛠️ Tool Result [14:28:01]:
  ✅ read_file
  127.0.0.1       localhost
  ::1             localhost ip6-localhost ip6-loopback
  ...

🤖 Assistant (gpt-4) [14:28:02]:
  I've read the /etc/hosts file. It contains the standard localhost 
  configuration mappings for both IPv4 and IPv6 addresses.
```

## 演示 5: 错误处理

```
You: /model UnknownProvider/gpt-4
✗ Command error: provider 'UnknownProvider' not found

You: /model OpenAI/unknown-model
✗ Command error: model 'unknown-model' not found in provider 'OpenAI'

You: /model gpt-4
✗ Command error: invalid format, use: provider-name/model-name

You: /model OpenAI/gpt-4
✓ Switched to model: gpt-4 (from OpenAI)
```

## 演示 6: Reasoning 内容展示 (Kimi模型)

```
You: /model Moonshot/moonshot-v1-128k
✓ Switched to model: moonshot-v1-128k (from Moonshot)

You: 帮我分析一下量子计算的发展趋势
⠋ Thinking...
🤖 Assistant (moonshot-v1-128k) [14:30:00]:
  💭 Reasoning:
  用户询问量子计算的发展趋势，这是一个复杂的技术话题。我需要从多个
  角度分析，包括技术进展、应用领域、挑战和未来方向。
  
  量子计算是一个快速发展的领域，目前主要有以下几个趋势：
  
  1. 量子比特数量增加...
  2. 错误纠正能力提升...
  3. 云端量子计算服务...
```

## 特性总结

### ✅ 实现的用户体验改进

1. **零配置启动**
   - 无需指定session或model
   - 自动恢复上次对话
   - 智能选择默认模型

2. **斜杠命令系统**
   - `/new` - 快速开始新对话
   - `/status` - 查看会话详情
   - `/model` - 动态切换模型
   - `/help` - 获取帮助

3. **友好的错误提示**
   - 清晰的错误信息
   - 使用说明和示例
   - 格式验证

4. **丰富的视觉反馈**
   - 彩色消息类型标识
   - 表情符号增强可读性
   - 加载动画显示处理状态
   - Reasoning内容特殊展示

### 🎯 实现的技术目标

1. **Session无感知**
   - ✅ 自动选择最近session
   - ✅ 自动创建新session
   - ✅ 保留历史记录

2. **命令系统**
   - ✅ 斜杠命令解析
   - ✅ `/new` 实现
   - ✅ `/status` 实现
   - ✅ `/model` 实现

3. **代码质量**
   - ✅ 通过go vet检查
   - ✅ 通过构建测试
   - ✅ 良好的错误处理
   - ✅ 完整的文档
