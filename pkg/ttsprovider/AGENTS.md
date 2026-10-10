# TTS provider contract

This directory is the HTTP protocol leaf package. It must not import `service`
or `model`. Qwen-Audio-TTS uses `SpeechSynthesizer` and the `voice-enrollment`
customization branch; do not substitute Qwen3-TTS or chat-completion payloads.

- A charged POST is sent once. Timeout, EOF, malformed data and ambiguous 5xx
  do not establish zero usage and must not trigger an automatic retry.
- `usage.characters` is cumulative. Preserve the maximum valid count and only
  mark it confirmed on the successful terminal event. `usage.count` on query,
  list or delete is not evidence of a charged creation operation.
- SSE framing and media framing are independent. Append decoded `audio.data`
  bytes in order. `finish_reason=stop` establishes protocol completion, not
  audio integrity. The caller must validate the complete media before publishing
  a resource. `format=opus` alone does not establish an Ogg container.
- Return design `preview_audio` directly; never synthesize an additional preview.
- Keep cloud voice versions immutable; no update-voice operation is exposed.

Caller integration must keep speech money separate from text money and audio
storage bytes. Preview/creating expiry may claim temporary voices; saved voices
are never expiry-GC targets. Saving and expiry must share the database user lock.
Neither cloud voice visibility nor channel membership grants access to a private
message or clone source sample; resource and stream authorization must enforce
the actual message visibility, including whispers, before delivering bytes.

Validate protocol changes with `go test ./pkg/ttsprovider`. Service invariants are
covered separately by `go test ./service -run TestTTS`.
