# Meshtastic MQTT Server

本程序启动一个本地 MQTT broker，并在转发客户端发布的消息前校验 Meshtastic MQTT payload。

每条传入的 `PUBLISH` 都会先进入：

```go
valid, _, record := mqtpp.MQTTPP(topic, payload, key, mqtpp.Options{})
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
- Web：`0.0.0.0:8080`，静态目录 `./dist`
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
web:
  enabled: true
  host: 0.0.0.0
  port: 8080
  static_dir: ./dist
  admin:
    username: admin
    password: admin
    session_secret: ""
    session_secure: false
```

配置优先级：

```text
内置默认值 < 配置文件 < 环境变量 < 命令行参数
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
--mysql-dsn        MySQL database DSN
--web              Enable Gin web server
--web-host         Web server listen host
--web-port         Web server listen port
--web-static-dir   Web frontend static files directory
```

## Web 前端

开发模式：

```bash
go run . --web-host 127.0.0.1 --web-port 8080
cd meshmap_frontend
npm run dev
```

生产构建：

```bash
cd meshmap_frontend
npm run build
cd ..
go run .
```

构建后的文件位于项目根目录 `dist/`，Gin 会提供静态文件服务；`/api` 路径保留给后端接口。

管理页面位于 `/admin`，默认管理员账号为 `admin` / `admin`。生产环境请修改 `web.admin.password` 或设置 `MESH_ADMIN_PASSWORD`，并配置固定的 `web.admin.session_secret` 或 `MESH_ADMIN_SESSION_SECRET`；如果 `session_secret` 为空，程序会在启动时生成临时签名密钥，重启后需要重新登录。后台页面包括 `/admin` 服务状态、`/admin/users` 用户管理、`/admin/log/login` 登录日志、`/admin/discard_details` 丢弃数据。`/admin` 中的“丢弃消息”统计来自 `discard_details` 表记录数，点击可进入丢弃数据分页页。后台支持新增管理员用户和修改用户密码；密码使用 bcrypt hash 保存，API 不会返回密码 hash。修改密码不会立即使已签发 Session 失效，当前 Session 到期或退出登录后才需要使用新密码。登录成功和失败都会记录到登录日志，包含用户名、结果、原因、来源地址、User-Agent 和时间。管理员可在主页右键删除聊天消息、地图节点或节点列表记录；删除节点会删除 `nodeinfo` 和 `map_report` 当前状态，不会删除历史消息、位置、遥测等 append 记录，后续收到新的节点上报时可能重新出现。

常用 API：

```text
GET /api/health
POST /api/admin/login
POST /api/admin/logout
GET /api/admin/me
GET /api/admin/mqtt/status
GET /api/admin/log/login
GET /api/admin/users
POST /api/admin/users
PUT /api/admin/users/:id/password
DELETE /api/admin/text-messages/:id
DELETE /api/admin/nodes/:id
GET /api/nodeinfo
GET /api/nodeinfo/:id
GET /api/map-reports
GET /api/map-reports/:id
GET /api/nodes        # /api/nodeinfo 的兼容别名
GET /api/nodes/:id    # /api/nodeinfo/:id 的兼容别名
GET /api/text-messages
GET /api/discard-details
GET /api/positions
GET /api/telemetry
GET /api/routing
GET /api/traceroute
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

程序默认启用 SQLite，数据库表迁移和操作由 GORM 执行，并持久化以下数据：

- `login_log`：追加保存后台登录成功和失败日志
- `discard_details`：追加保存 `MQTTPP` 判定无效而被 broker 丢弃的数据，raw payload 使用 base64 保存
- `nodeinfo`：保存 `type == "nodeinfo"` 的节点身份和设备信息
- `map_report`：保存 `type == "map_report"` 的地图报告信息，前端地图从该表读取
- `text_message`：追加保存 `type == "text_message"` 的聊天消息
- `position`：追加保存 `type == "position"` 的位置包
- `telemetry`：追加保存 `type == "telemetry"` 的遥测包
- `routing`：追加保存 `type == "routing"` 的路由控制包
- `traceroute`：追加保存 `type == "traceroute"` 的路径追踪包

`nodeinfo` / `map_report` 规则：

- 两张表都以 `node_id`（即解析结果中的 `from`，例如 `!a8dfd867`）作为主键
- `nodeinfo` 只保存节点身份和设备字段，例如 `user_id`、名称、硬件型号、角色、授权状态和公钥
- `map_report` 只保存地图报告字段，例如名称、硬件型号、角色、固件版本、区域、调制预设、经纬度、海拔、位置精度和在线节点数
- 重复收到同一节点时不会插入重复行，只更新 `updated_at`、`content_json` 和本次记录中有值的字段
- `first_seen_at` 保留第一次写入时间
- `content_json` 分别保存最新一次 `nodeinfo` 或 `map_report` 的完整解析结果 JSON
- 旧版本创建的 `nodeinfo_map` 融合表不会被自动删除，新版本不再写入该表；新表会从新收到的数据开始填充

`text_message` 规则：

- 使用自增 `id` 作为主键
- 每条聊天消息都会新增一行，不做去重
- 保存 `from_id`、`from_num`、`text`、`payload_hex`、topic、packet 元数据和完整 `content_json`
- 保存 MQTT 客户端信息：`mqtt_client_id`、`mqtt_username`、`mqtt_listener`、`mqtt_remote_addr`、`mqtt_remote_host`、`mqtt_remote_port`

`position` / `telemetry` / `routing` / `traceroute` 规则：

- 都使用自增 `id` 作为主键
- 每条有效记录都会新增一行，不做去重
- 保存通用 packet 元数据、MQTT 客户端信息和完整 `content_json`
- `position` 额外保存经纬度、海拔、时间、定位来源、精度、速度、卫星数等字段
- `telemetry` 额外保存 `telemetry_type`，并把动态 `metrics` 对象保存为 `metrics_json`
- `routing` 和 `traceroute` 当前保存通用元数据和完整 JSON；后续如果解析更多 payload 字段，可继续扩展列

查询最近聊天消息示例：

```sql
SELECT id, created_at, from_id, text, mqtt_remote_host
FROM text_message
ORDER BY id DESC
LIMIT 20;
```

查询位置包示例：

```sql
SELECT id, created_at, from_id, latitude, longitude, altitude
FROM position
ORDER BY id DESC
LIMIT 20;
```

查询遥测包示例：

```sql
SELECT id, created_at, from_id, telemetry_type, metrics_json
FROM telemetry
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

无法解密的加密包会输出为 `encrypted_packet`，属于 `valid == false`，因此会被拒绝并丢弃。

丢弃的 publish 会写入 `discard_details`，记录 topic、错误原因、payload 长度、base64 raw payload、MQTT 客户端信息和完整 `content_json`。

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
