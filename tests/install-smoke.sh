#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT
mkdir -p "$ROOT/bin" "$ROOT/etc/systemd/system" "$ROOT/etc/init.d"
cat >"$ROOT/fake-sb-installer" <<'EOF_INSTALLER'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ "${1:-}" == --no-menu ]]
cat >"$SBM_TEST_SB_TARGET" <<'EOF_SB'
#!/usr/bin/env sh
if [ "${1:-}" = version ]; then echo 'sb-manager 0.1.0-alpha.27'; else echo '{}'; fi
EOF_SB
chmod +x "$SBM_TEST_SB_TARGET"
touch "$SBM_TEST_SB_MARKER"
EOF_INSTALLER
chmod +x "$ROOT/fake-sb-installer"
for service in sb-manager-web sb-manager-web-helper sb-manager-web-agent; do
  cat >"$ROOT/etc/init.d/$service" <<'EOF_LEGACY'
#!/sbin/openrc-run
description="sb-manager Go WebUI legacy service"
command="/usr/local/bin/sb-web"
EOF_LEGACY
done
CGO_ENABLED=0 go build -trimpath -o "$ROOT/sb-web" "$PROJECT/cmd/sb-web"
PATH="$ROOT/bin:/usr/bin:/bin" \
SBM_WEB_VERSION=0.1.0 \
SBM_WEB_BINARY_URL="$ROOT/sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_SB_INSTALL_URL="$ROOT/fake-sb-installer" \
SBM_TEST_SB_TARGET="$ROOT/bin/sb" \
SBM_TEST_SB_MARKER="$ROOT/sb-manager-installed" \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_INIT_SYSTEM=systemd \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start
test -f "$ROOT/sb-manager-installed"
test -x "$ROOT/usr/local/bin/sb-web"
test -f "$ROOT/etc/sb-manager-web/config.json"
test -f "$ROOT/var/lib/sb-manager-web/web.db"
test -f "$ROOT/etc/systemd/system/sb-manager-web.service"
test -f "$ROOT/etc/systemd/system/sb-manager-web-helper.service"
test ! -e "$ROOT/etc/init.d/sb-manager-web"
test ! -e "$ROOT/etc/init.d/sb-manager-web-helper"
test ! -e "$ROOT/etc/init.d/sb-manager-web-agent"
grep -Fq "$ROOT/usr/local/bin/sb-web" "$ROOT/etc/systemd/system/sb-manager-web.service"
jq -e --arg dir "$ROOT/var/lib/sb-manager-web" '.data_dir==$dir and .database==($dir+"/web.db") and .backup_dir==($dir+"/backups")' "$ROOT/etc/sb-manager-web/config.json" >/dev/null
[[ $(stat -c '%U' "$ROOT/var/lib/sb-manager-web") == daemon ]]
grep -Fq 'User=daemon' "$ROOT/etc/systemd/system/sb-manager-web.service"
"$ROOT/usr/local/bin/sb-web" version | grep -Fq 'sb-manager-web'

repeat_output=$(PATH="$ROOT/bin:/usr/bin:/bin" \
  SBM_WEB_VERSION=0.1.0 \
  SBM_WEB_BINARY_URL="$ROOT/sb-web" \
  SBM_WEB_SKIP_VERIFY=1 \
  SBM_WEB_PREFIX="$ROOT/usr/local" \
  SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
  SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
  SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
  SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
  SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
  SBM_WEB_INIT_SYSTEM=systemd \
  SBM_WEB_SERVICE_USER=daemon \
  bash "$PROJECT/install.sh" --no-start)
grep -Eq '^管理员账号：owner-' <<<"$repeat_output"
grep -Fq '管理员密码已存在且不会回显' <<<"$repeat_output"

previous_hash=$(sha256sum "$ROOT/usr/local/lib/sb-manager-web/sb-web" | awk '{print $1}')
rm -f "$ROOT/etc/sb-manager-web/config.json"
cat >"$ROOT/failing-sb-web" <<'EOF_FAILING'
#!/usr/bin/env sh
exit 42
EOF_FAILING
chmod +x "$ROOT/failing-sb-web"
set +e
PATH="$ROOT/bin:$PATH" \
SBM_WEB_VERSION=broken \
SBM_WEB_BINARY_URL="$ROOT/failing-sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_INIT_SYSTEM=systemd \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start >/dev/null 2>&1
failed_rc=$?
set -e
[[ "$failed_rc" != 0 ]]
[[ $(sha256sum "$ROOT/usr/local/lib/sb-manager-web/sb-web" | awk '{print $1}') == "$previous_hash" ]]

