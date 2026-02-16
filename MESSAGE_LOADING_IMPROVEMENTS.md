# 历史消息加载和工具调用展示优化

本文档描述了CLI聊天界面的两个重要改进：分批加载历史消息和优化的工具调用展示。

## 1. 分批加载历史消息

### 功能描述

当恢复一个历史会话时，CLI不再一次性加载所有历史消息，而是：
- 启动时只显示最近的10条消息
- 如果有更多历史消息，显示提示信息
- 用户可以使用 `/history` 命令按需加载更多历史

### 使用方法

```bash
# 启动chat，自动显示最近10条消息
$ ./agenty-cli chat

ℹ Showing last 10 messages (total: 50). Use /history to load more.

Previous Messages
[显示最近10条消息...]

# 加载更多历史（默认20条）
You: /history
📜 Message History (20 of 50 total)
Showing messages from #31 to #50
[显示消息...]

# 加载指定数量的历史消息
You: /history 50
📜 Message History (50 of 50 total)
[显示所有消息...]
```

### 命令格式

```
/history [n]
```

- `n`: 可选参数，指定要加载的消息数量（默认：20）
- 如果消息总数少于请求的数量，显示所有消息

### 优点

1. **快速启动**: 不需要等待加载大量历史消息
2. **减少干扰**: 只显示最相关的最近消息
3. **按需加载**: 需要时可以轻松查看完整历史
4. **内存友好**: 对于有大量历史的会话更高效

## 2. 优化的工具调用展示

### 功能描述

工具调用序列（Assistant → Tool → Assistant）现在以更清晰、更美观的树状结构展示：

- 自动识别工具调用序列
- 合并相关消息到一个逻辑单元
- 使用树状结构展示工具执行流程
- 紧凑显示参数和结果

### 展示格式

#### 旧格式（独立消息）

```
🤖 Assistant (gpt-4) [14:30:00]:
  
  🔧 Tool Calls:
    • read_file
      {
        "path": "/etc/hosts"
      }

🛠️ Tool Result [14:30:01]:
  ✅ read_file
  127.0.0.1  localhost
  ...

🤖 Assistant (gpt-4) [14:30:02]:
  The file contains the localhost configuration...
```

#### 新格式（合并展示）

```
🤖 Assistant (gpt-4) [14:30:00]:
  
  🔧 Tool Execution:
  └─ read_file {"path": "/etc/hosts"}
     ✅ Success
     127.0.0.1  localhost...
  
  📝 Final Response:
  The file contains the localhost configuration...
```

### 展示特点

#### 1. 树状结构

使用 `├─` 和 `└─` 字符创建视觉层次：

```
🔧 Tool Execution:
├─ tool1 {"arg": "value1"}
│  ✅ Success
│  Result content...
└─ tool2 {"arg": "value2"}
   ✅ Success
   Result content...
```

#### 2. 紧凑的参数显示

- 参数显示在一行内（长度限制80字符）
- 超长参数自动截断并添加 `...`
- 使用紧凑的JSON格式

#### 3. 清晰的结果状态

- ✅ Success - 绿色显示成功
- ❌ Error - 红色显示错误
- 结果内容限制在100字符内

#### 4. 分段显示

- 工具执行部分独立显示
- 最终回复单独显示在 "📝 Final Response" 部分
- 支持Reasoning内容（Kimi模型）

### 支持的消息序列

1. **简单工具调用**
   ```
   Assistant (with tool calls) → Tool (result) → Assistant (final response)
   ```

2. **多个工具调用**
   ```
   Assistant (with multiple tool calls) → Tool → Tool → Tool → Assistant
   ```

3. **普通消息**
   - 没有工具调用的消息继续使用原有格式显示

### 示例场景

#### 场景1: 文件操作

```
🤖 Assistant (gpt-4) [14:30:00]:
  
  🔧 Tool Execution:
  ├─ list_files {"path": "/home/user"}
  │  ✅ Success
  │  file1.txt, file2.txt, file3.txt
  └─ read_file {"path": "/home/user/file1.txt"}
     ✅ Success
     Content of file1.txt...
  
  📝 Final Response:
  I found 3 files in the directory. Here's the content of file1.txt...
```

#### 场景2: API调用

```
🤖 Assistant (gpt-4) [14:35:00]:
  
  🔧 Tool Execution:
  └─ http_request {"url": "https://api.example.com/data", "method": "GET"}
     ✅ Success
     {"status": "ok", "data": [...]}
  
  📝 Final Response:
  The API returned successfully with the following data...
```

#### 场景3: 错误处理

```
🤖 Assistant (gpt-4) [14:40:00]:
  
  🔧 Tool Execution:
  └─ read_file {"path": "/nonexistent/file.txt"}
     ❌ Error
     File not found: /nonexistent/file.txt
  
  📝 Final Response:
  I couldn't read the file because it doesn't exist. Would you like me to...
```

## 技术实现

### 消息分组算法

```go
func printMessageHistory(messages []models.ChatMessageDto) {
    i := 0
    for i < len(messages) {
        msg := &messages[i]
        
        // 检查是否是带工具调用的assistant消息
        if msg.Role == models.RoleAssistant && len(msg.ToolCalls) > 0 {
            // 这是工具调用序列的开始
            printToolCallingSequence(messages, &i)
        } else {
            // 普通消息
            printMessage(msg)
            i++
        }
    }
}
```

### 工具调用序列识别

1. 检测 Assistant 消息中的 `ToolCalls` 字段
2. 向前查找对应的 Tool 结果消息
3. 继续查找最终的 Assistant 响应
4. 将这些消息合并为一个逻辑单元展示

### 显示优化

- 使用pterm库的颜色功能
- 树状字符：`├─`, `└─`, `│`
- 智能截断长文本
- 保持视觉层次清晰

## 配置选项

### 初始消息数量

在 `chat.go` 中定义：

```go
const maxInitialMessages = 10
```

可以根据需要调整这个值。

### 历史加载默认数量

```go
count := 20  // 默认加载20条
```

### 内容截断长度

- 参数显示: 80字符
- 结果显示: 100字符

## 用户体验改进

### Before（改进前）

1. 启动慢：一次性加载所有历史
2. 信息过载：屏幕被大量历史消息占据
3. 工具调用混乱：多个独立消息分散展示
4. 难以追踪：工具调用的因果关系不清晰

### After（改进后）

1. ✅ 快速启动：只加载最近10条
2. ✅ 清晰简洁：默认显示最相关的消息
3. ✅ 按需加载：使用 `/history` 查看更多
4. ✅ 逻辑清晰：工具调用序列合并展示
5. ✅ 视觉层次：树状结构直观展示流程

## 注意事项

1. **消息顺序**: 历史消息始终按时间顺序展示
2. **数据一致性**: `/history` 命令重新获取session数据，确保显示最新状态
3. **内存使用**: 大型session可以更高效地处理
4. **兼容性**: 新旧消息格式都支持正确显示

## 未来改进

可能的增强方向：

1. 添加消息搜索功能
2. 支持按时间范围过滤
3. 添加消息导出功能
4. 支持折叠/展开工具调用详情
5. 添加消息统计信息（工具调用次数、成功率等）
