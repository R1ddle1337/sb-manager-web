#!/usr/bin/env bash
set -Eeuo pipefail
umask 027

PREFIX=${SBM_WEB_PREFIX:-/usr/local}
LIB_DIR=${SBM_WEB_LIB:-$PREFIX/lib/sb-manager-web}
BIN_DIR=${SBM_WEB_BIN:-$PREFIX/bin}
ETC_DIR=${SBM_WEB_ETC:-/etc/sb-manager-web}
VAR_DIR=${SBM_WEB_VAR:-/var/lib/sb-manager-web}
LOG_DIR=${SBM_WEB_LOG:-/var/log/sb-manager-web}
SYSTEMD_DIR=${SBM_WEB_SYSTEMD_DIR:-/etc/systemd/system}
OPENRC_DIR=${SBM_WEB_OPENRC_DIR:-/etc/init.d}
VERSION=${SBM_WEB_VERSION:-latest}
REPO=${SBM_WEB_REPO:-R1ddle1337/sb-manager-web}
NO_START=0

usage() {
  cat <<'EOF'
用法：sudo ./install.sh [--version VERSION] [--no-start]

环境变量：
  SBM_WEB_BINARY_URL       直接指定二进制地址
  SBM_WEB_CHECKSUM_URL     SHA256SUMS 地址
  SBM_WEB_SKIP_VERIFY=1    跳过摘要校验（仅开发环境）
  SBM_WEB_PREFIX/LIB/ETC/VAR/LOG  覆盖安装路径
EOF
}

while (($#)); do
  case "$1" in
    --version) VERSION=${2:?}; shift 2;;
    --no-start) NO_START=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "未知参数：$1" >&2; usage; exit 2;;
  esac
done

[[ ${EUID:-$(id -u)} -eq 0 ]] || { echo '请使用 root/sudo 运行。' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo '缺少 curl。' >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo '缺少 sha256sum。' >&2; exit 1; }
command -v sb >/dev/null 2>&1 || { echo '未发现 sb-manager，请先安装 sb-manager。' >&2; exit 1; }

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) asset_arch=amd64;;
  aarch64|arm64) asset_arch=arm64;;
  armv7l|armv7|armhf) asset_arch=armv7;;
  *) echo "暂不支持的架构：$arch" >&2; exit 1;;
esac

if [[ "$VERSION" == latest ]]; then
  command -v jq >/dev/null 2>&1 || { echo '解析 latest 需要 jq。' >&2; exit 1; }
  VERSION=$(curl --fail --silent --show-error --location "https://api.github.com/repos/$REPO/releases/latest" | jq -r '.tag_name // empty' | sed 's/^v//')
  [[ -n "$VERSION" ]] || { echo '无法解析 sb-manager-web 最新版本。' >&2; exit 1; }
fi

binary_url=${SBM_WEB_BINARY_URL:-"https://github.com/$REPO/releases/download/v$VERSION/sb-web-linux-$asset_arch"}
checksum_url=${SBM_WEB_CHECKSUM_URL:-"https://github.com/$REPO/releases/download/v$VERSION/SHA256SUMS"}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
if [[ -f "$binary_url" ]]; then
  cp -p "$binary_url" "$tmp/sb-web"
else
  curl --fail --silent --show-error --location "$binary_url" -o "$tmp/sb-web"
fi
chmod 0755 "$tmp/sb-web"
if [[ ${SBM_WEB_SKIP_VERIFY:-0} != 1 ]]; then
  if [[ -f "$checksum_url" ]]; then
    cp -p "$checksum_url" "$tmp/SHA256SUMS"
  else
    curl --fail --silent --show-error --location "$checksum_url" -o "$tmp/SHA256SUMS"
  fi
  expected=$(awk -v file="sb-web-linux-$asset_arch" '$2==file || $2=="*"file {print $1; exit}' "$tmp/SHA256SUMS")
  actual=$(sha256sum "$tmp/sb-web" | awk '{print $1}')
  [[ -n "$expected" && "$expected" == "$actual" ]] || { echo 'sb-web SHA256 校验失败。' >&2; exit 1; }
fi

