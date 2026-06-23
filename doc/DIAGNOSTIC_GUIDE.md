# QoS0 重发问题诊断指南

## 快速诊断

### 1. 启用详细日志运行服务器

```bash
./meshtastic_mqtt_server --console-log-mqtt=true --console-log-meshtastic=true
```

### 2. 观察日志输出

#### ✅ 正常接收的消息
```
[mqtt] connect    client_id=device123 username=user1 remote=192.168.1.100:54321
text       from=!12345678 channel=LongFast text="hello world"
```

#### ❌ 被拒绝的消息（关键！）
```
[mqtt] PUBLISH rejected: client_id=device123 topic=msh/CN/2/e/LongFast/!12345678 qos=0 payload_len=156 error=protobuf decode failed
```

#### ❌ 被屏蔽的消息
```
[mqtt] PUBLISH blocked: client_id=device123 topic=msh/CN/2/e/LongFast/!12345678 type=forbidden_word reason=blocked node
```

### 3. 根据日志判断问题

| 日志内容 | 问题原因 | 解决方法 |
|---------|---------|---------|
| `PUBLISH rejected: error=protobuf decode failed` | 消息格式错误 | 检查设备固件版本 |
| `PUBLISH rejected: error=cannot be decrypted` | 无法解密 | 检查 PSK 配置或启用 `allow_encrypted_forwarding` |
| `PUBLISH blocked: type=node` | 节点被屏蔽 | 检查屏蔽规则 |
| `PUBLISH blocked: type=forbidden_word` | 内容被屏蔽 | 检查关键词过滤规则 |
| **没有 rejected/blocked 日志** | **不是服务器拒绝** | **问题在设备端** |

## 详细诊断步骤

### 步骤 1: 检查数据库中被拒绝的消息

```bash
# 进入数据库
sqlite3 /path/to/database.db

# 查看最近被拒绝的消息
SELECT 
    datetime(created_at, 'unixepoch', 'localtime') as time,
    client_id,
    json_extract(record, '$.error') as error,
    json_extract(record, '$.topic') as topic,
    payload_len
FROM discarded_packets
ORDER BY created_at DESC
LIMIT 20;

# 统计拒绝原因
SELECT 
    json_extract(record, '$.error') as error_type,
    COUNT(*) as count
FROM discarded_packets
WHERE created_at > strftime('%s', 'now', '-1 hour')
GROUP BY error_type;
```

### 步骤 2: 抓包分析

```bash
# 开始抓包
sudo tcpdump -i any -nn port 1883 -w /tmp/mqtt_traffic.pcap

# 让设备发送几条消息，然后停止抓包 (Ctrl+C)

# 用 Wireshark 打开 /tmp/mqtt_traffic.pcap
# 过滤器: mqtt
# 查看:
# 1. 是否看到重复的 PUBLISH 包（PacketID 相同）
# 2. 重发的时间间隔是多少
# 3. 是否有 TCP 重传标志 [TCP Retransmission]
```

### 步骤 3: 测试不同的消息类型

```bash
# 安装 mosquitto 客户端
# macOS: brew install mosquitto
# Linux: apt-get install mosquitto-clients

# 发送一个简单的测试消息（QoS 0）
mosquitto_pub -h localhost -p 1883 -t "test/topic" -m "hello" -q 0 -d

# 观察:
# 1. mosquitto_pub 是否报错
# 2. 服务器日志是否显示 rejected
# 3. 是否看到重发行为
```

### 步骤 4: 检查 PSK 配置

```bash
# 查看当前配置
cat config.yaml | grep -A 5 "meshtastic:"

# 如果使用默认 PSK
psk: "AQ==" # 这是索引 1 的默认 PSK

# 如果使用自定义 PSK，确保与设备一致
psk: "your_base64_encoded_psk"
```

## 常见原因和解决方案

### 原因 1: 消息无法解密

**症状：** 日志显示 `error=cannot be decrypted`

**解决方案 A - 配置正确的 PSK：**
```yaml
# config.yaml
meshtastic:
  psk: "your_base64_psk"  # 与设备 channel 的 PSK 一致
```

**解决方案 B - 允许转发加密消息：**
```yaml
# config.yaml
meshtastic:
  allow_encrypted_forwarding: true  # 即使无法解密也转发
```

### 原因 2: 节点或内容被屏蔽

**症状：** 日志显示 `PUBLISH blocked`

**解决方案：**
```sql
-- 查看屏蔽规则
SELECT * FROM blocking_rules WHERE enabled = 1;

-- 临时禁用特定规则
UPDATE blocking_rules SET enabled = 0 WHERE id = <rule_id>;

-- 或禁用所有规则测试
UPDATE blocking_rules SET enabled = 0;
```

### 原因 3: Protobuf 解析失败

**症状：** 日志显示 `error=protobuf decode failed`

**可能原因：**
- 设备发送的不是标准的 Meshtastic 协议包
- 固件版本不兼容
- 数据损坏

**解决方案：**
- 更新设备固件到最新版本
- 检查设备配置是否正确
- 联系设备厂商

### 原因 4: 设备端 Bug

**症状：** 服务器日志显示消息正常接收，没有 rejected/blocked，但设备仍然重发

**诊断方法：**
1. 检查设备日志（如果可访问）
2. 更新设备固件
3. 尝试不同的 QoS 级别（QoS 1）看是否还重发
4. 联系设备厂商报告问题

## 监控脚本

创建一个监控脚本 `monitor_rejects.sh`：

```bash
#!/bin/bash
echo "监控 MQTT 消息拒绝情况..."
echo "按 Ctrl+C 停止"
echo ""

# 实时监控日志
tail -f /path/to/server.log | grep --line-buffered -E "rejected|blocked" | while read line; do
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$timestamp] $line"
    
    # 播放提示音（可选）
    # echo -e "\a"
done
```

使用：
```bash
chmod +x monitor_rejects.sh
./monitor_rejects.sh
```

## 性能统计

查看消息处理统计：

```sql
-- 最近一小时的消息统计
SELECT 
    'Forwarded' as type,
    COUNT(*) as count
FROM packets
WHERE created_at > strftime('%s', 'now', '-1 hour')
UNION ALL
SELECT 
    'Rejected' as type,
    COUNT(*) as count
FROM discarded_packets
WHERE created_at > strftime('%s', 'now', '-1 hour');

-- 按客户端统计
SELECT 
    client_id,
    COUNT(*) as total_messages,
    SUM(CASE WHEN from_discarded = 1 THEN 1 ELSE 0 END) as rejected,
    printf('%.2f%%', 
        SUM(CASE WHEN from_discarded = 1 THEN 1 ELSE 0 END) * 100.0 / COUNT(*)
    ) as reject_rate
FROM (
    SELECT client_id, 0 as from_discarded FROM packets
    WHERE created_at > strftime('%s', 'now', '-1 hour')
    UNION ALL
    SELECT client_id, 1 as from_discarded FROM discarded_packets
    WHERE created_at > strftime('%s', 'now', '-1 hour')
)
GROUP BY client_id
ORDER BY rejected DESC;
```

## 总结

遵循这个诊断流程：

1. ✅ **启用详细日志** - 最重要的第一步
2. ✅ **观察是否有 rejected/blocked** - 判断是否服务器拒绝
3. ✅ **检查数据库** - 查看历史拒绝记录
4. ✅ **抓包分析** - 确认网络层行为
5. ✅ **根据原因修复** - 应用对应的解决方案

如果日志中**没有任何 rejected/blocked 消息**，但设备仍然重发，那么问题100%在**设备端固件**，需要：
- 更新设备固件
- 检查设备配置
- 联系设备厂商
