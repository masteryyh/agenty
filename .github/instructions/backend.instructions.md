# 后端开发指令

本文件包含针对后端 Go 代码开发的特定指令。

## Go 后端最佳实践

### 包组织

- 每个包应该有单一、明确的职责
- 包名应该简洁、有意义，全部小写，无下划线
- 避免循环依赖

### 错误处理

- 使用自定义错误类型（见 `pkg/customerrors`）
- 错误应该在适当的层级被处理或包装
- 不要忽略错误，即使是在 defer 语句中
- 使用 `errors.Is()` 和 `errors.As()` 进行错误检查

### 并发

- 使用 goroutines 和 channels 进行并发操作
- 使用 `sync.Once` 确保单例初始化的线程安全
- 使用 context 进行超时和取消控制
- 避免 goroutine 泄漏，确保所有 goroutines 都能正常退出

### 数据库操作

- 使用 GORM 作为 ORM
- 使用泛型助手 G[] 进行类型安全的查询
- 始终处理事务错误
- 使用连接池管理数据库连接
- 对敏感查询使用预处理语句防止 SQL 注入

### HTTP 处理

- 使用 Gin 框架处理 HTTP 请求
- 使用中间件处理横切关注点（日志、认证、CORS 等）
- 正确使用 HTTP 状态码
- 使用 `pkg/utils/response` 包提供统一的响应格式
- 验证所有输入数据

### 配置管理

- 使用 Viper 管理配置
- 敏感配置通过环境变量提供
- 配置文件应该有清晰的结构和文档
- 提供合理的默认值

## 服务层架构

- 服务应该实现业务逻辑
- 使用依赖注入提高可测试性
- 服务应该是无状态的或使用单例模式
- 分离关注点：路由 -> 控制器 -> 服务 -> 数据库

## 示例代码模式

### 服务初始化（单例模式）

```go
var (
    serviceInstance *Service
    serviceOnce     sync.Once
)

func GetService() *Service {
    serviceOnce.Do(func() {
        serviceInstance = &Service{}
    })
    return serviceInstance
}
```

### 错误处理

```go
if err != nil {
    return customerrors.Wrap(err, "operation failed")
}
```

### GORM 查询

```go
var results []Model
if err := db.Where("status = ?", status).Find(&results).Error; err != nil {
    return nil, err
}
```

## 测试

- 为所有公共函数编写单元测试
- 使用表驱动测试方法
- 使用 mock 隔离外部依赖
- 测试边缘情况和错误路径
