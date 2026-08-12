# termup

Self-hosted uptime & synthetic monitoring — CLI-first, single project, two small
binaries. `termupd` continuously probes a list of targets, evaluates them, and
alerts on state changes; `termup` is a thin read-only client that renders a live
terminal dashboard by talking to the daemon.

## How it works

Two binaries in one repo (like `dockerd`/`docker`):

- **`termupd`** (daemon) — scheduler + HTTP prober + state machine + SQLite
  storage + notifiers. Serves a small read-only HTTP/JSON API over a local Unix
  socket.
- **`termup`** (CLI) — read-only client. It never probes; it connects to the
  daemon's API and shows the dashboard. Targets are managed by editing
  `config.yaml`, not by the CLI.

```
config.yaml ──► termupd ──► probe ──► state machine ──► notifiers
                   │                        │
                   └── SQLite (history) ────┴──► HTTP API ──► termup (TUI)
```

## Features

- **Config-as-code.** Targets live in `config.yaml` (single source of truth).
  The daemon reads it and hot-reloads on change; a broken edit keeps the previous
  list.
- **Up/down semantics.** Up = `2xx` only. `3xx`/`4xx`/`5xx` and no-response
  (timeout / refused / DNS / TLS error) are all down.
- **State machine with debounce.** A target flips to *down* only after **3
  consecutive** failures, and back to *up* on a single `2xx`.
- **Flapping detection.** If a target oscillates (≥5 up/down flips over the last
  10 probes), it fires a one-shot flapping alert the debounced machine would miss.
- **TLS cert expiry.** Each HTTPS probe records the certificate's expiry; an alert
  fires when it drops below **14 days**.
- **Per-stage latency.** `httptrace` breaks each request into DNS / TCP connect /
  TLS handshake / TTFB, shown in the dashboard detail panel.
- **Notifiers with fan-out.** stdout is always on; add Slack / Discord / generic
  webhook targets in config. One event is delivered to all of them.
- **Jitter scheduling.** Probes are spread across the interval (deterministic
  per-target phase) to avoid a thundering herd.
- **Retention.** Results older than 30 days are pruned hourly so storage does not
  grow without bound.
- **Terminal dashboard.** Gatus-style up/down bars per target, mouse-hover and
  keyboard navigation for per-check detail, and a filter box.

## Quick start

### With Docker (recommended)

```bash
make up        # build the image and run the daemon continuously
make logs      # follow probes / alerts
make watch     # attach the dashboard (inside the container)
make down      # stop (keeps the db volume; `down -v` wipes it)
make restart   # apply config.yaml changes
```

### From source

```bash
make build                 # -> bin/termupd, bin/termup
./bin/termupd              # terminal 1: the daemon (reads ./config.yaml)
./bin/termup watch         # terminal 2: the dashboard
```

Requires Go 1.26+. The daemon writes `termup.db` (SQLite) and listens on
`/tmp/termupd.sock`.

## Configuration

`config.yaml`:

```yaml
monitors:
  - name: example
    url: https://example.com
  - name: api
    url: https://api.example.com/health

# Optional. stdout is always on; these are added on top.
notifiers:
  - type: slack
    url: https://hooks.slack.com/services/XXX
  - type: discord
    url: https://discord.com/api/webhooks/XXX
  - type: webhook
    url: https://my.endpoint/hook
```

Monitor names must be unique and non-empty. Notifiers are applied at boot (not
hot-reloaded).

## CLI

The only command is `watch` (read-only):

```bash
termup watch
termup --addr unix:///tmp/termupd.sock watch   # or TERMUP_ADDR env var
```

Dashboard controls:

| Key | Action |
| --- | --- |
| mouse hover | show a bar's detail (time · status · latency · stage timings · error) |
| `Tab` / `Shift+Tab` | move between monitors |
| `←` / `→` | move between a monitor's checks |
| `/` | filter by name or url |
| `r` | refresh now |
| `q` | quit |

## API

Read-only, served over the Unix socket (JSON):

| Method | Path | Description |
| --- | --- | --- |
| GET | `/v1/monitors` | list configured monitors |
| GET | `/v1/monitors/:name/status` | a monitor's current (debounced) state |
| GET | `/v1/dashboard` | per-monitor health + recent checks |

```bash
curl --unix-socket /tmp/termupd.sock http://localhost/v1/dashboard
```

## Development

```bash
make test                  # go test ./... -race
LOG_LEVEL=debug ./bin/termupd   # more verbose logging (default: info)
```

Layout (feature-first, no `internal/`):

```
cmd/termupd, cmd/termup   entrypoints
monitor                   domain: Monitor/Result/State, state machine, cert & flap trackers
scheduler                 watchdog loop, jitter, worker pool, Clock
probe                     HTTP prober (httptrace stage timings)
storage                   Reader/Writer interfaces + SQLite and in-memory impls
notify                    Notifier port + stdout / slack / discord / webhook / fan-out
config                    config.yaml parse, validate, hot-reload watcher
api                       HTTP/JSON boundary (fiber) + DTOs
server                    wiring: config → scheduler + API + retention
```

## Scope

- **No web UI, no auth** (in any phase). The only front end is the `termup` CLI.
  Security is at the transport layer: a local Unix socket where file permissions
  (`0600`) act as auth. If the daemon is ever exposed to the network, protection
  belongs at the edge (VPN / reverse proxy).
- Out of scope: distributed agents, multi-region, per-monitor intervals,
  multi-step synthetic flows.
