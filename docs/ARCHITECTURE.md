# Architecture

## Boundary

`sb-manager-web` is a control plane for `sb-manager`. It does not own sing-box state and never edits generated `config.json`.

```text
Browser
  -> authenticated Go HTTP API
  -> fixed action/task model
  -> local runner or signed remote Agent channel
  -> sb CLI
  -> sb-manager transaction/check/rollback
```

The source of truth remains `/etc/sb-manager/state.json` and its protected secret/certificate files.

## Components

- `cmd/sb-web`: server, Agent, join, service and password commands.
- `internal/api`: HTML/API transport, authentication gates, CSRF and Agent endpoints.
- `internal/runner`: the only mapping from Web actions to `sb` arguments. It never invokes a shell.
- `internal/agent`: one-time enrollment, Ed25519 identity, heartbeat and task execution.
- `internal/storage`: bbolt users, sessions, servers, tasks, enrollments and audit events.
- `web`: embedded server-rendered HTML, CSS and JavaScript.

## Local mode

The local server is stored as ID `local`. Read actions execute immediately; mutations create a task and execute the matching CLI action asynchronously. The browser polls tasks to render the result.

The first release runs the Web service as root because the existing `sb` CLI owns root-only state and system services. Risk is constrained by:

- loopback-only default listener;
- login, session and CSRF checks;
- no terminal or generic command endpoint;
- `exec.CommandContext` with argument arrays;
- strict action/field validation;
- service sandboxing;
- bounded command output and timeouts.

A later privilege split can place the HTTP server behind a restricted Unix-socket helper without changing the public API.

## Remote Agent mode

The controller creates a random, single-use enrollment token valid for ten minutes. The Agent:

1. creates an Ed25519 key on the managed server;
2. sends only its public key with the token;
3. receives a stable server ID;
4. signs every heartbeat, poll and result request;
5. keeps its private key in a mode-0600 local identity file.

Transport uses normal HTTPS certificate verification. Application request signatures bind method, path, timestamp and exact body. The controller rejects requests outside a two-minute window. Removing the server record revokes that identity.

No node password, certificate private key, Snell PSK or Realm token is included in heartbeat data.

## Task model

Tasks have a stable ID, target server, fixed action, structured arguments, status, timestamps and optional idempotency key.

```text
pending -> running -> success|failed
```

The controller atomically claims a pending remote task. An Agent can only submit results for its own task. Local and remote output is truncated and secret JSON fields are redacted before persistence.

Batch actions create one task per eligible server. This intentionally does not pretend that cross-server changes are atomic. Each target retains sb-manager's local transaction and rollback behavior.

## Storage

The controller uses a single bbolt database with these buckets:

- users
- sessions
- servers
- tasks
- enrollments
- audit

Agent private keys are not in the controller database. Node secrets remain exclusively in sb-manager storage on their target server.

## Network defaults

- Web listener: `127.0.0.1:9091`
- Remote Agent: outbound HTTPS to the configured controller URL
- No installer firewall changes
- No inbound Agent port

Public access should use an existing HTTPS reverse proxy or Cloudflare Tunnel. Binding the root Web service directly to a public address without TLS is unsupported.

## Compatibility

The WebUI checks sb-manager through its public CLI. Features unavailable on an older target should be rejected by sb-manager and surfaced as task failures. The runner does not reproduce protocol defaults; sb-manager remains authoritative.
