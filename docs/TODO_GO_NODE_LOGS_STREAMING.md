# TODO: Go Node Logs Streaming

Node log reads currently have a Go-native buffered path for HTTP responses, but
the WebSocket/streaming path is intentionally not implemented yet. This TODO
tracks the future streaming design so Python does not regain node log ownership.

## Goal

Expose live or tailing node logs through the Go gateway, Go Master API, and
rebecca-node gRPC without routing WebSocket traffic through Python.

## Proposed Routes

- `GET /api/node/{node_id}/logs`
  - Keep the existing buffered/tail response for compatibility.
- `WS /api/node/{node_id}/logs`
  - Future browser-facing WebSocket route through the Go gateway.
- Optional internal route:
  - `GET /internal/node/{node_id}/logs/stream`
  - Server-sent events or chunked stream if this fits the gateway better than
    direct WebSocket bridging.

## Stream Chain

1. Browser connects to the Go gateway WebSocket route.
2. Gateway validates/forwards auth and opens a stream to Go Master API.
3. Go Master API validates node access and opens `NodeLogsService.StreamLogs`
   against rebecca-node over mTLS gRPC.
4. rebecca-node streams log lines from the local service/journal/file source.
5. Gateway forwards lines to the browser until the client disconnects, the node
   stream ends, or limits are reached.

## Auth Strategy

- Accept the same admin JWT/API-key auth as other node routes.
- Query-token auth may be kept only for backwards-compatible WebSocket clients.
- Require sudo/full_access for live streaming unless product requirements later
  allow read-only node permissions.
- Do not expose node certificate material or private connection errors to
  standard admins.

## Backpressure And Limits

- Enforce `max_lines` and a server-side default, such as `200`.
- Add a hard stream duration limit to avoid abandoned browser sessions.
- Apply write deadlines to WebSocket sends.
- Drop or close slow consumers rather than buffering unbounded log lines.
- Rate-limit reconnect loops per admin/node pair.

## Node Down Behavior

- If the node is unreachable before the stream starts, return a clear
  `503 node unavailable` style error.
- If the node disconnects mid-stream, close the WebSocket with a clear reason
  and update the node status/message through the normal Go node controller path.
- Do not fall back to Python or local master logs.

## Runtime Notes

- Python `app/routers/node.py` may keep only a disabled WebSocket placeholder
  until this work is implemented.
- The existing Go node controller can already consume `NodeLogsService.StreamLogs`
  for buffered reads; the future work is exposing that stream end-to-end.
- Tests should cover auth denial, node down, slow client handling, max line
  limits, and clean disconnects.