mkdir -p "$LIB_DIR" "$BIN_DIR" "$ETC_DIR" "$VAR_DIR" "$LOG_DIR"
install -m 0755 "$tmp/sb-web" "$LIB_DIR/sb-web"
ln -sfn "$LIB_DIR/sb-web" "$BIN_DIR/sb-web"
if [[ ! -e "$ETC_DIR/config.json" ]]; then
  install -m 0600 /dev/stdin "$ETC_DIR/config.json" <<EOF_CONFIG
{
  "listen": "127.0.0.1:9091",
  "sb_path": "$(command -v sb)",
  "data_dir": "$VAR_DIR",
  "database": "$VAR_DIR/web.db",
  "log_dir": "$LOG_DIR",
  "tls": {"enabled": false, "cert_file": "", "key_file": ""},
  "agent": {"enabled": false, "controller_url": "", "identity_file": "$VAR_DIR/agent-identity/ed25519.json", "heartbeat_interval": "30s"},
  "tasks": {"default_timeout": "10m", "batch_concurrency": 1, "failure_stop_percent": 25}
}
EOF_CONFIG
  "$BIN_DIR/sb-web" init --config "$ETC_DIR/config.json"
fi
if [[ -d "$SYSTEMD_DIR" ]]; then
  install -m 0644 /dev/stdin "$SYSTEMD_DIR/sb-manager-web.service" <<EOF_SYSTEMD
[Unit]
Description=sb-manager Go WebUI
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$BIN_DIR/sb-web server --config $ETC_DIR/config.json
Restart=on-failure
RestartSec=3s
User=root
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full
ReadWritePaths=$ETC_DIR /etc/sb-manager /etc/sysctl.d $VAR_DIR /var/lib/sb-manager $LOG_DIR /var/log/sb-manager /run/sb-manager-web /run/sb-manager $LIB_DIR /usr/local/lib/sb-manager $BIN_DIR /usr/local/bin

[Install]
WantedBy=multi-user.target
EOF_SYSTEMD
  install -m 0644 /dev/stdin "$SYSTEMD_DIR/sb-manager-web-agent.service" <<EOF_AGENT_SYSTEMD
[Unit]
Description=sb-manager WebUI remote Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$BIN_DIR/sb-web agent --config $ETC_DIR/config.json
Restart=always
RestartSec=5s
User=root
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full
ReadWritePaths=$ETC_DIR /etc/sb-manager /etc/sysctl.d $VAR_DIR /var/lib/sb-manager $LOG_DIR /var/log/sb-manager /run/sb-manager-web /run/sb-manager $LIB_DIR /usr/local/lib/sb-manager $BIN_DIR /usr/local/bin

[Install]
WantedBy=multi-user.target
EOF_AGENT_SYSTEMD
fi
if [[ -d "$OPENRC_DIR" ]]; then
  install -m 0755 /dev/stdin "$OPENRC_DIR/sb-manager-web" <<EOF_OPENRC
#!/sbin/openrc-run
name="sb-manager-web"
description="sb-manager Go WebUI"
command="$BIN_DIR/sb-web"
command_args="server --config $ETC_DIR/config.json"
command_user="root:root"
supervisor="supervise-daemon"
supervise_daemon_args="--stdout /var/log/sb-manager-web/server.log --stderr /var/log/sb-manager-web/server.err.log"
output_log="/var/log/sb-manager-web/server.log"
error_log="/var/log/sb-manager-web/server.err.log"
pidfile="/run/\${RC_SVCNAME}.pid"
command_background="yes"
respawn_delay=3
depend() { need net; after firewall; }
EOF_OPENRC
  install -m 0755 /dev/stdin "$OPENRC_DIR/sb-manager-web-agent" <<EOF_AGENT_OPENRC
#!/sbin/openrc-run
name="sb-manager-web-agent"
description="sb-manager WebUI remote Agent"
command="$BIN_DIR/sb-web"
command_args="agent --config $ETC_DIR/config.json"
command_user="root:root"
supervisor="supervise-daemon"
supervise_daemon_args="--stdout /var/log/sb-manager-web/agent.log --stderr /var/log/sb-manager-web/agent.err.log"
output_log="/var/log/sb-manager-web/agent.log"
error_log="/var/log/sb-manager-web/agent.err.log"
pidfile="/run/\${RC_SVCNAME}.pid"
command_background="yes"
respawn_delay=5
depend() { need net; }
EOF_AGENT_OPENRC
fi
if [[ "$NO_START" == 0 ]]; then
  if [[ -d /run/systemd/system ]] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable --now sb-manager-web.service
  elif command -v rc-update >/dev/null 2>&1 && command -v rc-service >/dev/null 2>&1; then
    rc-update add sb-manager-web default || true
    rc-service sb-manager-web start
  else
    echo '未发现 systemd/OpenRC；请手动运行 sb-web server。' >&2
  fi
fi
echo "sb-manager-web $VERSION 安装完成。"
