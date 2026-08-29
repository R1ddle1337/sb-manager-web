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
GET /api/v1/logs
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

Node fields are allow-listed by the runner. Unknown JSON fields are rejected and no request is passed to a shell.

## Batch and tasks

```text
POST /api/v1/batch/actions
GET  /api/v1/tasks
GET  /api/v1/tasks/{task_id}
GET  /api/v1/audit
```

Batch request:

```json
{
  "server_ids": ["local", "srv_example"],
  "action": "bbr.enable",
  "args": {}
}
```

## Enrollment

```text
POST /api/v1/enrollment
```

Returns the raw token once, its expiry, and a complete `sb-web join` command. Only a SHA-256 hash is stored.

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
