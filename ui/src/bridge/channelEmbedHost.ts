import type { ChannelIForm } from '@/types/iform'
import { createChannelExtensionContext, channelExtensionPublicError, type ChannelExtensionDeps } from './channelExtensionContext'
import { CHANNEL_EMBED_EVENT, CHANNEL_EMBED_HANDSHAKE_ACK, CHANNEL_EMBED_RESPONSE, isEmbedHandshakeRequest, isEmbedRequest, randomEmbedId } from './channelEmbedProtocol'
export type { ChannelEmbedTheaterCharacterSource, ChannelEmbedTheaterEventSink, ChannelEmbedTheaterContext, ChannelEmbedPublishedEvent } from './channelExtensionContext'

type HostDeps = Omit<ChannelExtensionDeps, 'scope' | 'capabilities' | 'form'> & { form: ChannelIForm; iframe: HTMLIFrameElement }

/** Iframe security and transport only. All channel behavior belongs to Context. */
export const createChannelEmbedHost = (deps: HostDeps) => {
  const context = createChannelExtensionContext({ ...deps, scope: { type: 'iform', id: deps.form.id }, capabilities: deps.form.bridgePolicy?.capabilities || [], onDispose: () => { closeTransport(); window.removeEventListener('message', handleHandshake) } })
  let sessionId = randomEmbedId('session')
  let session: ReturnType<typeof context.openSession> | null = null
  let port: MessagePort | null = null
  const nonceSeen = new Set<string>()
  const expectedOrigin = (() => { try { const src = deps.iframe.src || deps.form.url; return src ? new URL(src, window.location.href).origin : '' } catch { return '' } })()
  const allowedOrigin = (origin: string) => {
    const allowed = (deps.form.bridgePolicy?.allowedOrigins || []).map(origin => String(origin).trim()).filter(Boolean)
    return origin === 'null' || (allowed.length ? allowed.includes(origin) : !!expectedOrigin && expectedOrigin !== 'null' && expectedOrigin === origin)
  }
  const closeTransport = () => { session?.dispose(); session = null; port?.close(); port = null }
  const handleHandshake = async (event: MessageEvent) => {
    if (!context.isActive() || !deps.user?.info?.id || event.source !== deps.iframe.contentWindow || !isEmbedHandshakeRequest(event.data) || nonceSeen.has(event.data.nonce)) return
    const targetOrigin = event.origin === 'null' ? '*' : event.origin
    if (!deps.form.bridgePolicy?.enabled || !allowedOrigin(event.origin)) {
      event.source?.postMessage({ type: CHANNEL_EMBED_HANDSHAKE_ACK, version: 1, nonce: event.data.nonce, ok: false, error: { code: 'ORIGIN_DENIED', message: 'Embed origin denied' } }, targetOrigin)
      return
    }
    nonceSeen.add(event.data.nonce)
    await context.ready
    if (!context.isActive()) return
    if (port) { try { port.postMessage({ type: CHANNEL_EMBED_EVENT, version: 1, sessionId, eventId: randomEmbedId('event'), topic: 'session.closed', contextVersion: context.getContext().contextVersion, payload: null, at: Date.now() }) } catch { /* detached */ } }
    closeTransport()
    sessionId = randomEmbedId('session')
    const id = sessionId
    const channel = new MessageChannel()
    const targetPort = channel.port1
    port = targetPort
    session = context.openSession(event => {
      try { targetPort.postMessage({ ...event, type: CHANNEL_EMBED_EVENT, version: 1, sessionId: id, eventId: randomEmbedId('event') }) } finally { if (event.topic === 'session.closed') { closeTransport(); window.removeEventListener('message', handleHandshake) } }
    })
    const targetSession = session
    targetPort.onmessage = message => {
      const request = message.data
      if (!context.isActive() || session !== targetSession || !isEmbedRequest(request) || request.sessionId !== id) return
      void targetSession.request(request.method, request.params, request.contextVersion).then(
        result => targetPort.postMessage({ type: CHANNEL_EMBED_RESPONSE, version: 1, sessionId: id, requestId: request.requestId, ok: true, result }),
        error => targetPort.postMessage({ type: CHANNEL_EMBED_RESPONSE, version: 1, sessionId: id, requestId: request.requestId, ok: false, error: channelExtensionPublicError(error) }),
      ).catch(closeTransport).finally(() => { if (request.method === 'session.close') closeTransport() })
    }
    targetPort.start?.()
    event.source?.postMessage({ type: CHANNEL_EMBED_HANDSHAKE_ACK, version: 1, nonce: event.data.nonce, ok: true, sessionId: id, contextVersion: context.getContext().contextVersion, capabilities: context.getContext().capabilities }, targetOrigin, [channel.port2])
    void session.initialize()
  }
  window.addEventListener('message', handleHandshake)
  const stop = () => { context.stop(); closeTransport(); window.removeEventListener('message', handleHandshake) }
  return { get sessionId() { return sessionId }, stop, getContext: context.getContext, isActive: context.isActive }
}
