# CLI消息加载和工具调用展示 - 可视化演示

本文档展示新功能的实际效果和使用场景。

## 演示1: 分批加载历史消息

### 场景：启动已有大量历史的会话

```bash
$ ./agenty-cli chat

▄████████    ▄██████▄     ▄████████ ███▄▄▄▄       ███     ▄██   ▄        
...

ℹ Resuming last session: 550e8400-e29b-41d4-a716-446655440000
ℹ Using model: gpt-4/OpenAI
ℹ Showing last 10 messages (total: 50). Use /history to load more.

Previous Messages

👤 User [14:20:00]:
  What files are in /home directory?

🤖 Assistant (gpt-4) [14:20:01]:
  
  🔧 Tool Execution:
  └─ list_files {"path": "/home"}
     ✅ Success
     user, admin, guest

  📝 Final Response:
  There are 3 directories in /home: user, admin, and guest.

👤 User [14:21:00]:
  Read the hosts file

🤖 Assistant (gpt-4) [14:21:01]:
  
  🔧 Tool Execution:
  └─ read_file {"path": "/etc/hosts"}
     ✅ Success
     127.0.0.1  localhost...

  📝 Final Response:
  The hosts file contains the standard localhost configuration.

[显示最近8条消息...]

ℹ Available commands:
  • Type your message and press Enter to chat
  • /new - Start a new chat session
  • /status - Show current session status
  • /history [n] - Load more message history (default: 20)
  • /model [provider/model] - Switch to a different model
  • /exit - Quit the chat

You: 
```

### 场景：加载更多历史

```bash
You: /history

📜 Message History (20 of 50 total)
Showing messages from #31 to #50

👤 User [13:50:00]:
  Hello

🤖 Assistant (gpt-4) [13:50:01]:
  Hi! How can I help you today?

👤 User [13:51:00]:
  List all Python files

🤖 Assistant (gpt-4) [13:51:01]:
  
  🔧 Tool Execution:
  └─ list_files {"path": ".", "pattern": "*.py"}
     ✅ Success
     main.py, utils.py, config.py

  📝 Final Response:
  I found 3 Python files: main.py, utils.py, and config.py.

[显示更多历史消息...]

You: 
```

### 场景：加载所有历史

```bash
You: /history 50

📜 Message History (50 of 50 total)

👤 User [12:00:00]:
  Let's start a new project

🤖 Assistant (gpt-4) [12:00:01]:
  Great! What kind of project would you like to work on?

[显示所有50条消息...]

You: 
```

## 演示2: 优化的工具调用展示

### 场景1: 单个工具调用

#### 改进前
```
🤖 Assistant (gpt-4) [14:30:00]:
  
  🔧 Tool Calls:
    • read_file
      {
        "path": "/etc/hosts"
      }

🛠️ Tool Result [14:30:01]:
  ✅ read_file
  127.0.0.1       localhost
  ::1             localhost ip6-localhost ip6-loopback
  fe00::0         ip6-localnet
  ...

🤖 Assistant (gpt-4) [14:30:02]:
  The hosts file contains the standard localhost configuration mappings 
  for both IPv4 and IPv6 addresses.
```

#### 改进后
```
🤖 Assistant (gpt-4) [14:30:00]:
  
  🔧 Tool Execution:
  └─ read_file {"path": "/etc/hosts"}
     ✅ Success
     127.0.0.1       localhost...
  
  📝 Final Response:
  The hosts file contains the standard localhost configuration mappings 
  for both IPv4 and IPv6 addresses.
```

### 场景2: 多个工具调用

#### 改进前（分散混乱）
```
🤖 Assistant (gpt-4) [14:35:00]:
  
  🔧 Tool Calls:
    • list_files
      {
        "path": "/home/user"
      }
    • read_file
      {
        "path": "/home/user/config.json"
      }

🛠️ Tool Result [14:35:01]:
  ✅ list_files
  config.json, data.txt, notes.md

🛠️ Tool Result [14:35:02]:
  ✅ read_file
  {"server": "localhost", "port": 8080}

🤖 Assistant (gpt-4) [14:35:03]:
  I found 3 files in the directory. The config.json contains server 
  settings with localhost:8080.
```

#### 改进后（清晰有序）
```
🤖 Assistant (gpt-4) [14:35:00]:
  
  🔧 Tool Execution:
  ├─ list_files {"path": "/home/user"}
  │  ✅ Success
  │  config.json, data.txt, notes.md
  └─ read_file {"path": "/home/user/config.json"}
     ✅ Success
     {"server": "localhost", "port": 8080}
  
  📝 Final Response:
  I found 3 files in the directory. The config.json contains server 
  settings with localhost:8080.
```

### 场景3: 工具调用失败

```
🤖 Assistant (gpt-4) [14:40:00]:
  
  🔧 Tool Execution:
  └─ read_file {"path": "/nonexistent/file.txt"}
     ❌ Error
     File not found: /nonexistent/file.txt
  
  📝 Final Response:
  I couldn't read the file because it doesn't exist. Would you like me 
  to create it, or check a different path?
```

### 场景4: 复杂工作流

