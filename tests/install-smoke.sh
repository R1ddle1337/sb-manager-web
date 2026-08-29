#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT
mkdir -p "$ROOT/bin" "$ROOT/etc/systemd/system" "$ROOT/etc/init.d"
cat >"$ROOT/bin/sb" <<'EOF_SB'
#!/usr/bin/env sh
if [ "${1:-}" = version ]; then echo 'sb-manager 0.1.0-alpha.27'; else echo '{}'; fi
EOF_SB
chmod +x "$ROOT/bin/sb"
CGO_ENABLED=0 go build -trimpath -o "$ROOT/sb-web" "$PROJECT/cmd/sb-web"
PATH="$ROOT/bin:$PATH" \
SBM_WEB_VERSION=0.1.0 \
SBM_WEB_BINARY_URL="$ROOT/sb-web" \
SBM_WEB_SKIP_VERIFY=1 \
SBM_WEB_PREFIX="$ROOT/usr/local" \
SBM_WEB_ETC="$ROOT/etc/sb-manager-web" \
SBM_WEB_VAR="$ROOT/var/lib/sb-manager-web" \
SBM_WEB_LOG="$ROOT/var/log/sb-manager-web" \
SBM_WEB_SYSTEMD_DIR="$ROOT/etc/systemd/system" \
SBM_WEB_OPENRC_DIR="$ROOT/etc/init.d" \
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start
test -x "$ROOT/usr/local/bin/sb-web"
test -f "$ROOT/etc/sb-manager-web/config.json"
test -f "$ROOT/var/lib/sb-manager-web/web.db"
test -f "$ROOT/etc/systemd/system/sb-manager-web.service"
test -f "$ROOT/etc/systemd/system/sb-manager-web-helper.service"
test -x "$ROOT/etc/init.d/sb-manager-web"
test -x "$ROOT/etc/init.d/sb-manager-web-helper"
grep -Fq "$ROOT/usr/local/bin/sb-web" "$ROOT/etc/systemd/system/sb-manager-web.service"
jq -e --arg dir "$ROOT/var/lib/sb-manager-web" '.data_dir==$dir and .database==($dir+"/web.db")' "$ROOT/etc/sb-manager-web/config.json" >/dev/null
[[ $(stat -c '%U' "$ROOT/var/lib/sb-manager-web") == daemon ]]
grep -Fq 'User=daemon' "$ROOT/etc/systemd/system/sb-manager-web.service"
"$ROOT/usr/local/bin/sb-web" version | grep -Fq 'sb-manager-web'

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
SBM_WEB_SERVICE_USER=daemon \
bash "$PROJECT/install.sh" --no-start >/dev/null 2>&1
failed_rc=$?
set -e
[[ "$failed_rc" != 0 ]]
[[ $(sha256sum "$ROOT/usr/local/lib/sb-manager-web/sb-web" | awk '{print $1}') == "$previous_hash" ]]
printf 'INSTALL SMOKE PASSED\n'
