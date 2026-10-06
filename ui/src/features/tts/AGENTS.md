# TTS frontend contract

This directory owns speech requests, state, dialogs and the single main-window
playback coordinator. Chat items and split/theater children must not own players
or submit synthesis from message reception, history loading or component mount.

- Keep all HTTP calls in `api.ts`. Charged POSTs use stable request keys; an
  ambiguous response must not generate a new key and repeat a possible charge.
- Frontend runtime IDs and request keys must not assume `crypto.randomUUID()` is
  available: HTTP/LAN deployments and older WebViews may lack it. Reuse the
  feature's compatibility helper or guard the API and provide a fallback.
- Speech money, text money and audio-library storage bytes are separate. Saved
  private and public personal voices both occupy the author's slots; browsing
  others' voices and replaying existing files do not charge or occupy slots.
- Only `SpeechHost.vue` in the main window owns automatic playback. Child
  contexts forward intent, with origin, frame source and account checks. Do not
  reuse provider credentials or the main login token in a WebSocket URL.
- Temporary synthesis overrides live only in memory, scoped to account/channel
  and synchronized between same-tab views. Account changes clear them. Browser
  automatic playback is independent and requires a user gesture to unlock audio.
- Playback obtains a message/resource ticket; it never synthesizes a missing
  file. Message visibility, whispers, revocation and revision remain server
  authorization checks even when the UI displays a playback button.
- `player.ts` owns both complete authorized file playback and the PCM AudioWorklet.
  `pcm-worklet.js` continuously decodes PCM16 with a bounded ring buffer and
  sample-rate conversion. Stream frames contain big-endian epoch/sequence (8
  bytes), followed by raw interleaved little-endian PCM16; the backend strips the
  validated WAV header. SSE boundaries are not media boundaries. Do not send
  chunks to `decodeAudioData`, or claim Opus is Ogg without verified media.
  MP3 remains explicitly labeled file mode. End flushes input; speaker completion
  is separate. Stop must disconnect the worklet and invalidate pending loading.
- Stop, account/channel changes and cancellation invalidate pending async work.
  Do not allow a late callback to restart an old utterance.
- Automatic playback ownership: the server orders and broadcasts (channel lane,
  realtime start/PCM/end, then a file-mode announcement of the archive); only the
  browser knows what it played. `SpeechQueue` (runtime.ts) is the one client
  FIFO: a busy page queues successors and plays their archived files later,
  never buffering their PCM. Only natural completion or an explicit stop/cancel
  settles a message; `/queue` is queried once per (re)connect, not polled.
- `/states` is a recovery snapshot for channel/account scope entry and after a
  successful `channel.enter`/re-enter (`channel-switch-to`); never poll it
  periodically. Message TTS metadata normally arrives through the main chat
  gateway's `message-tts-updated`; the dedicated TTS playback socket does not
  synchronize metadata.
- Preview promotion reuses a cloud voice. Expiration applies to previews/creating
  voices, never saved voices; cache expiry does not delete durable message audio.

Validation from repository root:

```sh
node --test ui/src/features/tts/runtime.test.mjs
node --test ui/src/features/tts/pcm.test.mjs
node --test ui/src/features/tts/player.test.mjs
node --test ui/src/features/tts/speech-host.test.mjs
npm --prefix ui run type-check
npm --prefix ui run build-only
```

The node tests stub the app, Web Audio and the server; they do not establish
browser playback or provider compatibility.
