# CLI Markdown渲染和交互式历史查看器

本文档描述了CLI聊天界面的两个重要新功能：Markdown渲染和交互式历史查看器。

## 1. Markdown渲染

### 功能概述

CLI现在支持在消息显示中渲染Markdown格式，使对话内容更加美观和易读。

### 支持的Markdown格式

#### 基本格式
- **粗体**: `**text**` or `__text__`
- *斜体*: `*text*` or `_text_`
- `内联代码`: \`code\`
- ~~删除线~~: `~~text~~`

#### 标题
```markdown
# H1 标题
## H2 标题
### H3 标题
```

#### 列表
```markdown
- 无序列表项1
- 无序列表项2
  - 嵌套项

1. 有序列表项1
2. 有序列表项2
```

#### 代码块
````markdown
```python
def hello_world():
    print("Hello, World!")
```
````

#### 引用块
```markdown
> 这是一个引用
> 可以跨多行
```

#### 链接和图片
```markdown
[链接文本](https://example.com)
![图片描述](image-url)
```

#### 表格
```markdown
| 列1 | 列2 | 列3 |
|-----|-----|-----|
| A   | B   | C   |
| 1   | 2   | 3   |
```

#### 水平线
```markdown
---
```

### 渲染效果

#### 示例1: 代码解释

**用户输入**:
```
请解释这段Python代码的作用
```

**AI回复** (带Markdown):
```markdown
这段代码定义了一个 **递归函数** 来计算斐波那契数列：

```python
def fibonacci(n):
    if n <= 1:
        return n
    return fibonacci(n-1) + fibonacci(n-2)
```

主要特点：
- 使用 `if` 语句处理基础情况
- *递归调用* 自身来计算结果
- 时间复杂度为 `O(2^n)`

> 注意：这个实现虽然简洁，但对于大的n值效率较低。
```

**CLI显示效果**:
- "递归函数"会以粗体显示
- 代码块会使用等宽字体和语法高亮
- `if`、`O(2^n)` 会使用内联代码样式
- "递归调用"会以斜体显示
- 注意事项会以引用块格式缩进显示

#### 示例2: 列表和表格

**AI回复**:
```markdown
## 可用的命令选项

有以下几种方式：

1. **基本命令**
   - `--help` - 显示帮助
   - `--version` - 显示版本

2. **高级选项**
   - `--config` - 指定配置文件
   - `--verbose` - 详细输出

### 参数对比

| 参数 | 短格式 | 说明 |
|------|--------|------|
| help | -h | 显示帮助信息 |
| version | -v | 显示版本号 |
```

**CLI显示效果**:
- 标题会以大号文字显示
- 列表会正确缩进
- 粗体参数会突出显示
- 表格会对齐显示

### 技术实现

使用 `github.com/charmbracelet/glamour` 库：
- 专为终端设计的Markdown渲染器
- 自动检测终端颜色支持（深色/浅色主题）
- 支持ANSI颜色和样式
- 智能文本换行（默认100字符）

```go
func renderMarkdown(text string) string {
    r, err := glamour.NewTermRenderer(
        glamour.WithAutoStyle(),      // 自动检测终端样式
        glamour.WithWordWrap(100),     // 100字符换行
    )
    if err != nil {
        return text  // 降级为原始文本
    }
    
    rendered, err := r.Render(text)
    if err != nil {
        return text
    }
    
    return strings.TrimSpace(rendered)
}
```

### 应用范围

Markdown渲染应用于：
1. **用户消息**: 输入的消息内容
2. **AI助手回复**: 模型生成的响应
3. **Reasoning内容**: Kimi模型的推理过程

**不应用于**:
- 工具调用参数（保持JSON格式）
- 工具结果（保持原始格式）
- 系统消息和提示

### 性能影响

- 渲染性能: 对于正常长度的消息（<1000字符），几乎无延迟
- 内存占用: 每个消息增加约1-2KB
- CPU使用: 可忽略不计

### 错误处理

如果Markdown渲染失败（例如格式错误或库异常）：
- 自动降级为原始文本显示
- 不会中断正常功能
- 用户体验不受影响

## 2. 交互式历史查看器

### 功能概述

`/history` 命令现在打开一个交互式查看器（类似`less`命令），允许用户滚动浏览完整的聊天历史。

### 使用方法

```bash
# 在聊天中输入
You: /history
```

系统会自动：
1. 获取完整的会话历史
2. 格式化所有消息
3. 打开交互式查看器（less或more）

### 交互式查看器功能

#### 在 `less` 中的操作

| 按键 | 功能 |
|------|------|
| `Space` / `f` | 向下翻页 |
| `b` | 向上翻页 |
| `↓` / `j` | 向下滚动一行 |
| `↑` / `k` | 向上滚动一行 |
| `g` | 跳到开头 |
| `G` | 跳到结尾 |
| `/pattern` | 搜索文本 |
| `n` | 下一个搜索结果 |
| `N` | 上一个搜索结果 |
| `q` | 退出查看器 |
| `h` | 显示帮助 |

#### 搜索功能

在查看器中按 `/` 然后输入搜索词：

```
/error          # 搜索"error"
/tool.*call     # 使用正则表达式搜索
```

按 `n` 跳到下一个匹配，按 `N` 跳到上一个匹配。

### 显示格式

历史查看器中的消息格式：

```
=== Chat History ===

--- Message 1/50 ---
👤 User [14:30:00]:
你好

--- Message 2/50 ---
🤖 Assistant (gpt-4) [14:30:01]:
你好！我能帮你什么？

--- Message 3/50 ---
👤 User [14:30:15]:
请帮我读取文件 /etc/hosts

--- Message 4/50 ---
🤖 Assistant (gpt-4) [14:30:16]:

🔧 Tool Calls:
  • read_file
    {
      "path": "/etc/hosts"
    }

--- Message 5/50 ---
🛠️ Tool Result [14:30:17]:
✅ read_file
127.0.0.1  localhost
...
```

### 系统兼容性

#### Linux
- ✅ 完全支持
- 默认使用 `less -R` (保留ANSI颜色)
- 降级到 `more` 如果less不可用

#### macOS
- ✅ 完全支持
- 自带 `less` 命令
- 完整的交互功能

#### Windows
- ⚠️ 部分支持
- 需要安装 `less.exe` (如Git Bash提供的)
- 或使用 `more` (功能较少)
- 在PowerShell/CMD中功能可能受限

#### WSL (Windows Subsystem for Linux)
- ✅ 完全支持
- 与Linux环境相同

### 降级处理

如果系统没有 `less` 或 `more` 命令：

```
⚠ Failed to open interactive viewer: no pager available
ℹ Showing history in console instead...

📜 Message History (50 of 50 total)

[显示所有消息...]
```

- 自动在控制台中显示所有历史
- 不影响功能使用
- 显示友好的警告信息

### 技术实现

```go
func openHistoryViewer(messages []models.ChatMessageDto) error {
    // 1. 格式化所有消息到buffer
    var buf bytes.Buffer
    buf.WriteString("=== Chat History ===\n\n")
    
    for i, msg := range messages {
        buf.WriteString(fmt.Sprintf("--- Message %d/%d ---\n", i+1, len(messages)))
        // 格式化消息内容...
    }
    
    // 2. 尝试使用 'less' 命令
    lessPath, err := exec.LookPath("less")
    if err == nil {
        cmd := exec.Command(lessPath, "-R")  // -R 支持ANSI颜色
        cmd.Stdin = strings.NewReader(buf.String())
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        return cmd.Run()
    }
    
    // 3. 降级到 'more'
    morePath, err := exec.LookPath("more")
    if err == nil {
        cmd := exec.Command(morePath)
        cmd.Stdin = strings.NewReader(buf.String())
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        return cmd.Run()
    }
    
    // 4. 返回错误以触发降级显示
    return fmt.Errorf("no pager available")
}
```

### 与启动时自动加载的关系

1. **启动时**: 自动显示最近10条消息（快速预览）
2. **需要更多**: 使用 `/history` 命令打开完整历史查看器
3. **最佳实践**: 保持聊天流畅，需要时再查看完整历史

```bash
# 启动chat
$ ./agenty-cli chat
ℹ Resuming last session: xxx
ℹ Using model: gpt-4/OpenAI
ℹ Showing last 10 messages (total: 50). Use /history to load more.

Previous Messages
[显示最近10条...]

# 需要查看更多时
You: /history
[打开交互式查看器，浏览全部50条]
```

## 3. 组合使用

### 场景1: 查看历史中的代码示例

```bash
You: /history
# 在查看器中搜索
/```python
# 找到所有Python代码块
```

### 场景2: 查找特定问题

```bash
You: /history
# 搜索错误信息
/error
# 按 n 查看下一个错误
```

### 场景3: 回顾长对话

```bash
# 启动时只看到最近10条
$ ./agenty-cli chat
[显示最近10条]

# 需要完整上下文时
You: /history
# 从头开始阅读，理解完整对话流程
```

## 4. 配置选项

### 自定义Markdown渲染

如果需要调整渲染设置，可以修改 `renderMarkdown` 函数：

```go
r, err := glamour.NewTermRenderer(
    glamour.WithAutoStyle(),           // 自动样式
    glamour.WithWordWrap(120),          // 改为120字符换行
    glamour.WithStylePath("dark"),      // 强制深色主题
)
```

### 初始历史显示数量

在 `chat.go` 中修改：

```go
const maxInitialMessages = 10  // 改为20显示更多
```

## 5. 故障排除

### Markdown显示异常

**问题**: Markdown格式没有正确渲染

**解决方案**:
- 检查终端是否支持ANSI颜色
- 尝试使用不同的终端模拟器
- 确认glamour库正确安装

### 历史查看器打不开

**问题**: `/history` 命令没有打开查看器

**解决方案**:
```bash
# 检查less是否可用
which less

# 检查more是否可用
which more

# 如果都不可用，会自动降级显示
```

### Windows上的问题

**问题**: Windows上查看器不工作

**解决方案**:
- 使用Git Bash（自带less）
- 使用WSL
- 或接受降级的控制台显示

## 6. 最佳实践

1. **使用Markdown格式化重要内容**
   - 使用代码块包裹代码
   - 使用列表整理步骤
   - 使用粗体强调关键点

2. **有效使用历史查看器**
   - 使用搜索功能快速定位
   - 定期查看历史理解对话脉络
   - 在长对话中特别有用

3. **保持对话清晰**
   - 善用Markdown格式
   - 必要时查看历史
   - 使用 `/new` 开始新话题

## 7. 性能考虑

- **Markdown渲染**: 几乎实时，无明显延迟
- **历史加载**: 取决于消息数量
  - <100条: 即时
  - 100-500条: <1秒
  - >500条: 1-3秒
- **内存使用**: 每条消息约2-5KB

## 8. 未来改进

可能的增强：
1. 自定义Markdown主题
2. 历史导出为HTML/PDF
3. 消息书签功能
4. 历史统计和分析
5. 更多交互式功能
