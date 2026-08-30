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

The repositories and release lifecycles remain separate. If `sb` is missing,
the Web bootstrap downloads and invokes sb-manager's public installer; it does
not vendor sb-manager source, service definitions, or installation logic. A
deployment can pin that upstream installer independently or provision `sb`
before running the Web installer.

`sb-web` is the Web control-plane command: with no arguments it opens the
interactive service/TLS/password/uninstall menu, while `sb-web server` is the
long-running process used by systemd/OpenRC. Web uninstall removes only Web
artifacts by default and never invokes the separate sb-manager uninstall path.

On first initialization the controller creates a random owner username (with
the `admin` role) and a random password; neither the username nor password is
hard-coded to `admin`. The credentials are printed once by `sb-web init` or
`sb-web server` and can be rotated with `sb-web reset-admin-password`.

## Components

- `cmd/sb-web`: server, Agent, join, service and password commands.
- `internal/api`: HTML/API transport, authentication gates, CSRF and Agent endpoints.
- `internal/runner`: the only mapping from Web actions to `sb` arguments. It never invokes a shell.
- `internal/agent`: one-time enrollment, Ed25519 identity, heartbeat and task execution.
- `internal/storage`: SQLite users, sessions, servers, tasks, enrollments, audit events, API tokens and metric samples.
- `web`: embedded server-rendered HTML, CSS and JavaScript.

## Local mode

The local server is stored as ID `local`. Read actions execute immediately; mutations create a task and execute the matching CLI action asynchronously. The browser polls tasks to render the result.

The Web service runs as the unprivileged `sbweb` user. A separate root helper owns a mode-0660 Unix socket and is used only when a CLI action needs root access. If the helper is unavailable, the WebUI does not silently elevate and the task fails. Risk is constrained by:

- loopback-only default listener;
- login, session and CSRF checks;
- no terminal or generic command endpoint;
- `exec.CommandContext` with argument arrays;
- strict action/field validation;
- service sandboxing;
- bounded command output and timeouts.

The installer can configure the panel's own HTTPS listener independently of
node certificates: a self-signed certificate (including IP SANs), ACME
HTTP-01 for a domain or supported IP identifier, ACME DNS-01 through
Cloudflare, or an existing certificate/key pair. Panel keys are stored under
the Web configuration directory with service-user read permission; ACME
account material remains root-only, and renewal is handled by the generated
systemd timer/OpenRC periodic job.

To turn on enforced Agent mTLS, set `tls.enabled=true`, provide the WebUI
server certificate/key, and set `tls.require_agent_mtls=true`. If
`client_ca_file` and `client_ca_key_file` are empty, the controller creates a
dedicated ten-year Agent CA under its data directory with mode `0600` for the
key. This CA is separate from the public HTTPS server certificate.

The public API is unchanged whether an action is handled by the helper or by a development-mode direct runner.

## Remote Agent mode

The controller creates a random, single-use enrollment token valid for ten minutes. The Agent:

1. creates an Ed25519 key on the managed server;
2. sends only its public key with the token;
3. receives a stable server ID;
4. signs every heartbeat, poll and result request;
5. keeps its private key in a mode-0600 local identity file.

Transport uses normal HTTPS certificate verification. Application request signatures bind method, path, timestamp and exact body. The controller rejects requests outside a two-minute window. Deployments can set `tls.require_agent_mtls=true`; registration then bootstraps a controller-signed client certificate and Agent requests must pass both mTLS and Ed25519 checks. Removing the server record revokes that identity.

Each heartbeat also carries a SHA-256 digest of the target's state file. Mutating tasks include the digest observed by the controller; the Agent fails a task instead of applying it when the state has drifted locally.

No node password, certificate private key, Snell PSK or Realm token is included in heartbeat data.
The heartbeat may include a non-secret node configuration snapshot (protocol,
ports and 1.14 options) so remote detail pages can edit the same fields; user
credentials and secret files are still excluded.

## Task model

Tasks have a stable ID, target server, fixed action, structured arguments, status, timestamps and optional idempotency key.

```text
pending -> running -> success|failed
```

Pending and running tasks can be canceled. Failed/canceled tasks can be retried
with a new idempotency key and attempt number; a running subprocess is allowed
to finish safely and its result is recorded as canceled when requested.
On controller startup, any task left in `running` is atomically requeued as
`pending`, so a process restart does not strand work claimed before the crash.

The UI also exposes dedicated server and node detail routes (`/servers/{id}`
and `/servers/{id}/nodes/{node}`) while keeping the single-page overview for
first-time users.

The controller atomically claims a pending remote task. An Agent can only submit results for its own task. Local and remote output is truncated and secret JSON fields are redacted before persistence.

Batch actions create one task per eligible server. This intentionally does not pretend that cross-server changes are atomic. Each target retains sb-manager's local transaction and rollback behavior.

## Storage

The controller uses a single SQLite database with these tables:

- `users`
- `sessions`
- `servers`
- `tasks`
- `enrollments`
- `audit`

Agent private keys are not in the controller database. Node secrets remain exclusively in sb-manager storage on their target server.

SQLite is deliberately the single-writer control-plane store. High availability
uses an active/passive deployment (shared or replicated filesystem plus an
external VIP/reverse-proxy health check), not concurrent active writers. The
`/healthz` endpoint is suitable for the proxy's liveness check, and the WAL
database can be copied while the service is stopped for a deterministic
failover snapshot.

## Network defaults

- Web listener: `127.0.0.1:9091`
- Remote Agent: outbound HTTPS to the configured controller URL
- No installer firewall changes
- No inbound Agent port

Public access should use an existing HTTPS reverse proxy or Cloudflare Tunnel. Binding the root Web service directly to a public address without TLS is unsupported.

## Compatibility

The WebUI checks sb-manager through its public CLI. Features unavailable on an older target should be rejected by sb-manager and surfaced as task failures. The runner does not reproduce protocol defaults; sb-manager remains authoritative.
