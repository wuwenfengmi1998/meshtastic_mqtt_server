# 安全修复 TODO

依据 2026-08-20 安全审计(源码 + meshmap.lmve.net 线上实测)整理,按优先级排列。
完成一项勾选一项;每项含位置、修复方案、验收标准。

---

## P0 - 立即处理(已被暴露/可直接利用)

- [x] **T1 数据库口令治理(2026-08-20 完成,含一处误报修正)**
  - ~~位置:`py/db_config.py` 两套 MySQL 明文口令已入库~~
  - **修正:该文件从未进入 git 历史**(`py/.gitignore` 已忽略,`git log --all -- py/db_config.py` 为空),无需 filter-repo 清洗
  - 已完成:`py/db_config.py` 与可入库模板 `py/db_config.example.py` 全部改为环境变量读取(`MESH_SOURCE_DB_PASSWORD` / `MESH_TARGET_DB_PASSWORD`),口令不再落盘
  - **剩余(需手动)**:建议仍轮换两套 MySQL 口令(本地明文留存过,且出现在审计输出中):`ALTER USER ... IDENTIFIED BY '新口令';`
  - 验收:口令文件不存在明文;脚本从环境变量读取 ✓

- [x] **T2 MQTT broker 认证(2026-08-20 代码完成,待部署启用)**
  - 位置:`main.go`(原 AllowHook 零认证,1883 公网可达)
  - 已完成:
        1. 新增 `internal/mqttauth`:可配置用户(bcrypt)+ 匿名开关 + 按 IP 失败限速(默认 5 次/分钟,封禁 5 分钟)+ 未知用户名 dummy bcrypt 防时间侧信道
        2. 配置段 `mqtt.auth`(enabled/allow_anonymous/users);配置中明文 `password` 首次载入自动转 `password_hash` 并从文件剔除
        3. `auth.enabled: false` 默认,升级零影响;install.sh 模板与 README 已更新
        4. 测试:单元 + 真实 broker/paho 端到端(有效凭据放行、错误拒绝、匿名拒绝、超限封禁)全部通过
  - **剩余(需手动,服务器上执行)**:
        1. `/etc/mesh_mqtt_go/config.yaml` 增加:`mqtt.auth.enabled: true` 与用户(哈希生成:`htpasswd -bnBC 10 "" '密码' | tr -d ':\n'`,或直接写明文 password 由首启转哈希)
        2. 重启服务;Meshtastic 节点/客户端 MQTT 上行配置填入相同账号
        3. 仍建议防火墙限制 1883 来源网关 IP(认证之外的纵深防御)

## P1 - 高优先(1-2 周内)

- [x] **T3 install.sh 不再生成 admin/admin 默认口令(2026-08-20 完成)**
  - `install.sh` 首启随机生成 16 位管理员密码并仅打印一次;`main.go` 新增守卫:Web 监听非回环地址且口令仍为默认 `admin` 时拒绝启动(`guardDefaultAdminPassword`);README 同步
  - 验收:新部署拿到随机口令;公网 0.0.0.0+admin/admin 无法启动 ✓(冒烟验证)
  - 注意:已部署实例不受影响(守卫只拦默认口令);本地开发若用 0.0.0.0+admin/admin 需先改口令

- [x] **T4 登录防爆破与用户名枚举(2026-08-20 完成)**
  - 新增 `internal/ratelimit`(按 key 失败计数+临时封锁,默认 5 次/分钟锁定 10 分钟);登录按来源 IP + 用户名双维度限速,超限返回 429
  - 未知用户名也执行 dummy bcrypt,消除时间侧信道
  - 验收:连续 5 次错误→第 6 次 429;未知/已知用户名耗时一致 ✓(冒烟验证)

- [x] **T5 瓦片代理 SSRF 加固(2026-08-20 完成)**
  - `map_tile_proxy_routes.go`:自定义 DialContext 按解析 IP 拒绝 loopback/私网/链路本地/CGNAT/组播/ULA(DNS rebinding 安全);CheckRedirect 限 2 跳且每跳同样受 Dial 检查;`mapTileContentType` 仅放行 `image/*`,其余强制 `application/octet-stream` + `X-Content-Type-Options: nosniff`(堵同源 HTML XSS)
  - 验收:元数据地址 502;外网 302→内网 502;HTML 响应不再以 text/html 下发 ✓(单测+端到端)

- [x] **T6 `/api/discard-details` 去敏(2026-08-20 完成)**
  - 公开接口改用 `discardDetailsPublicDTO`,剔除 `mqtt_remote_addr/host/port` 与 `raw_base64`;新增 `GET /api/admin/discard-details` 全字段端点(RequireAdmin);前端 `AdminDiscardDetails` 改调 admin 端点
  - 验收:匿名响应无 IP/原始报文;管理页功能保留 ✓(冒烟验证)
  - 注意:前端变更需重新 build 部署;`/api/text-messages` 仍公开返回 `mqtt_remote_host`(同源泄露,已列入 P3 观察)

- [x] **T7 外部瓦片 URL/key 不再下发前端(2026-08-20 完成)**
  - `PublicDTO` 对外部 http(s) 模板一律改写为 `/api/map/{hash}` 代理路径(不再依赖 proxy_enabled 标志);高德 key 等不再出现在 `/api/map-source/enabled` 响应中
  - 验收:公开响应无 `key=`/外部域名;瓦片经代理加载 ✓(单测+端到端)

