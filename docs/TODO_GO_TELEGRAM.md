# TODO: Go Telegram Handling

Telegram delivery is being restored in Go in small pieces. Settings, Bot API
delivery, panel report notifications, manual backup delivery, and scheduled
backup delivery are Go-native again. Telegram bot commands and webhook delivery
are still TODO.

## Current Decision

- Go report delivery is best-effort. Telegram errors must never roll back or
  fail the business mutation that produced the report.
- Do not keep Python Telegram handlers as a dependency for Go mutation
  correctness.
- Python Telegram bot command handlers are disabled during the Go migration.
  The old handlers mutated users, nodes, templates, subscriptions, and runtime
  state through Python paths that are no longer the source of truth.
- Python Telegram bot command handlers and webhook notification queue code have
  been removed from the active runtime.
- `/api/settings/telegram` is Go-native and controls Bot API delivery settings,
  report toggles, destinations, proxy URL, topics, and backup schedule fields.
- Periodic Telegram backup delivery is Go-native and uses the Go backup exporter
  plus Telegram document sender.
- Legacy in-memory webhook notification queue delivery is disabled until a Go
  event/outbox design exists.

## Future Go Scope

- Keep the Go notification/event abstraction for admin, user, and node events
  small and mutation-boundary driven.
- Add a Go Telegram bot command layer only after the corresponding Go APIs are
  stable. Bot commands should call Go Admin/User/Node/Service APIs instead of
  direct database CRUD helpers.
- Decide whether events are delivered synchronously, through an outbox table, or
  through a background worker.
- Keep backup delivery on the Go worker path; future changes should preserve the
  same DB settings and Telegram destination rules.
- Webhook delivery still needs a persistent outbox design.

## Events To Revisit

- Admin created, updated, deleted.
- Admin enabled/disabled.
- Admin user disable/activate actions.
- Admin usage and deleted-users usage reset.
- User created, updated, deleted, reset, revoked, next-plan changes.
- Node created (`node_created`).
- Node deleted (`node_deleted`).
- Node usage reset (`node_usage_reset`).
- Node status changes (`node_status_change`), including connected, connecting,
  error, disabled, and limited transitions.
- Service mutation notifications, if required by the product behavior.

## Node Report Notes

- Node mutation paths are Go-native and emit create/delete/usage-reset reports
  from Go.
- Node status-change reports are emitted from Go mutation/status-change
  boundaries that currently update node status.
- Notification delivery must not affect node transaction success. Prefer an
  outbox/background worker if future high-volume Telegram delivery needs
  stronger retry semantics.
- Message formatting should include the previous Python report content where it
  still makes product sense: node name, address, API port, data limit, usage
  coefficient, previous/current status, and actor username.

## Bot Command Notes

The removed Python bot handlers previously covered:

- Admin panel commands such as `/start`, system info, runtime restart, user
  search/list, user create/edit/delete, subscription links, and QR generation.
- User `/usage` lookup.
- Inline keyboards for user lifecycle changes and protocol/inbound selection.

When rebuilt in Go, command handlers should:

- Use the same Go auth/permission/limit core as HTTP APIs.
- Call Go-native User, Node, Service, and Subscription endpoints or internal
  services instead of reading/writing database state ad hoc.
- Avoid exposing runtime restart commands that conflict with the node-only
  master architecture.
- Reintroduce only product-approved commands; old Python commands should not be
  copied blindly.

## Legacy Bot Logic To Port

The Python bot used `pyTelegramBotAPI` with admin chat-id filtering from
`TelegramSettingsService.admin_chat_ids`. The Go implementation should preserve
the product behavior below only where it still fits the Go-native architecture.

### Admin Commands

- `/start` and `/help` showed the admin main menu.
- `/user <username>` searched a user and opened the user detail menu.
- System info showed CPU, RAM, disk, uptime/process information.
- Runtime restart triggered the old local/node runtime restart path. This must
  be redesigned around Go node runtime APIs; the master must not restart a local
  Xray core.

### User Detail And Lifecycle

- User menus displayed status, username, data limit, used traffic, expiry/on-hold
  information, online/subscription timestamps, note, owner admin, and
  subscription URL.
- Lifecycle callbacks:
  - `delete:{username}`
  - `suspend:{username}`
  - `activate:{username}`
  - `reset_usage:{username}`
  - `revoke_sub:{username}`
  - `edit:{username}`
  - `edit_note:{username}`
  - `links:{username}`
  - `genqr:{configs|sub}:{username}`
- All user mutations must call the Go User API/core and must enforce the same Go
  admin permissions and limits as HTTP APIs.
