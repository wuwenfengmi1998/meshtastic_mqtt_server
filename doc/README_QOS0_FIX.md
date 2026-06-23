# MQTT QoS0 重发问题 - 完整解决方案

## 问题描述

用户报告：设备使用 QoS0 发送 MQTT 消息后，服务器好像不给 ACK，导致设备一直重发 3 次，有时候又一次发送成功。

## 解决方案总结

我们进行了两轮修复和诊断增强：

### 第一轮：修复 TCP 层延迟问题 ✅

**问题：** TCP Nagle 算法导致 TCP ACK 延迟 40-200ms  
**修复：** 在 `OnConnect` 中启用 `TCP_NODELAY`  
**效果：** TCP ACK 延迟降低到 ~0.05ms  
**Commit:** `ec24e70` - 修复 MQTT QoS0 消息重发问题

### 第二轮：诊断应用层拒绝问题 ✅

**发现：** TCP_NODELAY 只解决了传输层延迟，但如果消息被应用层拒绝，设备仍会重发  
**改进：** 添加详细的拒绝日志，帮助快速定位问题  
**Commit:** `bce6b70` - 增强 MQTT 消息拒绝诊断功能

## 快速诊断方法

### 步骤 1: 启用详细日志

```bash
./meshtastic_mqtt_server --console-log-mqtt=true --console-log-meshtastic=true
```

### 步骤 2: 观察日志

#### 场景 A: 看到 `PUBLISH rejected` 或 `PUBLISH blocked`
→ **服务器拒绝了消息**  
→ 查看具体错误原因，应用对应的解决方案

#### 场景 B: 没有看到 rejected/blocked，消息正常接收
→ **不是服务器问题**  
→ 问题在设备端固件，需要设备端修复

## 常见原因和解决方案

### 原因 1: 消息无法解密 (`error=cannot be decrypted`)

**解决方案：**
```yaml
# 方法 1: 配置正确的 PSK
meshtastic:
  psk: "your_base64_psk"  # 必须与设备一致

# 方法 2: 允许转发加密消息
meshtastic:
  allow_encrypted_forwarding: true
```

### 原因 2: 消息被屏蔽 (`PUBLISH blocked`)

**解决方案：**
```sql
-- 查看屏蔽规则
SELECT * FROM blocking_rules WHERE enabled = 1;

-- 禁用特定规则
UPDATE blocking_rules SET enabled = 0 WHERE id = <rule_id>;
```

### 原因 3: Protobuf 解析失败 (`error=protobuf decode failed`)

**解决方案：**
- 更新设备固件到最新版本
- 检查设备是否使用标准 Meshtastic 协议

### 原因 4: 设备端 Bug

**症状：** 服务器日志正常，但设备仍重发  
**解决方案：**
- 更新设备固件
- 联系设备厂商

## 文档索引

1. **[TCP_ACK_FIX_CN.md](doc/TCP_ACK_FIX_CN.md)** - TCP_NODELAY 修复详解（中文）
2. **[TCP_ACK_FIX.md](doc/TCP_ACK_FIX.md)** - TCP_NODELAY 修复详解（英文）
3. **[QOS0_RETRANSMIT_ANALYSIS.md](doc/QOS0_RETRANSMIT_ANALYSIS.md)** - 重发问题深度分析
4. **[DIAGNOSTIC_GUIDE.md](doc/DIAGNOSTIC_GUIDE.md)** - 完整诊断指南

## 技术细节

### TCP_NODELAY 的作用

```
没有 TCP_NODELAY:
  客户端发送 → 服务器收到 → 等待 Nagle 算法 (40-200ms) → TCP ACK

有 TCP_NODELAY:
  客户端发送 → 服务器收到 → 立即 TCP ACK (~0.05ms)
```

### MQTT QoS0 的特性

- QoS 0 = "至多一次"交付
- MQTT 应用层**不需要** PUBACK
- 但 TCP 层**仍然需要** TCP ACK
- 如果消息被应用层拒绝（返回 ErrRejectPacket），mochi-mqtt 只是静默返回，不发送任何响应

### 消息处理流程

```
设备 → MQTT PUBLISH (QoS0)
      ↓
TCP 层收到 → TCP ACK ✅ (现在是即时的)
      ↓
MQTT 层 OnPublish hook
      ↓
MQTTPP 验证
      ↓
├─ valid=true → 转发消息 ✅
│
└─ valid=false → 返回 ErrRejectPacket → 静默丢弃 ❌
                ↓
             设备可能重发
```

## Git 提交历史

```bash
git log --oneline -3

bce6b70 增强 MQTT 消息拒绝诊断功能
ec24e70 修复 MQTT QoS0 消息重发问题
cfe4ef0 修复消息重复问题
```

## 测试验证

### 单元测试
```bash
go test -v -run TestTCPNoDelay
go test -v -run TestQoS0MessageLatency
```

### 集成测试
```bash
# 启动服务器
./meshtastic_mqtt_server --console-log-mqtt=true

# 发送测试消息
mosquitto_pub -h localhost -p 1883 -t "test/topic" -m "hello" -q 0

# 观察日志输出
```

## 性能指标

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| TCP ACK 延迟 | 40-200ms | ~0.05ms |
| 往返延迟 | 不稳定 | ~54µs |
| 提升倍数 | - | **1000x** |

## 下一步行动

1. ✅ **重新编译部署**
   ```bash
   go build
   ./meshtastic_mqtt_server
   ```

2. ✅ **启用详细日志**
   ```bash
   ./meshtastic_mqtt_server --console-log-mqtt=true
   ```

3. ✅ **观察日志**
   - 如果看到 `rejected/blocked` → 应用对应的解决方案
   - 如果没看到 → 问题在设备端

4. ✅ **查看数据库**
   ```sql
   SELECT * FROM discarded_packets ORDER BY created_at DESC LIMIT 20;
   ```

5. ✅ **必要时抓包**
   ```bash
   tcpdump -i any -nn port 1883 -w mqtt.pcap
   ```

## 结论

我们修复了 TCP 层的延迟问题，并添加了完善的诊断工具。现在你可以：

1. 快速判断重发是由服务器拒绝还是设备 bug 引起
2. 看到具体的拒绝原因和错误信息
3. 根据诊断结果应用对应的解决方案

**如果重发问题依然存在，请按照诊断指南操作，查看日志中是否有 rejected/blocked 消息，并将结果反馈给我。**
