# Meshtastic MQTT Server

这是一个 Meshtastic MQTT 订阅工具，用 Go 实现 [py/mqtt_nodeinfo_subscriber.py](py/mqtt_nodeinfo_subscriber.py) 的主要功能。

程序会连接 Meshtastic MQTT broker，订阅指定 topic，解析 `ServiceEnvelope` / `MeshPacket`，并将重点数据包以 JSONL 形式输出到控制台。

## 运行

```bash
go run .
```

默认连接：

- broker：`mqtt.meshtastic.org:1883`
- username：`meshdev`
- password：`large4cats`
- topic：`msh/US/#`
- PSK：`AQ==`

也可以指定 topic：

```bash
go run . --topic 'msh/US/#'
```

多个 topic 可重复传入：

```bash
go run . --topic 'msh/US/#' --topic 'msh/EU_868/#'
```

## 参数

```text
--host       MQTT broker hostname
--port       MQTT broker port
--username   MQTT username
--password   MQTT password
--psk        Base64 channel PSK used to try decrypting encrypted packets
--topic      Topic to subscribe; may be repeated
--qos        MQTT subscription QoS: 0, 1, or 2
--client-id  MQTT client id
```

## 控制台颜色说明

程序会按数据包类型使用不同背景色，方便快速区分消息类型。

| 背景色 | type | portnum | 含义 |
|---|---|---|---|
| 绿色 | `nodeinfo` | `NODEINFO_APP` | 节点信息包，包含节点 ID、长名称、短名称、硬件型号、角色、公钥等 |
| 蓝色 | `map_report` | `MAP_REPORT_APP` | 地图报告包，包含节点名称、硬件、固件版本、区域、调制预设、位置等地图信息 |
| 紫色 | `text_message` | `TEXT_MESSAGE_APP` | 聊天文本消息 |
| 青色 | `position` | `POSITION_APP` | 位置包，表示节点位置相关数据；当前只标记类型，不展开解析 payload |
| 黄色 | `telemetry` | `TELEMETRY_APP` | 遥测包，表示电池、信道、设备或环境传感器相关数据；当前只标记类型，不展开解析 payload |
| 灰色 | `routing` | `ROUTING_APP` | 路由控制包，常见于 ACK、NAK、路由错误等控制信息 |
| 灰色 | `traceroute` | `TRACEROUTE_APP` | 路径追踪包，用于 mesh 网络路径探测 |
| 红色 | error record | - | protobuf 解析失败、payload 解码失败或其他处理错误 |
| 无颜色 | `encrypted_packet` | - | 加密包但当前 PSK/频道 hash 无法解密；这不一定是错误 |
| 无颜色 | `decoded_packet` | 其他 portnum | 已解码/已解密，但程序尚未细分的其他应用包 |

## 过滤规则

程序默认不显示 `empty_packet`。

`empty_packet` 指 `MeshPacket` 中没有 `decoded` 或 `encrypted` payload 的包，只包含类似 `from`、`to`、`id`、`via_mqtt` 等包头信息。根据固件源码分析，这类包通常不是普通业务数据，更多是 MQTT 回显/隐式 ACK 相关的元信息，对查看节点信息、地图报告和聊天内容价值较低。

## 输出示例

节点信息包：

```json
{"type":"nodeinfo","portnum":"NODEINFO_APP","from":"!a8dfd867","long_name":"Kabi Matrix 🖥️","short_name":"KaMX","hw_model":"PRIVATE_HW","role":"CLIENT_MUTE"}
```

地图报告包：

```json
{"type":"map_report","portnum":"MAP_REPORT_APP","from":"!675c9803","long_name":"PaulHome","latitude":42.51043,"longitude":-83.08624999999999,"hw_model":"PORTDUINO"}
```

聊天消息包：

```json
{"type":"text_message","portnum":"TEXT_MESSAGE_APP","from":"!12345678","text":"hello mesh"}
```

解密失败的加密包：

```json
{"type":"encrypted_packet","decrypt_success":false,"decrypt_status":"channel hash mismatch","encrypted_len":43}
```
