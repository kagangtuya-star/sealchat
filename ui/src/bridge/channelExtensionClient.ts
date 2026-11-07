import { ChannelEmbedClient, type ChannelClientTransport } from './channelEmbedSdk'
import { createChannelExtensionContext, channelExtensionPublicError, type ChannelExtensionDeps } from './channelExtensionContext'
import { CHANNEL_EMBED_EVENT, CHANNEL_EMBED_RESPONSE, isEmbedRequest, randomEmbedId } from './channelEmbedProtocol'

/** In-process transport to the same dispatcher and SDK facade used by real iForms. */
export const createChannelExtensionClient = (deps: ChannelExtensionDeps) => {
  const context = createChannelExtensionContext(deps)
  const id = randomEmbedId('local')
  let closed = false
  const deliver = (data: unknown) => { if (!closed) transport.onmessage?.(new MessageEvent('message', { data: structuredClone(data) })) }
  const transport: ChannelClientTransport = {
    onmessage: null,
    postMessage(value) {
      value = structuredClone(value)
      if (closed || !isEmbedRequest(value)) return
      const request = value
      void session.request(request.method, request.params, request.contextVersion).then(
        result => deliver({ type: CHANNEL_EMBED_RESPONSE, version: 1, sessionId: id, requestId: request.requestId, ok: true, result }),
        error => deliver({ type: CHANNEL_EMBED_RESPONSE, version: 1, sessionId: id, requestId: request.requestId, ok: false, error: channelExtensionPublicError(error) }),
      )
    },
    close() { if (closed) return; closed = true; session.dispose(); context.dispose() },
  }
  const session = context.openSession(event => deliver({ ...event, type: CHANNEL_EMBED_EVENT, version: 1, sessionId: id, eventId: randomEmbedId('event') }))
  const client = new ChannelEmbedClient(transport, id, context.getContext().contextVersion, context.getContext().capabilities)
  void context.ready.then(() => {
    if (!context.isActive()) return
    deliver({ type: CHANNEL_EMBED_EVENT, version: 1, sessionId: id, topic: 'capabilities.changed', contextVersion: context.getContext().contextVersion, payload: context.getContext().capabilities })
    void session.initialize()
  })
  return client
}