- Subscription links/configs must be generated by Go subscription/config logic.

### Bulk User Actions

- Edit-all menu supported:
  - delete expired users
  - delete limited users
  - add/reduce traffic for all selected users
  - add/reduce expiry time for all selected users
  - add/remove inbound from users
- Go implementation should map these to the Go bulk user action/service-scoped
  action APIs where possible. Every config/status-changing action must enqueue
  the same node operation(s) as the HTTP path.

### User Creation Flow

- The manual create flow collected:
  - username or random username
  - optional bulk count
  - data limit
  - status: `active` or `on_hold`
  - expire date, on-hold duration, or on-hold timeout
  - selected protocols and inbound tags
- Protocol/inbound selection was built from Xray config inbounds. The Go version
  must use Go config/inbound repository data and never import Python `runtime.xray`.
- Default VLESS flow came from Xray config. The Go version must derive this from
  Go config builder state.
- Bulk create generated suffixed usernames and created multiple users using the
  same selected plan.

### Removed Template Flow

User templates have been removed from the product and their tables are dropped
by migration. Do not rebuild the legacy template bot flow unless the product
explicitly reintroduces a new template concept.

### User `/usage`

- Public `/usage <username>` looked up a user and returned status, data limit,
  data used, expire date, and days left.
- If restored, this must use Go read-only user APIs and must follow the desired
  privacy policy for public Telegram chats.

### Keyboards And Callback State

- Legacy keyboards included:
  - main menu
  - edit-all menu
  - inbound selection
  - random username
  - user menu
  - status select
  - subscription/config QR actions
  - confirm/cancel actions
  - paginated user list
  - protocol/inbound selection
- Multi-step flows used in-memory bot state and registered next-step handlers.
  The Go version should prefer a small conversation-state table with expiry so
  deploy/restart does not strand users mid-flow.

### Legacy Dependencies That Must Not Return

- Direct Python `crud.*` calls.
- Direct Python `node_operations.*` calls.
- Direct `app.runtime.xray` / local Xray config reads.
- Direct Python subscription helpers.
- Direct runtime restart commands against the master host.

## Go Report Logic

Reports are emitted from Go mutation boundaries and use `telegram_settings`
event toggles:

- User created: username, traffic limit, expire date, proxies/protocols, reset
  strategy, next-plan flag, owner admin, actor.
- User updated: same core fields plus owner/actor.
- User deleted: username, owner admin, actor.
- User status change: username, owner admin, new status.
- User usage reset: username, owner admin, actor.
- User auto reset / next plan: username, traffic limit, expire date.
- User auto-renew set/applied: username, rule count or resulting plan.
- Subscription revoked: username, owner admin, actor.
- Login: username, password, client IP, success/failure. This preserves the old
  Python report shape; revisit the password field before a future public API
  stability promise.
- Admin created/updated/deleted/usage reset/limit reached: username, role/scope,
  users/data limits, changed fields, actor, current/limit values.

Delivery honors Telegram settings, event enable flags, forum topics, logs
chat/admin chat fallback, proxy settings, retry/backoff, and last-error tracking
for the dashboard.

## Backup Delivery Logic

Telegram backup delivery is Go-native. Preserve:

- Backup enabled/scope/interval settings.
- Backup chat or admin-chat fallback.
- Forum topic routing for `backup`.
- File splitting around the Telegram document size limit.
- Caption fields: filename, scope, date, time, and part number.
- Last sent/error status updates in Telegram settings.

## Webhook Notification Queue To Preserve

The removed Python webhook queue used `WEBHOOK_ADDRESS` and `WEBHOOK_SECRET` to
send batches of user/admin events to external HTTP endpoints. It was in-memory,
retried failed batches with `RECURRENT_NOTIFICATIONS_TIMEOUT`, and stopped after
`NUMBER_OF_RECURRENT_NOTIFICATIONS`.

When rebuilt in Go:

- Prefer a persistent outbox table over an in-memory queue so events survive
  restarts.
- Keep payload compatibility for existing webhook consumers where possible:
  `action`, `username`, user/admin snapshots, actor, enqueue/send timestamps,
  and retry count.
- Send `x-webhook-secret` when configured.
- Batch events per tick and retry failed events with bounded attempts.
- Ensure webhook delivery failure never rolls back the business mutation that
  produced the event.

## Rollout Notes

- The Go admin/auth migration should not depend on this file being completed.
- Once Go-native admin/user/service mutation paths are stable, revisit this TODO
  and implement Telegram delivery as a separate migration.
