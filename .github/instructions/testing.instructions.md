# 测试指令

本文件包含针对测试代码的指令和最佳实践。

## 测试原则

- 所有新功能必须包含单元测试
- 测试应该是独立的，可以按任意顺序运行
- 测试应该快速且可靠
- 使用有意义的测试名称描述测试内容

## Go 测试最佳实践

### 测试文件组织

- 测试文件与被测试的代码放在同一包中
- 测试文件以 `_test.go` 结尾
- 测试函数以 `Test` 开头，后跟被测试函数名

### 表驱动测试

使用表驱动测试方法测试多个场景：

```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid input",
            input:    "test",
            expected: "TEST",
            wantErr:  false,
        },
        {
            name:     "empty input",
            input:    "",
            expected: "",
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := Function(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Function() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if result != tt.expected {
                t.Errorf("Function() = %v, want %v", result, tt.expected)
            }
        })
    }
}
```

### Mock 和依赖注入

- 使用接口定义依赖
- 使用 mock 对象隔离外部依赖
- 可以使用 `go.uber.org/mock` 生成 mock

### 测试覆盖率

- 运行 `go test -cover ./...` 查看覆盖率
- 生成覆盖率报告：`go test -coverprofile=coverage.out ./...`
- 查看详细报告：`go tool cover -html=coverage.out`

## HTTP 处理器测试

使用 `httptest` 包测试 HTTP 处理器：

```go
func TestHandler(t *testing.T) {
    router := gin.New()
    router.GET("/test", Handler)

    req := httptest.NewRequest("GET", "/test", nil)
    w := httptest.NewRecorder()
    
    router.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
    }
}
```

## 数据库测试

- 使用内存数据库或测试数据库
- 在测试前后清理数据
- 使用事务隔离测试

## 集成测试

- 集成测试应该在单独的文件中，使用构建标签
- 使用 `// +build integration` 标签
- 运行时使用 `go test -tags=integration ./...`

## 注意事项

- 不要在测试中使用 sleep，使用适当的同步机制
- 避免测试实现细节，测试行为而不是实现
- 不要忽略测试失败，及时修复
- 保持测试代码的质量与生产代码一致
