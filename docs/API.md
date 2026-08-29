# HTTP API v1

All browser APIs require a valid `sbweb_session` cookie. Mutating browser requests also require the `X-CSRF-Token` header matching the session CSRF token.

需要 root 的操作由本机 `sb-manager-web-helper` Unix Socket 执行；Web 服务进程本身不应直接拥有修改 sb-manager 系统文件的权限。

## Session

```text
POST /login
POST /logout
GET  /api/v1/session
POST /api/v1/password
```

## Local read endpoints

```text
GET /api/v1/status
GET /api/v1/nodes
GET /api/v1/capabilities
GET /api/v1/bbr/status
GET /api/v1/hy2-buffer/status
GET /api/v1/realm
GET /api/v1/logs
GET /api/v1/traffic
GET /api/v1/health
GET /api/v1/firewall
GET /api/v1/firewall/ports
GET /api/v1/mux
GET /api/v1/tunnel
GET /api/v1/notify
GET /api/v1/settings
GET /api/v1/config/validate
GET /api/v1/config/diff
GET /api/v1/logs?target=all&lines=200
GET /api/v1/certificates/{domain}
POST /api/v1/certificates/cloudflare
GET /api/v1/metrics
GET /api/v1/shares
GET /api/v1/exports/outbounds
GET /api/v1/subscriptions
GET /api/v1/subscriptions/status
POST /api/v1/subscriptions
DELETE /api/v1/subscriptions
```

## Servers

```text
GET  /api/v1/servers
GET  /api/v1/servers/{server_id}
GET  /api/v1/servers/{server_id}/status
GET  /api/v1/servers/{server_id}/capabilities
GET  /api/v1/servers/{server_id}/nodes
POST /api/v1/servers/{server_id}/actions
```

Action request:

```json
{
  "action": "hy2-buffer.enable",
  "args": {},
  "idempotency_key": "optional-stable-key"
}
```

Supported system actions include status/capabilities reads plus:

- `bbr.enable`, `bbr.disable`
- `hy2-buffer.enable`, `hy2-buffer.disable`
- `realm.enable`, `realm.disable`, `realm.status`
- `node.enable-all`, `node.disable-all`, `node.rotate`, `share.all`
- `subscription.create`, `subscription.list`, `subscription.status`, `subscription.revoke`
- `traffic.status`, `traffic.set`, `traffic.disable`, `traffic.remove`, `traffic.reset`, `traffic.reconcile`
- `health.status`, `health.enable`, `health.disable`, `health.configure`
- `config.validate`, `config.diff`, `firewall.status`, `firewall.ports`, `firewall.ufw-allow`
- `mux.status`, `mux.enable`, `mux.disable`, `mux.route.*`, `tunnel.*`, `notify.*`, `settings.*`
- `core.check`, `core.update`, `core.rollback`
- `backup.create`
- `health.check`
- `doctor`, `doctor.repair-safe`

## Nodes

```text
POST  /api/v1/servers/{server_id}/nodes
PATCH /api/v1/servers/{server_id}/nodes/{node_id}
GET   /api/v1/servers/{server_id}/nodes/{node_id}
POST  /api/v1/servers/{server_id}/nodes/{node_id}/enable
POST  /api/v1/servers/{server_id}/nodes/{node_id}/disable
POST  /api/v1/servers/{server_id}/nodes/{node_id}/delete
GET   /api/v1/servers/{server_id}/nodes/{node_id}/share
GET   /api/v1/servers/{server_id}/nodes/{node_id}/users
POST  /api/v1/servers/{server_id}/nodes/{node_id}/users
POST  /api/v1/servers/{server_id}/nodes/{node_id}/users/{user_id}/{enable|disable|delete|rotate}
```

Node fields are allow-listed by the runner. Unknown JSON fields are ignored by the command mapper and no request is passed to a shell. The mapper covers the 1.14-era Hysteria2 options (Gecko, Chrome QUIC fingerprint switch, BBR profile/debug and Realm binding), Snell v5/v6 options, and the same protocol-specific fields for `PATCH` edits. The underlying `sb-manager node set` path validates and applies these fields transactionally.

Realm enable arguments are structured and validated before task creation:

```json
{
  "action": "realm.enable",
  "args": {
    "port": 9443,
    "public_url": "https://relay.example.com",
    "listen": "::",
    "tls_domain": "relay.example.com",
    "max_realms": 0
  }
}
```

The Realm token is generated and stored by `sb-manager` on the target server. It is never stored in the controller SQLite database.

## Batch and tasks

```text
POST /api/v1/batch/actions
POST /api/v1/batch/preflight
GET  /api/v1/tasks
GET  /api/v1/tasks/{task_id}
POST /api/v1/tasks/{task_id}/cancel
POST /api/v1/tasks/{task_id}/retry
GET  /api/v1/audit
GET  /api/v1/users
POST /api/v1/users
PATCH /api/v1/users/{username}
DELETE /api/v1/users/{username}
POST /api/v1/database/backup
```

Batch request:

```json
{
  "server_ids": ["local", "srv_example"],
  "action": "bbr.enable",
  "args": {},
  "strategy": "percentage",
  "percentage": 25
}
```

`strategy` can be `all`, `canary` (first eligible server only), or
`percentage` (deterministic prefix of eligible servers). The response returns
the selected strategy and creates one independently observable task per
selected server.

Task lifecycle endpoints:

- `POST /api/v1/tasks/{id}/cancel` marks pending work canceled; running work is
  marked for cancellation and its eventual result is recorded as canceled.
- `POST /api/v1/tasks/{id}/retry` creates a new task with a new idempotency key,
  incremented attempt number, and the latest remote state digest.

On controller restart, tasks left in `running` are requeued as `pending`.

## Roles

The installer creates a random username with the `admin` role and a random
password. An `admin` role can manage users and all operations. `operator` can
execute server/node tasks but cannot enroll servers or manage accounts;
`viewer` can read status, capabilities, tasks and audit data only.

## Enrollment

```text
POST /api/v1/enrollment
```

Returns the raw token once, its expiry, and a complete one-command installer
(`curl ... install.sh --agent ...`). Only a SHA-256 hash is stored. The
installer performs registration and starts the Agent service; no second manual
step is required.

## Agent endpoints

```text
POST /api/v1/agent/register
POST /api/v1/agent/heartbeat
POST /api/v1/agent/poll
POST /api/v1/agent/result
```

Except for registration, Agent requests use:

```text
X-Agent-ID
X-Agent-Timestamp
X-Agent-Signature
```

The signature is Ed25519 over:

```text
METHOD + "\n" + PATH + "\n" + UNIX_TIMESTAMP + "\n" + EXACT_BODY
```

For deployments that enable `tls.require_agent_mtls`, registration additionally
returns a client certificate signed by the controller's local Agent CA. The
certificate and private key are saved only in the Agent identity file. Agent
heartbeat/poll/result requests must then present that certificate and still
pass the Ed25519 signature check.

Agent heartbeats include a SHA-256 digest and schema number for the local
`state.json`. Mutating remote tasks carry the digest observed at enqueue time;
an Agent refuses to execute when the local digest has changed, preventing a
central task from overwriting a local CLI change.

## Errors

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "invalid node id",
    "details": null
  }
}
```

Common codes include `AUTH_REQUIRED`, `AUTH_FAILED`, `CSRF_FAILED`, `VALIDATION_FAILED`, `AGENT_OFFLINE`, `COMMAND_TIMEOUT`, `COMMAND_FAILED`, `STORAGE_ERROR` and `NOT_FOUND`.
