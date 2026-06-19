#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="mesh_mqtt_go"
SERVICE_USER="mesh_mqtt_go"
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "拉取最新代码..."
git pull

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
go build -o "${BINARY_NAME}" .

echo "检查系统用户..."
if ! id -u "${SERVICE_USER}" >/dev/null 2>&1; then
  useradd --system --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin "${SERVICE_USER}"
fi

echo "创建目录..."
install -d -m 0750 -o "${SERVICE_USER}" -g "${SERVICE_USER}" "${CONFIG_DIR}" "${DATA_DIR}"
install -d -m 0755 -o "${SERVICE_USER}" -g "${SERVICE_USER}" "${INSTALL_DIR}"

echo "安装程序和前端文件..."
install -m 0755 -o root -g root "${SCRIPT_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
rm -rf "${INSTALL_DIR}/dist"
cp -a "${SCRIPT_DIR}/${FRONTEND_DIST_DIR}" "${INSTALL_DIR}/dist"
chown root:root "${INSTALL_DIR}/${BINARY_NAME}"
chown -R root:root "${INSTALL_DIR}/dist"
chown "${SERVICE_USER}:${SERVICE_USER}" "${INSTALL_DIR}"
chmod 0755 "${INSTALL_DIR}"
find "${INSTALL_DIR}/dist" -type d -exec chmod 0755 {} \;
find "${INSTALL_DIR}/dist" -type f -exec chmod 0644 {} \;

if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
  cat > "${CONFIG_DIR}/config.yaml" <<EOF
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
    path: ${DATA_DIR}/${SERVICE_NAME}.db
  mysql:
    dsn: ""
web:
  enabled: true
  host: 0.0.0.0
  port: 8080
  socket_path: ${SOCKET_PATH}
  static_dir: ${INSTALL_DIR}/dist
  console_log: true
  admin:
    username: admin
    password: admin
    session_secret: ""
    session_secure: false
EOF
  chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}/config.yaml"
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
Group=${SERVICE_USER}
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
