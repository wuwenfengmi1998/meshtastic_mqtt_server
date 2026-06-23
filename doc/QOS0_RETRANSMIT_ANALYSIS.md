# QoS0 消息重发问题深度分析

## 问题现状

即使启用了 TCP_NODELAY，设备发送 QoS0 消息后仍然重发 3 次。

## 根本原因分析

### 1. TCP_NODELAY 修复了什么？

✅ TCP_NODELAY 确实解决了 **TCP ACK 延迟**问题：
- Nagle 算法延迟从 40-200ms 降低到 ~0.05ms
- TCP 层的确认现在是即时的

❌ 但这**不能解决消息被拒绝的问题**。

### 2. 消息被拒绝的流程

当设备发送的消息不符合服务器要求时：

```
设备 → MQTT PUBLISH (QoS0)
      ↓
服务器 TCP 层收到 → 发送 TCP ACK ✅
      ↓
MQTT 层处理 → OnPublish hook
      ↓
MQTTPP 验证失败 → valid=false
      ↓
返回 packets.ErrRejectPacket
      ↓
mochi-mqtt 处理: return nil (不发送任何 MQTT 响应)
      ↓
设备收到 TCP ACK ✅ 但没有收到 MQTT 层响应
      ↓
设备认为消息可能丢失 → 重发 ❌
```

### 3. 为什么会重发？

可能的原因：

#### 原因 A: 消息验证失败

检查以下验证失败的情况：

1. **Protobuf 解码失败**
   ```
   parseServiceEnvelope() 返回错误
   → MQTTPP 返回 valid=false
   ```

2. **解密失败**
   ```
   describePacket() 无法解密
   → type="encrypted_packet" 且 AllowEncryptedForwarding=false
   → MQTTPP 返回 valid=false
   ```

3. **屏蔽规则命中**
   ```
   blockingViolationForRecord() 返回非 nil
   → OnPublish 返回 ErrRejectPacket
   ```

#### 原因 B: 设备期待应用层响应

某些 MQTT 客户端实现可能：
- 虽然使用 QoS0（不需要 PUBACK）
- 但仍然期待某种应用层响应或订阅回显
- 没有收到预期响应时触发重试逻辑

#### 原因 C: 设备端 Bug

设备固件可能有 bug：
- 错误地认为 QoS0 需要应用层确认
- 超时机制设置不当
- 重试逻辑实现错误

## 诊断步骤

### 步骤 1: 查看服务器日志

检查消息是否被拒绝：

```bash
# 启用控制台日志
./meshtastic_mqtt_server --console-log-mqtt=true --console-log-meshtastic=true

# 查找被拒绝的消息
grep -E "error|dropped|rejected" logs.txt
```

**关键日志标识：**
- `protobuf decode failed` - protobuf 解析失败
- `cannot be decrypted` - 解密失败
- `blocked node` / `forbidden word` - 屏蔽规则命中

### 步骤 2: 抓包分析

```bash
# 抓取 MQTT 流量
tcpdump -i any -nn port 1883 -w mqtt.pcap

# 用 Wireshark 分析:
# 1. 查看是否有 TCP 重传 (Retransmission)
# 2. 查看 MQTT PUBLISH 是否有对应的响应
# 3. 检查时序图，看设备重发的时间间隔
```

**期待的正常流程 (QoS0)：**
```
Client → Server: MQTT PUBLISH (QoS0)
Server → Client: TCP ACK
(没有 MQTT 层的 PUBACK，因为是 QoS0)
```

**如果消息被拒绝：**
```
Client → Server: MQTT PUBLISH (QoS0)
Server → Client: TCP ACK
(服务器静默丢弃，没有任何 MQTT 响应)
Client → Server: MQTT PUBLISH (QoS0) [重发]
Server → Client: TCP ACK
...
```

### 步骤 3: 检查数据库

```sql
-- 查看被丢弃的消息
SELECT * FROM discarded_packets 
ORDER BY created_at DESC 
LIMIT 20;

-- 统计丢弃原因
SELECT 
    json_extract(record, '$.error') as error_type,
    COUNT(*) as count
FROM discarded_packets
GROUP BY error_type;
```

### 步骤 4: 测试不同的消息

```bash
# 发送一个有效的测试消息
mosquitto_pub -h localhost -p 1883 -t "msh/CN/2/e/LongFast/!12345678" -m "test" -q 0

# 观察是否也会重发
```

## 解决方案

### 方案 1: 修复消息验证问题

如果是消息验证失败导致：

**检查 PSK 配置：**
```bash
# 确保服务器配置了正确的 PSK
./meshtastic_mqtt_server --psk="your_base64_psk"
```

**检查屏蔽规则：**
```sql
-- 查看当前的屏蔽规则
SELECT * FROM blocking_rules WHERE enabled = 1;

-- 临时禁用所有规则测试
UPDATE blocking_rules SET enabled = 0;
```

### 方案 2: 允许加密消息转发

如果消息是加密的且无法解密：

```yaml
# config.yaml
meshtastic:
  allow_encrypted_forwarding: true
```

这样即使无法解密，消息也会被转发而不是拒绝。

### 方案 3: 返回明确的错误响应（不推荐）

理论上可以在消息被拒绝时返回 MQTT 错误码，但：
- ❌ QoS 0 协议规定不应该有 PUBACK
- ❌ 违反 MQTT 规范
- ❌ 可能导致客户端行为异常

### 方案 4: 设备端修复

如果是设备固件 bug：
- 更新设备固件到最新版本
- 检查设备日志，确认重发原因
- 联系设备厂商报告 bug

## 监控和调试

### 添加详细日志

修改 `main.go` 的 `OnPublish` 方法：

```go
func (h *meshtasticFilterHook) OnPublish(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
    valid, _, record := mqtpp.MQTTPP(pk.TopicName, pk.Payload, h.key, mqtpp.Options{
        AllowEncryptedForwarding: h.settings.AllowEncryptedForwarding(),
        PKIKeyResolver:           h.pkiResolver,
    })
    
    info := mqttClientInfoFromClient(cl)
    
    if !valid {
        // 添加详细日志
        printJSON(map[string]any{
            "event": "publish_rejected",
            "reason": "validation_failed",
            "client_id": info.ClientID,
            "topic": pk.TopicName,
            "payload_len": len(pk.Payload),
            "error": record["error"],
        })
        h.rejectPublish(cl, pk, record)
        return pk, packets.ErrRejectPacket
    }
    
    // ... 其他逻辑
}
```

### 监控重发率

```sql
-- 创建视图统计每个客户端的重发率
CREATE VIEW client_retransmit_stats AS
SELECT 
    client_id,
    COUNT(*) as total_attempts,
    COUNT(DISTINCT packet_id) as unique_packets,
    (COUNT(*) - COUNT(DISTINCT packet_id)) * 100.0 / COUNT(*) as retransmit_rate
FROM packets
GROUP BY client_id
HAVING retransmit_rate > 10;
```

## 结论

TCP_NODELAY 修复了 TCP 层的延迟问题，但如果消息本身被服务器拒绝（验证失败、解密失败、屏蔽规则等），设备仍然会重发。

**下一步行动：**

1. ✅ 启用详细日志，查看是否有消息被拒绝
2. ✅ 检查 `discarded_packets` 表，确认拒绝原因
3. ✅ 抓包分析，确认是 TCP 重传还是应用层重发
4. ✅ 根据诊断结果选择对应的解决方案

**如果所有消息都被正常处理（没有被拒绝），但仍然重发，那么问题在设备端固件。**
