#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="mesh_mqtt_go"
SERVICE_USER="mesh_mqtt_go"
SERVICE_GROUP="${SERVICE_USER}"
CONFIG_DIR="/etc/${SERVICE_NAME}"
DATA_DIR="/srv/${SERVICE_NAME}"
INSTALL_DIR="/opt/${SERVICE_NAME}"
SOCKET_PATH="${INSTALL_DIR}/web.sock"
FRONTEND_DIR="meshmap_frontend"
FRONTEND_DIST_DIR="dist"
BINARY_NAME="${SERVICE_NAME}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

if [[ "${EUID}" -ne 0 ]]; then
  echo "请使用 root 权限运行: sudo $0" >&2
  exit 1
fi

if id -u "www" >/dev/null 2>&1; then
  SERVICE_USER="www"
  SERVICE_GROUP=$(id -gn "www")
  echo "检测到 www 用户，将以 www 用户运行服务"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "拉取最新代码..."
git pull
COMMIT_HASH=$(git rev-parse --short HEAD)

echo "编译前端..."
cd "${SCRIPT_DIR}/${FRONTEND_DIR}"
if [[ -f package-lock.json ]]; then
  npm ci
else
  npm install
fi
npm run build

echo "编译 Go 程序..."
cd "${SCRIPT_DIR}"
go build -ldflags "-X meshtastic_mqtt_server/internal/web.CommitVersion=${COMMIT_HASH}" -o "${BINARY_NAME}" .

echo "检查系统用户..."
if ! id -u "${SERVICE_USER}" >/dev/null 2>&1; then
  useradd --system --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin "${SERVICE_USER}"
fi

echo "创建目录..."
install -d -m 0750 -o "${SERVICE_USER}" -g "${SERVICE_GROUP}" "${CONFIG_DIR}" "${DATA_DIR}"
install -d -m 0755 -o "${SERVICE_USER}" -g "${SERVICE_GROUP}" "${INSTALL_DIR}"

echo "安装程序和前端文件..."
install -m 0755 -o root -g root "${SCRIPT_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
rm -rf "${INSTALL_DIR}/dist"
cp -a "${SCRIPT_DIR}/${FRONTEND_DIST_DIR}" "${INSTALL_DIR}/dist"
chown root:root "${INSTALL_DIR}/${BINARY_NAME}"
chown -R root:root "${INSTALL_DIR}/dist"
chown "${SERVICE_USER}:${SERVICE_GROUP}" "${INSTALL_DIR}"
chmod 0755 "${INSTALL_DIR}"
find "${INSTALL_DIR}/dist" -type d -exec chmod 0755 {} \;
find "${INSTALL_DIR}/dist" -type f -exec chmod 0644 {} \;

if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
  ADMIN_PASSWORD="$(head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 16)"
  if [[ -z "${ADMIN_PASSWORD}" ]]; then
    ADMIN_PASSWORD="admin" # 极低概率兜底,启动守卫会强制修改
  fi
  cat > "${CONFIG_DIR}/config.yaml" <<EOF
mqtt:
  host: 0.0.0.0
  port: 1883
  # MQTT 连接认证:enabled 改为 true 后,客户端必须携带 users 中的账号连接
  # (或 allow_anonymous: true 放行匿名)。生成哈希:
  #   htpasswd -bnBC 10 "" '你的密码' | tr -d ':\n'
  auth:
    enabled: false
    allow_anonymous: false
    users: []
    # users:
    #   - username: mesh
    #     password_hash: "\$2y\$10\$..."
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
meshtastic:
  psk: AQ==
database:
  driver: sqlite
  sqlite:
    path: ${DATA_DIR}/${SERVICE_NAME}.db
  mysql:
    dsn: ""
web:
  enabled: true
  port_enabled: true
  socket_enabled: true
  host: 0.0.0.0
  port: 8080
  socket_path: ${SOCKET_PATH}
  static_dir: ${INSTALL_DIR}/dist
  admin:
    username: admin
    password: ${ADMIN_PASSWORD}
    session_secret: ""
    # 前端经 HTTPS(nginx 反代)访问时保持 true;纯 HTTP 部署需改回 false
    session_secure: true
console_log:
  web: true
  mqtt: true
  llm: true
  sql: true
  # 默认不打印解码后的 Meshtastic 数据包(含私聊明文),调试时改回 true
  meshtastic: false
EOF
  chown "${SERVICE_USER}:${SERVICE_GROUP}" "${CONFIG_DIR}/config.yaml"
  chmod 0640 "${CONFIG_DIR}/config.yaml"
fi

echo "写入 systemd 服务文件..."
cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=Mesh MQTT Go Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -web-socket-path ${SOCKET_PATH} -web-static-dir ${INSTALL_DIR}/dist
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=${CONFIG_DIR} ${DATA_DIR} ${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl restart "${SERVICE_NAME}"

echo "部署完成，服务状态："
systemctl --no-pager --full status "${SERVICE_NAME}"
if [[ -n "${ADMIN_PASSWORD:-}" ]]; then
  echo
  echo "======================================================"
  echo "  Web 管理后台初始账号: admin / ${ADMIN_PASSWORD}"
  echo "  该密码仅显示这一次,请立即登录并修改!"
  echo "======================================================"
fi
