# sb-manager-web

基于 Go 的 sing-box Web 控制层。它建立在 [`sb-manager`](https://github.com/R1ddle1337/sb-manager) 之上，不重写协议渲染、状态迁移、服务管理、校验或回滚逻辑。

## 当前状态

已实现第一版可运行基础：

- Go 静态 WebUI，页面资源嵌入二进制
- 登录、Argon2id 密码、Session、CSRF
- 首次安装随机管理员用户名和随机密码（不使用固定 `admin` 账号名）
- TOTP 双因素认证、一次性恢复码和 API Token（指标读取）
- sb-manager 备份列表、上传与恢复向导（128 MiB 上限）
- 本机 `sb` CLI 安全调用和命令白名单
- 状态、节点、核心能力、BBR、Hysteria2 UDP 缓冲区查看
- 协议节点创建覆盖 VMess、Shadowsocks、AnyTLS、Hysteria2、Trojan、TUIC、VLESS、NaiveProxy、ShadowTLS、Snell v5/v6；含 1.14 Gecko/指纹/BBR profile 及 Hysteria Realm 参数
- BBR/HY2/core 操作任务和任务状态
- SQLite 控制端元数据（WAL、单文件、可直接备份）
- 一次性 enrollment token
- Agent Ed25519 身份、心跳、轮询任务和结果上报
- 多服务器清单、批量 BBR/HY2/核心/备份/健康任务、失败阈值和审计记录
- 任务取消/重试、状态摘要漂移保护、Agent mTLS（可选强制）和证书自动轮换
- 节点新增/编辑/启停/删除、用户、分享链接和证书操作 API
- 服务器/节点详情页，以及 admin/operator/viewer 角色接口
- SQLite 在线备份、数据库健康检查和主动/被动控制端部署路径
- 任务筛选/详情抽屉、灰度进度统计、浅色主题和自动刷新
- 运维中心：健康/配置校验、日志、流量策略、防火墙、服务重启、分享和订阅
- systemd/OpenRC 服务文件
- root helper Unix Socket 权限隔离（Web 服务使用 `sbweb` 用户）
- amd64/arm64/armv7 交叉编译
- 参考 Cli-Proxy-API-Management-Center 信息架构的响应式控制台（侧栏、KPI、工作台和审计区）

前端布局、视觉规范和后续迭代边界见 [`docs/FRONTEND_DESIGN.md`](docs/FRONTEND_DESIGN.md)。

项目边界、组件和部署模型由本仓库的 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) 独立维护。

## 开发

要求 Go 1.18 或更高版本：

```bash
gofmt -w cmd internal web
go test ./...
go vet ./...
make release
```

运行本机面板：

```bash
go run ./cmd/sb-web init --config /tmp/sb-web/config.json
go run ./cmd/sb-web server --config /tmp/sb-web/config.json
```

默认监听 `127.0.0.1:9091`。生产环境请使用 HTTPS、反向代理或 Cloudflare Tunnel，不要直接把 root Web 服务暴露到公网。

## 安装

直接运行 Web 一键安装：

```bash
curl -fsSL https://github.com/R1ddle1337/sb-manager-web/raw/main/install.sh | sudo bash
```

如果服务器尚未安装 `sb-manager`，Web 安装器会下载并调用 [`sb-manager`](https://github.com/R1ddle1337/sb-manager) 独立仓库的官方安装器；不会把它的源码、服务定义或业务逻辑复制到 Web 项目。可以用 `SBM_INSTALL_REF`/`SBM_INSTALL_SHA256` 固定上游版本，用 `SBM_WEB_SB_INSTALL_URL` 指定内部镜像，或用 `SBM_WEB_AUTO_INSTALL_SB=0` 要求预先供应依赖。

安装器会校验 Web 发布包 SHA256，并且只为当前实际运行的 systemd 或 OpenRC 生成服务。当前预览版本为 `0.1.0-alpha.17`，开发时可以使用：

```bash
SBM_WEB_BINARY_URL=/path/to/sb-web SBM_WEB_SKIP_VERIFY=1 sudo -E bash install.sh --no-start
```

首次在 SSH 中安装时，安装器会显示管理员账号、密码和面板访问地址；重复安装只显示已有账号，并提示使用 `sb-web reset-admin-password` 生成新密码。默认面板只监听 `127.0.0.1:9091`，输出中会同时给出 SSH 隧道命令；如需直接监听公网地址，可显式设置 `--panel-listen 0.0.0.0:9091`，并自行放行防火墙端口。

安装器也提供面板 HTTPS 证书流程。交互式 SSH 安装会显示选择菜单；非交互安装可以直接指定模式：

```bash
# IP 或内网使用自签名证书
curl -fsSL https://github.com/R1ddle1337/sb-manager-web/raw/main/install.sh | \
  sudo bash -s -- --panel-tls self-signed --panel-domain 203.0.113.10

# Let's Encrypt HTTP-01，域名或已获支持的公网 IP，要求 80 端口可达
sudo bash install.sh --panel-tls acme-http --panel-domain panel.example.com --panel-email admin@example.com

# Let's Encrypt DNS-01（Cloudflare）
sudo bash install.sh --panel-tls acme-dns-cloudflare --panel-domain panel.example.com \
  --panel-email admin@example.com --panel-cf-token 'TOKEN'

# 导入已有证书链和私钥（交互安装选择此项时也会询问这两个绝对路径）
sudo bash install.sh --panel-tls existing --panel-cert /path/fullchain.pem --panel-key /path/key.pem
```

安装器默认会启动面板；如果使用了 `--no-start`，再运行 `sb-web enable`。面板日常操作统一使用 `sb-web enable|restart|logs`；独立的 `sb` 命令只用于底层 sing-box 节点和证书管理，不是面板启动命令。

## Agent 加入

控制端登录后创建一次性 enrollment token，然后在新服务器执行：

```bash
curl -fsSL https://github.com/R1ddle1337/sb-manager-web/raw/main/install.sh | sudo bash -s -- --agent https://panel.example.com TOKEN
```

面板生成的命令会包含确切 Release 安装参数；Agent 主动连接控制端，不需要开放远程 SSH 管理端口。长期身份由每台服务器独立 Ed25519 私钥提供，私钥保存在本机。也可以先安装后手动执行 `sb-web join`。
