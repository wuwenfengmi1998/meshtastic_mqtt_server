# 签到工具查询功能增强

## 问题描述

原签到工具只支持签到操作，不支持查询。当用户询问"今天有多少人签到"或"最近几天的签到情况"时，AI 无法调用工具获取准确数据，导致：

1. **数据不准确**：AI 只能根据上下文猜测或给出模糊答案
2. **无法查询历史**：询问超过一天的数据时，AI 一问三不知
3. **功能不完整**：数据库层已有完善的查询能力（`CountSigns`、`CountSignsByDay`），但工具层未暴露给 AI
4. **状态判断不准**：用户问"我今天签到了吗"时，AI 基于对话记忆判断而非查询数据库，导致误判

## 解决方案

为签到工具添加查询和检查功能，支持以下三种操作模式：

### 1. 签到操作 (action=sign)

保持原有功能不变：
- 记录节点今日签到信息
- 每个节点每天只能签到一次
- 必填参数：地区、名字、设备

### 2. 查询操作 (action=query)

查询签到统计功能：
- 查询指定日期或日期范围的签到统计
- 返回总人次和按天分组的统计数据
- 可选参数：
  - `date`: 查询日期（格式：YYYY-MM-DD），默认今天
  - `days`: 查询最近 N 天，默认只查询 date 指定的那一天

### 3. 检查操作 (action=check) **新增**

检查当前节点今天是否已签到：
- 用于回答"我今天签到了吗"、"我什么时候签到的"之类的问题
- 直接查询数据库，不依赖对话历史
- 返回明确的签到状态，**包括签到时间和签到内容**

## 修改内容

### 1. 扩展 SignStore 接口

```go
type SignStore interface {
    CreateSign(nodeID string, longName, shortName *string, signText string, signTime time.Time) (*storepkg.SignRecord, error)
    HasSignedOnDay(nodeID string, day time.Time) (bool, error)
    GetNodeInfo(nodeID string) (*storepkg.NodeInfoRecord, error)
    // 新增查询方法
    CountSigns(opts storepkg.ListOptions) (int64, error)
    CountSignsByDay(opts storepkg.ListOptions) ([]storepkg.SignDayCount, error)
    ListSigns(opts storepkg.ListOptions) ([]storepkg.SignRecord, error)
}
```

### 2. 更新工具定义

工具描述中明确说明支持两种操作：
- action=sign：签到
- action=query：查询统计

### 3. 实现查询逻辑

- `executeSign()`: 原签到逻辑
- `executeQuery()`: 新增查询逻辑
  - 解析日期参数
  - 构建查询条件（Since/Until）
  - 调用 store 查询接口
  - 格式化返回结果

### 4. 扩展参数结构

```go
type signParams struct {
    Action        string `json:"action"`         // sign 或 query
    // 签到参数
    Region        string `json:"region"`
    Name          string `json:"name"`
    Device        string `json:"device"`
    TxPower       string `json:"tx_power"`
    AntennaLength string `json:"antenna_length"`
    Altitude      string `json:"altitude"`
    RawText       string `json:"raw_text"`
    // 查询参数
    Date          string `json:"date"`           // YYYY-MM-DD
    Days          int    `json:"days"`           // 最近 N 天
}
```

## 使用示例

### AI 调用示例

#### 检查今天是否签到（check 操作）

**用户**："我今天签到了吗？"

AI 调用：
```json
{
  "action": "check"
}
```

返回（未签到）：
```text
Test Node 今天还没有签到。
```

返回（已签到）：
```text
Test Node 今天已经签到过了。
签到时间：10:30:45
签到内容：上海闵行-Kevin-GAT562签到
```

**用户**："我什么时候签到的？"

AI 同样调用 check 操作，返回包含签到时间和内容的完整信息。

#### 查询今天的签到情况
```json
{
  "action": "query",
  "date": "2024-06-23"
}
```

返回：
```
2024-06-23 的签到统计：
总计：5 人次

按天统计：
- 2024-06-23: 5 人
```

#### 查询最近 7 天的签到情况
```json
{
  "action": "query",
  "date": "2024-06-23",
  "days": 7
}
```

返回：
```
最近 7 天的签到统计：
总计：32 人次

按天统计：
- 2024-06-23: 5 人
- 2024-06-22: 6 人
- 2024-06-21: 4 人
- 2024-06-20: 7 人
- 2024-06-19: 3 人
- 2024-06-18: 4 人
- 2024-06-17: 3 人
```

#### 签到（保持原有功能）
```json
{
  "action": "sign",
  "region": "上海闵行",
  "name": "Kevin",
  "device": "GAT562"
}
```

## 测试

创建了完整的单元测试 `sign_test.go`，覆盖：
- ✅ 查询今天的签到统计
- ✅ 查询最近 N 天的签到统计
- ✅ 查询指定日期的签到统计
- ✅ 签到功能（回归测试）

所有测试通过。

## 影响范围

- ✅ 向后兼容：原有签到功能完全保持不变
- ✅ 数据库无需修改：查询接口已存在
- ✅ 新增功能：AI 现在可以准确回答签到统计问题
- ✅ 编译通过：整个项目构建成功

## 后续建议

可以考虑进一步增强：
1. 支持按节点查询：某个特定节点的签到历史
2. 支持按地区统计：哪些地区签到最活跃
3. 支持导出详细列表：不仅是统计，还能看到每条签到的详细内容