PATH="$ROOT/bin:/usr/bin:/bin" \
SBM_WEB_VERSION=0.1.0 \
SBM_WEB_BINARY_URL="$ROOT/sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_TLS_MODE=self-signed \
SBM_WEB_TLS_DOMAIN=127.0.0.1 \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_INIT_SYSTEM=systemd \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start >/dev/null
test -s "$ROOT/etc/sb-manager-web/tls/fullchain.pem"
test -s "$ROOT/etc/sb-manager-web/tls/key.pem"
jq -e --arg cert "$ROOT/etc/sb-manager-web/tls/fullchain.pem" --arg key "$ROOT/etc/sb-manager-web/tls/key.pem" '.tls.enabled == true and .tls.cert_file == $cert and .tls.key_file == $key' "$ROOT/etc/sb-manager-web/config.json" >/dev/null
openssl x509 -in "$ROOT/etc/sb-manager-web/tls/fullchain.pem" -noout -checkip 127.0.0.1

PATH="$ROOT/bin:/usr/bin:/bin" \
SBM_WEB_VERSION=0.1.0 \
SBM_WEB_BINARY_URL="$ROOT/sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_TLS_MODE=existing \
SBM_WEB_TLS_CERT_FILE="$ROOT/etc/sb-manager-web/tls/fullchain.pem" \
SBM_WEB_TLS_KEY_FILE="$ROOT/etc/sb-manager-web/tls/key.pem" \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_INIT_SYSTEM=systemd \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start >/dev/null
test -s "$ROOT/etc/sb-manager-web/tls/fullchain.pem"
test -s "$ROOT/etc/sb-manager-web/tls/key.pem"

cat >"$ROOT/acme.sh" <<'EOF_ACME'
#!/usr/bin/env bash
set -Eeuo pipefail
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
  if [[ "${args[$i]}" == --install-cert ]]; then
    cert=''; key=''
    for ((j=i+1; j<${#args[@]}; j++)); do
      case "${args[$j]}" in
        --fullchain-file) cert=${args[$((j+1))]} ;;
        --key-file) key=${args[$((j+1))]} ;;
      esac
    done
    cp "$SBM_TEST_CERT_SOURCE" "$cert"
    cp "$SBM_TEST_KEY_SOURCE" "$key"
    exit 0
  fi
done
EOF_ACME
chmod +x "$ROOT/acme.sh"
mkdir -p "$ROOT/acme-home"
cp -p "$ROOT/acme.sh" "$ROOT/acme-home/acme.sh"
PATH="$ROOT/bin:/usr/bin:/bin" \
SBM_WEB_VERSION=0.1.0 \
SBM_WEB_BINARY_URL="$ROOT/sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_TLS_MODE=acme-http \
SBM_WEB_TLS_DOMAIN=127.0.0.1 \
SBM_WEB_TLS_EMAIL=admin@example.com \
SBM_WEB_LISTEN=0.0.0.0:9091 \
SBM_WEB_ACME_HOME="$ROOT/acme-home" \
SBM_TEST_CERT_SOURCE="$ROOT/etc/sb-manager-web/tls/fullchain.pem" \
SBM_TEST_KEY_SOURCE="$ROOT/etc/sb-manager-web/tls/key.pem" \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_PERIODIC_DIR="$ROOT/etc/periodic" \
SBM_WEB_INIT_SYSTEM=systemd \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start >/dev/null
test -f "$ROOT/acme-home/panel-certificate"
test -f "$ROOT/etc/systemd/system/sb-manager-web-cert-renew.service"
test -f "$ROOT/etc/systemd/system/sb-manager-web-cert-renew.timer"
jq -e '.listen == "0.0.0.0:9091"' "$ROOT/etc/sb-manager-web/config.json" >/dev/null

PATH="$ROOT/bin:/usr/bin:/bin" \
SBM_WEB_VERSION=0.1.0 \
SBM_WEB_BINARY_URL="$ROOT/sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_PERIODIC_DIR="$ROOT/etc/periodic" \
SBM_WEB_INIT_SYSTEM=openrc \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start >/dev/null
test -x "$ROOT/etc/init.d/sb-manager-web"
test -x "$ROOT/etc/init.d/sb-manager-web-helper"
printf 'INSTALL SMOKE PASSED\n'
