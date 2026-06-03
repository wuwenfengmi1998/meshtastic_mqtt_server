# Meshtastic MQTT Server

本程序启动一个本地 MQTT broker，并在转发客户端发布的消息前校验 Meshtastic MQTT payload。

每条传入的 `PUBLISH` 都会先进入：

```go
valid, _, record := mqtpp.MQTTPP(topic, payload, key)
```

- `valid == true`：保留原始 topic、payload、QoS、retain 等字段，正常转发给订阅匹配 topic 的客户端
- `valid == false`：丢弃该消息，不转发给订阅客户端

当前不桥接到 `mqtt.meshtastic.org` 等上游 broker。

## 运行

```bash
go run .
```

默认监听：

- host：`0.0.0.0`
- port：`1883`
- PSK：`AQ==`
- TLS：关闭
- 数据库：SQLite
- SQLite 文件：Unix/Linux 为 `/srv/mesh_mqtt_go/mesh_mqtt_go.db`，Windows 测试为 `./win/etc/mesh_mqtt_go/mesh_mqtt_go.db`

首次启动会自动生成配置文件；之后每次启动都会检查配置项，缺失项会自动补全并写回。

配置文件路径：

- Unix/Linux：`/etc/mesh_mqtt_go/config.yaml`
- Windows 测试：`./win/etc/mesh_mqtt_go/config.yaml`

默认配置内容：

```yaml
mqtt:
  host: 0.0.0.0
  port: 1883
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
meshtastic:
  psk: AQ==
database:
  driver: sqlite
  sqlite:
    path: /srv/mesh_mqtt_go/mesh_mqtt_go.db
  mysql:
    dsn: ""
```

配置优先级：

```text
内置默认值 < 配置文件 < 命令行参数
```

也可以用命令行临时覆盖监听地址、PSK 和 TLS 设置：

```bash
go run . --host 127.0.0.1 --port 1883 --psk AQ==
```

## 参数

```text
--host      MQTT broker listen host
--port      MQTT broker listen port
--psk       Base64 channel PSK used to try decrypting encrypted packets
--tls       Enable MQTT TLS listener
--tls-cert     MQTT TLS certificate file
--tls-key      MQTT TLS private key file
--db-driver    Database driver: sqlite or mysql
--sqlite-path  SQLite database file path
--mysql-dsn    MySQL database DSN
```

## TLS 配置示例

```yaml
mqtt:
  host: 0.0.0.0
  port: 8883
  tls:
    enabled: true
    cert_file: ./certs/server.crt
    key_file: ./certs/server.key
meshtastic:
  psk: AQ==
```

启用 TLS 后，`cert_file` 和 `key_file` 必须指向可读取的证书和私钥文件。

## 数据库持久化

程序默认启用 SQLite，并持久化以下数据：

- `nodeinfo_map`：融合 `type == "nodeinfo"` 和 `type == "map_report"` 的节点信息
- `text_message`：追加保存 `type == "text_message"` 的聊天消息

`nodeinfo_map` 规则：

- `nodeinfo` 表不再使用；如果旧数据库中已经存在该表，程序不会自动删除它
- 同一节点以 `node_id`（即解析结果中的 `from`，例如 `!a8dfd867`）作为主键
- 重复收到同一节点时不会插入重复行，只更新 `updated_at`、`content_json`、`latest_type` 和本次记录中有值的字段
- `nodeinfo` 独有字段和 `map_report` 独有字段会互相保留；例如后续 `map_report` 不会清空已有的 `public_key`
- `first_seen_at` 保留第一次写入时间
- `content_json` 保存最新一次 `nodeinfo` 或 `map_report` 的完整解析结果 JSON

`text_message` 规则：

- 使用自增 `id` 作为主键
- 每条聊天消息都会新增一行，不做去重
- 保存 `from_id`、`from_num`、`text`、`payload_hex`、topic、packet 元数据和完整 `content_json`
- 保存 MQTT 客户端信息：`mqtt_client_id`、`mqtt_username`、`mqtt_listener`、`mqtt_remote_addr`、`mqtt_remote_host`、`mqtt_remote_port`

查询最近聊天消息示例：

```sql
SELECT id, created_at, from_id, text, mqtt_remote_host
FROM text_message
ORDER BY id DESC
LIMIT 20;
```

SQLite 默认路径：

- Unix/Linux：`/srv/mesh_mqtt_go/mesh_mqtt_go.db`
- Windows 测试：`./win/etc/mesh_mqtt_go/mesh_mqtt_go.db`

MySQL 配置示例：

```yaml
database:
  driver: mysql
  sqlite:
    path: /srv/mesh_mqtt_go/mesh_mqtt_go.db
  mysql:
    dsn: mesh_user:mesh_pass@tcp(127.0.0.1:3306)/mesh_mqtt_go?parseTime=true&charset=utf8mb4,utf8
```

使用 MySQL 时，需要提前创建好 database/schema。

## 转发规则

程序监听所有传入 publish。payload 能被 `mqtpp.MQTTPP` 解析时，认为 `valid == true`，broker 会继续把原始 MQTT 消息转发给订阅者；解析失败时，认为 `valid == false`，broker 会拒绝并丢弃该 publish。

`empty_packet` 仍然属于 `valid == true`，会被转发；只是控制台默认不显示它。

无法解密但能解析的加密包通常会输出为 `encrypted_packet`，仍然属于 `valid == true`，因此会被转发。

## 本地验证

一个终端启动 broker：

```bash
go run . --host 127.0.0.1 --port 1883 --psk AQ==
```

另一个终端订阅：

```bash
mosquitto_sub -h 127.0.0.1 -p 1883 -t '#'
```

发布非法 payload：

```bash
mosquitto_pub -h 127.0.0.1 -p 1883 -t 'msh/US/test' -m 'not protobuf'
```

订阅端应该收不到该消息。

要验证 valid 消息转发，请使用真实 Meshtastic MQTT payload 发布到本 broker；订阅匹配 topic 的客户端应收到原始消息，broker 控制台会打印解析后的 `record`。

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
