# Readline功能测试说明

## 新增功能

### 1. 中文输入支持
使用readline库替换了bufio.Scanner，提供完整的Unicode/UTF-8支持：
- ✅ 中文字符正确显示
- ✅ Backspace正确删除中文字符（每次删除一个完整字符）
- ✅ 左右箭头在中文和英文字符间正确移动

### 2. 命令自动补全
按下TAB键时，会自动补全斜杠命令：
- `/new` - 开始新会话
- `/status` - 显示状态
- `/model` - 切换模型
- `/help` - 显示帮助
- `/exit` - 退出

### 3. 输入历史
- 使用上下箭头浏览历史输入
- 历史记录保存在 `/tmp/agenty-chat-history.txt`
- 跨会话保持历史

### 4. 高级编辑功能
- Ctrl+A: 移动到行首
- Ctrl+E: 移动到行尾
- Ctrl+K: 删除光标到行尾
- Ctrl+U: 删除光标到行首
- Ctrl+W: 删除前一个单词
- Ctrl+L: 清屏
- Ctrl+C: 中断当前输入

## 测试步骤

### 测试1: 中文输入和删除
```
You: 你好世界
（按几次Backspace，每次应该删除一个完整的中文字符）
You: 你好

🤖 Assistant: ...
```

### 测试2: 命令补全
```
You: /n[按TAB]
→ 自动补全为 "/new"

You: /s[按TAB]
→ 自动补全为 "/status"

You: /[按TAB两次]
→ 显示所有可用命令
```

### 测试3: 历史记录
```
You: 第一条消息
You: 第二条消息
You: 第三条消息
[按向上箭头]
→ 显示 "第三条消息"
[再按向上箭头]
→ 显示 "第二条消息"
```

### 测试4: 行编辑
```
You: 这是一个很长的消息[Ctrl+A]
→ 光标移动到行首
[Ctrl+E]
→ 光标移动到行尾
[Ctrl+K]
→ 删除光标到行尾的内容
```

## 技术细节

### Readline配置
```go
&readline.Config{
    Prompt:          "You: ",           // 提示符
    HistoryFile:     "/tmp/...",        // 历史文件
    AutoComplete:    completer,         // 自动补全器
    InterruptPrompt: "^C",             // 中断提示
    EOFPrompt:       "exit",           // EOF提示
    HistorySearchFold: true,           // 历史搜索
    FuncFilterInputRune: func(r rune) (rune, bool) {
        return r, true                  // 接受所有字符
    },
}
```

### 补全器实现
使用`readline.NewPrefixCompleter`创建命令补全：
- 自动匹配以`/`开头的输入
- 提供所有斜杠命令的补全建议
- 支持部分匹配

## 已知限制

1. **颜色提示符**: readline可能不完全支持pterm的彩色提示符，在某些终端中可能显示异常
2. **历史文件位置**: 当前使用`/tmp`目录，系统重启后会丢失
3. **Windows兼容性**: readline在Windows上支持有限

## 改进建议

1. 将历史文件移到用户主目录（`~/.agenty/history`）
2. 添加历史搜索功能（Ctrl+R）
3. 考虑为不同session使用不同的历史文件
4. 添加更多编辑快捷键的帮助说明
