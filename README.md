# sb-manager-web

基于 Go 的 sing-box Web 控制层。它建立在 [`sb-manager`](https://github.com/R1ddle1337/sb-manager) 之上，不重写协议渲染、状态迁移、服务管理、校验或回滚逻辑。

## 当前状态

已实现第一版可运行基础：

- Go 静态 WebUI，页面资源嵌入二进制
- 登录、Argon2id 密码、Session、CSRF
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
- systemd/OpenRC 服务文件
- root helper Unix Socket 权限隔离（Web 服务使用 `sbweb` 用户）
- amd64/arm64/armv7 交叉编译
- 参考 Cli-Proxy-API-Management-Center 信息架构的响应式控制台（侧栏、KPI、工作台和审计区）

前端布局、视觉规范和后续迭代边界见 [`docs/FRONTEND_DESIGN.md`](docs/FRONTEND_DESIGN.md)。

完整开发设计当前维护在 [`sb-manager/docs/SB_MANAGER_WEB_DEVELOPMENT.md`](https://github.com/R1ddle1337/sb-manager/blob/main/docs/SB_MANAGER_WEB_DEVELOPMENT.md)，新项目稳定后会同步维护本项目自身的架构文档。

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

服务器必须先安装 `sb-manager`：

```bash
curl -fsSL https://github.com/R1ddle1337/sb-manager-web/raw/main/install.sh | sudo bash
sb-web enable
```

安装器会校验发布包 SHA256，并根据 systemd/OpenRC 安装服务。当前预览版本为 `0.1.0-alpha.4`，开发时可以使用：

```bash
SBM_WEB_BINARY_URL=/path/to/sb-web SBM_WEB_SKIP_VERIFY=1 sudo -E bash install.sh --no-start
```

## Agent 加入

控制端登录后创建一次性 enrollment token，然后在新服务器执行：

```bash
sb-web join https://panel.example.com TOKEN
```

Agent 主动连接控制端，不需要开放远程 SSH 管理端口。长期身份由每台服务器独立 Ed25519 私钥提供，私钥保存在本机。
