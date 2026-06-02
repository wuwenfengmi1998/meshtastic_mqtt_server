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
| 青色 | `position` | `POSITION_APP` | 位置包，会展开解析经纬度、海拔、时间、定位来源、精度、速度、卫星数等字段 |
| 黄色 | `telemetry` | `TELEMETRY_APP` | 遥测包，会展开解析设备、电源、环境、空气质量、本地统计、健康、主机和流量管理指标 |
| 灰色 | `routing` | `ROUTING_APP` | 路由控制包，常见于 ACK、NAK、路由错误等控制信息 |
| 灰色 | `traceroute` | `TRACEROUTE_APP` | 路径追踪包，用于 mesh 网络路径探测 |
| 红色 | error record | - | protobuf 解析失败、payload 解码失败或其他处理错误 |
| 无颜色 | `encrypted_packet` | - | 加密包但当前 PSK/频道 hash 无法解密；这不一定是错误 |
| 无颜色 | `decoded_packet` | 其他 portnum | 已解码/已解密，但程序尚未细分的其他应用包 |

## 已展开解析的数据包

### `position` / `POSITION_APP`

位置包会从 Meshtastic `Position` payload 中展开常用字段，包括：

- `latitude` / `longitude`：经纬度，已从 `latitude_i` / `longitude_i` 转换为浮点角度
- `altitude`：海拔，单位米
- `time` / `timestamp`：位置相关时间戳
- `location_source`：定位来源，例如 `LOC_MANUAL`、`LOC_INTERNAL`、`LOC_EXTERNAL`
- `altitude_source`：海拔来源，例如 `ALT_MANUAL`、`ALT_INTERNAL`、`ALT_BAROMETRIC`
- `altitude_hae` / `altitude_geoidal_separation`：HAE 海拔和大地水准面分离值
- `pdop` / `hdop` / `vdop`：定位精度因子，已从 1/100 单位转换为浮点值
- `gps_accuracy`：GPS 精度，单位 mm
- `ground_speed`：地面速度，单位 m/s
- `ground_track`：地面航迹角，已从 1/100 度转换为度
- `fix_quality` / `fix_type` / `sats_in_view`：GPS fix 质量、类型和可见卫星数
- `sensor_id` / `next_update` / `seq_number` / `precision_bits`：传感器、更新间隔、序列号和位置精度位数

### `telemetry` / `TELEMETRY_APP`

遥测包会输出：

- `time`：遥测时间戳
- `telemetry_type`：具体 telemetry variant
- `metrics`：展开后的指标对象

当前支持的 `telemetry_type`：

| telemetry_type | 含义 | 常见 metrics |
|---|---|---|
| `device_metrics` | 设备状态 | `battery_level`、`voltage`、`channel_utilization`、`air_util_tx`、`uptime_seconds` |
| `environment_metrics` | 环境传感器 | `temperature`、`relative_humidity`、`barometric_pressure`、`gas_resistance`、`lux`、`wind_speed`、`rainfall_1h` 等 |
| `air_quality_metrics` | 空气质量 | `pm25_standard`、`pm100_standard`、`co2`、`pm_temperature`、`pm_humidity`、`pm_voc_idx` 等 |
| `power_metrics` | 多通道电源数据 | `ch1_voltage`、`ch1_current` 到 `ch8_voltage`、`ch8_current` |
| `local_stats` | 本地 mesh 统计 | `num_packets_tx`、`num_packets_rx`、`num_online_nodes`、`heap_free_bytes`、`noise_floor` 等 |
| `health_metrics` | 健康数据 | `heart_bpm`、`spO2`、`temperature` |
| `host_metrics` | Linux/Portduino 主机指标 | `uptime_seconds`、`freemem_bytes`、`diskfree1_bytes`、`load1`、`load5`、`load15`、`user_string` |
| `traffic_management_stats` | 流量管理统计 | `packets_inspected`、`position_dedup_drops`、`rate_limit_drops`、`unknown_packet_drops` 等 |

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

位置包：

```json
{"type":"position","portnum":"POSITION_APP","from":"!12345678","latitude":42.51043,"longitude":-83.08625,"altitude":192,"location_source":"LOC_INTERNAL","sats_in_view":8}
```

遥测包：

```json
{"type":"telemetry","portnum":"TELEMETRY_APP","from":"!12345678","telemetry_type":"device_metrics","metrics":{"battery_level":85,"voltage":4.1,"channel_utilization":2.3,"air_util_tx":0.5,"uptime_seconds":12345}}
```

解密失败的加密包：

```json
{"type":"encrypted_packet","decrypt_success":false,"decrypt_status":"channel hash mismatch","encrypted_len":43}
```
