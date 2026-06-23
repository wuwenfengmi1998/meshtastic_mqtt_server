# LLM Provider 配置热更新修复

## 问题描述

在 `/admin/llm/api` 页面修改 AI 提供商配置后，配置没有立即生效，AI 依然使用旧的提供商在工作。

## 根本原因

1. **LLM Provider 配置在程序启动时加载到内存** (`llm.State`)
2. **通过管理后台修改配置时，只更新了数据库**，内存中的配置没有更新
3. **AI 继续使用内存中的旧配置**工作

## 解决方案

### 1. 为 `llm.State` 添加动态更新方法

在 `internal/llm/state.go` 中添加了三个新方法：

- **`UpdateProvider(config ProviderConfig) error`** - 更新现有 provider 的配置
- **`AddProvider(config ProviderConfig) error`** - 添加新的 provider
- **`RemoveProvider(name string) error`** - 删除 provider

这些方法会：
- 更新内存中的配置
- 重新创建 LLM Client（使用新的 API Key、BaseURL 等）
- 管理 active provider 的切换

### 2. 在 AI Service 中暴露配置更新接口

在 `internal/ai/service.go` 中添加了：

```go
func (s *Service) ReloadLLMProvider(config interface{}) error
func (s *Service) AddLLMProvider(config interface{}) error
func (s *Service) RemoveLLMProvider(name string) error
```

这些方法接受 `interface{}` 类型（来自 HTTP handler 的 map），转换为 `llm.ProviderConfig`，然后调用 `llm.State` 的相应方法。

### 3. 在管理路由中触发配置重新加载

修改了 `internal/llmadmin/admin_llm_routes.go`：

- 添加了 `LLMProviderReloader` 接口
- `RegisterRoutes` 接受 `aiService LLMProviderReloader` 参数
- `handleCreateLLMProvider`、`handleUpdateLLMProvider`、`handleDeleteLLMProvider` 在数据库操作成功后，调用 `aiService` 的相应方法更新内存配置

### 4. 连接所有组件

修改了：
- `internal/web/web.go` - 添加 `LLMProviderReloader` 接口，并在路由注册时传递 `aiService`
- `main.go` - 将 `aiService` 传递给 web router

## 关键特性

### 线程安全
所有 `llm.State` 的更新操作都使用 `sync.RWMutex` 保护，确保并发安全。

### 容错处理
如果配置重新加载失败（例如 AI Service 未启用），不会导致请求失败。数据库已经更新，只会返回一个警告信息。

### Active Provider 管理
- 更新 provider 为 active 时，自动切换到该 provider
- 删除当前 active provider 时，自动切换到第一个可用的 provider
- 不允许删除最后一个 provider

## 测试

创建了 `internal/llm/state_test.go`，包含以下测试：
- `TestUpdateProvider` - 验证配置更新功能
- `TestAddProvider` - 验证添加新 provider
- `TestRemoveProvider` - 验证删除 provider

所有测试都通过 ✅

## 使用效果

修复后，在 `/admin/llm/api` 页面修改 AI 提供商配置时：
1. 配置保存到数据库
2. 立即更新内存中的配置
3. 创建新的 LLM Client
4. **无需重启服务**，配置立即生效

## 影响范围

### 修改的文件
- `internal/llm/state.go` - 添加配置更新方法
- `internal/ai/service.go` - 添加配置重新加载接口
- `internal/llmadmin/admin_llm_routes.go` - 在配置更新时触发重新加载
- `internal/web/web.go` - 传递 AI Service 到路由
- `main.go` - 连接组件

### 新增的文件
- `internal/llm/state_test.go` - 单元测试

### 兼容性
- 完全向后兼容
- 如果 AI Service 未启用（`aiService == nil`），配置更新仍然正常工作，只是不会触发重新加载
- 不影响现有功能

## 类似功能参考

该实现参考了项目中已有的 ToolRouter 和 TopicRouter 的配置热更新机制，它们通过 `ConfigStore` 接口在每次使用时从数据库重新加载配置。

不同之处在于：
- **ToolRouter/TopicRouter**: 每次使用时从 DB 读取（配置小，读取频率低）
- **LLM Provider**: 在配置更新时主动重新加载（Client 创建有开销，不适合频繁创建）

## 未来改进建议

1. 添加配置验证：在更新前验证 API Key、BaseURL 是否可用
2. 添加配置变更日志：记录谁在何时修改了配置
3. 支持配置回滚：保存历史版本，出问题时可以快速回滚