## P2 - 中优先(迭代内)

- [x] **T8 LLM 会话按 (bot, peer) 隔离(2026-08-20 完成)**
  - `message.Conversation` 增加 `peer_node_id`;`GetOrCreateForBot` 按 (bot, peer) 匹配会话,不同对端互不可见上下文;`AddMessage` 历史上限 50 条;无 peer 调用维持旧行为
  - 验收:不同 peer DM 得到独立会话;A 的注入不影响 B ✓(单测)

- [x] **T9 LLM 入队限流(2026-08-20 完成)**
  - `EnqueueLLMMessage` 增加 (bot, from_node) 窗口限流:60 秒内 pending/processing 超过 5 条拒绝(ErrLLMQueueRateLimited);频道/私聊两条入队路径对限流静默跳过
  - 验收:单一来源高频消息只消耗有限 LLM 调用 ✓

- [x] **T10 瓦片磁盘缓存配额(2026-08-20 完成)**
  - 单源 3000 文件 / 300MB 双上限,每写 20 个文件触发一次按 mtime 淘汰最旧;防止匿名遍历坐标打满磁盘
  - 验收:缓存目录超配额被收敛 ✓

- [x] **T11 sign 污染治理(2026-08-20 完成)**
  - 核实:每节点每日去重与 admin 删除已存在;补充:签到墙 `/api/signs`、`/api/signs/daily` 加 IP 限速(60 次/分钟);签到工具增加全站每日 1000 条总量封顶
  - 验收:伪造节点刷签到受每日总量限制;公开墙读接口有限速 ✓

- [x] **T12 敏感数据落盘加密(2026-08-20 完成)**
  - 新增 `internal/secrets`(AES-256-GCM,密钥由 `MESH_SECRET_KEY` SHA-256 派生,enc:v1: 前缀,无密钥时明文兼容)
  - 覆盖:LLM provider api_key、MQTT forwarder source/target 密码、bot X25519 私钥(在 store 写入点加密,读取点透明解密)
  - 验收:DB 中为 enc:v1: 密文,明文不可见;旧明文数据可读;未配置密钥时行为不变 ✓(冒烟验证)

- [x] **T13 改密需验证当前密码 + session 可撤销(2026-08-20 完成)**
  - `PUT /api/admin/users/:id/password` 必须携带请求方自己的 `current_password`,错误返回 403;users 表新增 `pwd_version`,改密自增,session claims 携带 pwd_ver,中间件对照 DB,改密后所有旧会话立即 401
  - 前端 AdminUsers 增加"当前登录密码"输入
  - 验收:改密后旧会话 401、新密码可登录、旧密码不可 ✓(冒烟验证)

## P3 - 低优先(择机)

- [x] **T14 解密内容默认不打控制台日志(2026-08-20 完成)** - `console_log.meshtastic` 默认改 false(新部署);install.sh 模板同步;README 注明调试可显式开启。既有配置显式写 true 的不受影响
- [x] **T15 session cookie `Secure: true` 默认(2026-08-20 完成)** - config 默认与 install.sh 模板改 `session_secure: true`(HTTPS 部署适用);纯 HTTP 部署需显式改回 false;README 注明
- [x] **T16 config.yaml 回写权限(2026-08-20 完成)** - `config.Write` 改 0600,配置文件含明文口令仅属主可读 ✓(冒烟验证)
- [x] **T17 公开接口错误信息脱敏(2026-08-20 完成)** - `webutil.WriteListResponse*` 统一返回 "internal error" 并 stderr 记详情;`/api/health`、`/api/channels` 同处理;`/api/text-messages` 移除 `mqtt_remote_host`(与 T6 同源泄露)
- [x] **T18 bot PSK 不回显(2026-08-20 完成)** - bot DTO 改 `psk_set` 布尔;`UpdateBotNode` 空 PSK 保持原值(避免改配置时密钥被重置为 AQ==);前端编辑表单留空保持不变 ✓(冒烟验证)
- [x] **T19 前端 help 页 DOMPurify 消毒(2026-08-20 完成)** - `HelpPage` 与 `AdminHelpEdit` 的 v-html 前增加 `DOMPurify.sanitize`(纵深防御,服务端 bluemonday 之上);新增依赖 `dompurify`

---

## 已确认无需修复(复核时勿重复排查)

- **nodeinfo 公钥替换影响 bot DM 加密**:Meshtastic 协议特性,公钥本就经由频道 PSK 加密的 nodeinfo 分发、无签名认证,官方 broker 与官方客户端(仅本地 key pinning + 变更告警)行为一致,不做服务端拦截
- SQL 注入:store 层全部参数化,无 LIKE/ORDER BY 拼接
- 前端 XSS:消息与节点名均经 Vue 转义,Leaflet popup 手工转义完整
- 静态服务/瓦片缓存路径穿越:不存在
- protobuf/PKI 解析 panic:未发现;calculator 工具为 AST 白名单,无 exec/文件访问
- 私钥/API key/forwarder 密码的 API DTO 脱敏与日志:现状正确
