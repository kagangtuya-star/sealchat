## Realtime Performance Notes

WebSocket chat paths are performance-sensitive and must preserve backpressure and ordering guarantees.

- Keep exactly one normal WebSocket data writer per connection. Do not add concurrent `WriteMessage` writers.
- Keep latency-sensitive responses, reliable chat events, and coalescible derived notifications logically separated. Never coalesce or silently drop real message events.
- Prefer bounded enqueue + asynchronous socket delivery for normal authenticated API responses. Do not block the read loop on physical socket completion unless the protocol requires it.
- Keep outbound queues bounded. On queue overflow or write timeout, close the slow consumer instead of retrying indefinitely or growing queues.
- Preserve the current timeout distinction unless profiling proves otherwise:
  - synchronous/protocol writes: `10s`
  - authenticated realtime data writes: `3s`
  - ping/control writes: `3s`
- Control frames may still contend with data writes inside the WebSocket library. Check the concrete library implementation before changing locking or timeout behavior.
- Clients must tolerate both `ACK -> Event` and `Event -> ACK`. Use stable `clientId` / message IDs for optimistic-message reconciliation.
- Diagnose latency by stage: business solve time, response queue wait, socket/write time, queue depth, queue-full/errors, DB pool waits, and background work.
- Do not treat high SQLite pool waits as an automatic reason to increase `MaxOpenConns`; reduce write/query amplification first.
- Any feature added to `message.create` must be reviewed for multiplicative DB/network work across users, channels, windows, or subscribers.
- Performance fixes should target the measured bottleneck only. Avoid mixing WebSocket scheduling, database tuning, schema changes, and unrelated cleanup in one patch.