```
🤖 Assistant (gpt-4) [14:45:00]:
  
  🔧 Tool Execution:
  ├─ search_files {"pattern": "*.log", "path": "/var/log"}
  │  ✅ Success
  │  system.log, error.log, access.log
  ├─ read_file {"path": "/var/log/error.log"}
  │  ✅ Success
  │  [ERROR] Connection timeout at 14:30:45...
  └─ http_request {"url": "https://api.example.com/alert", "method": "POST"}
     ✅ Success
     {"status": "alert_sent", "id": "alert_12345"}
  
  📝 Final Response:
  I found 3 log files. The error.log shows a connection timeout at 
  14:30:45. I've sent an alert to the monitoring system (alert_12345).
```

### 场景5: 带Reasoning的工具调用（Kimi模型）

```
🤖 Assistant (moonshot-v1-128k) [14:50:00]:
  💭 Reasoning:
  用户想要分析系统日志文件。我需要先列出日志目录的文件，
  然后读取最新的错误日志来帮助诊断问题。
  
  🔧 Tool Execution:
  ├─ list_files {"path": "/var/log", "sort": "mtime"}
  │  ✅ Success
  │  error.log (modified: 2m ago), system.log (modified: 1h ago)
  └─ read_file {"path": "/var/log/error.log", "lines": 50}
     ✅ Success
     [ERROR] 14:48:30 Database connection failed...
  
  📝 Final Response:
  我检查了日志目录，发现最近2分钟前更新的error.log文件。
  其中显示数据库连接在14:48:30失败。建议检查数据库服务状态。
```

## 演示3: 混合消息场景

### 场景：包含普通对话和工具调用

```bash
You: Hello!

🤖 Assistant (gpt-4) [15:00:00]:
  Hello! How can I help you today?

You: What's in the current directory?

🤖 Assistant (gpt-4) [15:00:05]:
  
  🔧 Tool Execution:
  └─ list_files {"path": "."}
     ✅ Success
     main.py, README.md, requirements.txt, data/

  📝 Final Response:
  The current directory contains:
  - main.py (Python script)
  - README.md (documentation)
  - requirements.txt (dependencies)
  - data/ (subdirectory)

You: Thanks!

🤖 Assistant (gpt-4) [15:00:15]:
  You're welcome! Let me know if you need help with anything else.

You: /status

📊 Session Status
  Session ID: 550e8400-e29b-41d4-a716-446655440000
  Token Consumed: 350
  Messages: 8
  Created: 2026-02-16 14:00:00
  Updated: 2026-02-16 15:00:15
```

## 演示4: 状态命令增强

```bash
You: /status

📊 Session Status
  Session ID: 550e8400-e29b-41d4-a716-446655440000
  Token Consumed: 1,234
  Messages: 50
  Created: 2026-02-16 12:00:00
  Updated: 2026-02-16 15:00:00

You: /history

📜 Message History (20 of 50 total)
Showing messages from #31 to #50

[显示20条消息...]
```

## 视觉对比总结

### 工具调用展示对比

| 特性 | 旧格式 | 新格式 |
|-----|--------|--------|
| 消息数量 | 3条独立消息 | 1条合并消息 |
| 视觉层次 | ❌ 平铺 | ✅ 树状结构 |
| 参数显示 | 多行JSON | 紧凑一行 |
| 结果显示 | 完整内容 | 智能截断 |
| 因果关系 | ❌ 不明确 | ✅ 清晰展示 |
| 屏幕占用 | 高（~20行） | 低（~8行） |
| 可读性 | ⭐⭐ | ⭐⭐⭐⭐⭐ |

### 历史加载对比

| 特性 | 改进前 | 改进后 |
|-----|--------|--------|
| 初始加载 | 全部消息 | 最近10条 |
| 启动时间 | 慢（500+ 消息） | 快 |
| 信息密度 | 过高 | 适中 |
| 历史访问 | 全部或没有 | 灵活按需 |
| 用户控制 | ❌ 无 | ✅ 完全控制 |
| 内存使用 | 高 | 优化 |

## 用户反馈场景

### 场景1: 新用户首次使用

**期望**: 快速看到相关信息，不被历史淹没

**体验**:
```
✅ 快速启动
✅ 只显示最近10条消息
✅ 清晰的提示告知有更多历史
✅ 可以按需加载更多
```

### 场景2: 调试工具调用问题

**期望**: 清楚地看到工具执行流程

**体验**:
```
✅ 树状结构展示调用顺序
✅ 清晰的成功/失败标识
✅ 紧凑但完整的信息
✅ 易于追踪问题
```

### 场景3: 回顾长期项目

**期望**: 能够浏览完整的对话历史

**体验**:
```
✅ 使用 /history 命令灵活加载
✅ 可以指定加载数量
✅ 清晰的加载范围提示
✅ 保持时间顺序
```

## 技术亮点

1. **智能分组**: 自动识别工具调用序列
2. **树状展示**: 使用Unicode字符创建视觉层次
3. **智能截断**: 保持信息完整性的同时节省空间
4. **状态标识**: 清晰的✅/❌标记
5. **按需加载**: 灵活的历史访问策略
6. **向后兼容**: 不影响现有功能

## 实际效果

这些改进使CLI聊天界面更加：
- 🚀 **快速**: 启动和响应更快
- 📊 **清晰**: 信息组织更有条理
- 🎯 **精准**: 显示最相关的内容
- 🛠️ **实用**: 工具调用流程一目了然
- 💡 **智能**: 自动优化展示格式
