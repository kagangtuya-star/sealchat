import assert from 'node:assert/strict'

import {
  buildRoleSnapshot,
  buildBridgeMessagePayload,
  normalizeBridgePlainText,
} from '../src/bridge/sealchatBridgeSerializer'
import { createSealChatBridgeRuntime } from '../src/bridge/sealchatBridgeRuntime'

const run = async () => {
  const originalLocation = globalThis.location
  Object.defineProperty(globalThis, 'location', {
    value: { protocol: 'http:' },
    configurable: true,
  })

  const role = buildRoleSnapshot({
    identity: {
      id: 'role-a',
      displayName: '阿尔文',
      color: '#88c0d0',
      avatarAttachmentId: 'avatar-base',
      isTemporary: false,
      icOocOnActivate: 'ic',
    },
    variant: {
      id: 'variant-hurt',
      displayName: '阿尔文·负伤',
      color: '#bf616a',
      avatarAttachmentId: 'avatar-hurt',
    },
    resolveAttachmentUrl: (id: string) => `//assets.test/${id}`,
  })

  assert.equal(role.identityId, 'role-a')
  assert.equal(role.displayName, '阿尔文·负伤')
  assert.equal(role.avatarUrl, 'http://assets.test/avatar-hurt')

  const text = normalizeBridgePlainText('[[图片:id:abc]]你好')
  assert.equal(text, '[图片]你好')

  const richText = normalizeBridgePlainText(JSON.stringify({
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        content: [
          { type: 'text', text: '你好' },
          { type: 'mention', attrs: { id: 'u1', name: '测试' } },
        ],
      },
    ],
  }))
  assert.equal(richText, '你好@测试')

  const payload = buildBridgeMessagePayload({
    event: 'message-created',
    worldId: 'world-1',
    channelId: 'channel-1',
    message: {
      id: 'msg-1',
      content: '[[图片:id:abc]]你好',
      createdAt: 123,
      icMode: 'ooc',
      isWhisper: false,
      identity: { id: 'role-a', displayName: '阿尔文', color: '#88c0d0', avatarAttachment: 'avatar-a' },
    },
    resolveAttachmentUrl: (id: string) => `//assets.test/${id}`,
  })

  assert.equal(payload.identityId, 'role-a')
  assert.equal(payload.contentText, '[图片]你好')
  assert.equal(payload.icMode, 'ooc')

  const livePayload = buildBridgeMessagePayload({
    event: 'message-created',
    worldId: 'world-1',
    channelId: 'channel-1',
    message: {
      id: 'msg-2',
      content: '你好',
      createdAt: 124,
      icMode: 'ic',
      isWhisper: false,
      senderRoleId: 'role-a',
    },
    liveIdentity: {
      id: 'role-a',
      displayName: '阿尔文',
      color: '#88c0d0',
      avatarAttachmentId: 'avatar-base',
    },
    liveVariant: {
      id: 'variant-hurt',
      displayName: '阿尔文·负伤',
      color: '#bf616a',
      avatarAttachmentId: 'avatar-hurt',
    },
    resolveAttachmentUrl: (id: string) => `//assets.test/${id}`,
  })

  assert.equal(livePayload.displayName, '阿尔文·负伤')
  assert.equal(livePayload.color, '#bf616a')
  assert.equal(livePayload.avatarUrl, 'http://assets.test/avatar-hurt')

  const sent: Array<{ payload: unknown; origin: string }> = []
  const runtime = createSealChatBridgeRuntime({
    postMessage: (bridgePayload, origin) => {
      sent.push({ payload: bridgePayload, origin })
    },
    getCurrentContext: () => ({ worldId: 'world-1', channelId: 'channel-1' }),
    loadRoles: async () => [role],
  })

  await runtime.handleWindowMessage({
    source: 'parent-window',
    origin: 'https://owlbear.test',
    data: {
      type: 'sealchat.bridge.handshake',
      version: 1,
      nonce: 'n1',
      want: ['roles', 'messages'],
      currentChannelOnly: true,
    },
  })

  assert.equal(runtime.isActive(), true)
  assert.equal(runtime.getTargetOrigin(), 'https://owlbear.test')
  assert.equal((sent[0]?.payload as { type?: string })?.type, 'sealchat.bridge.handshake.ack')
  assert.equal((sent[1]?.payload as { type?: string })?.type, 'sealchat.bridge.roles.snapshot')

  const nullOriginSent: Array<{ payload: unknown; origin: string }> = []
  const nullOriginRuntime = createSealChatBridgeRuntime({
    postMessage: (bridgePayload, origin) => {
      nullOriginSent.push({ payload: bridgePayload, origin })
    },
    getCurrentContext: () => ({ worldId: 'world-1', channelId: 'channel-1' }),
    loadRoles: async () => [role],
  })

  await nullOriginRuntime.handleWindowMessage({
    source: 'parent-window',
    origin: 'null',
    data: {
      type: 'sealchat.bridge.handshake',
      version: 1,
      nonce: 'n2',
      want: ['roles', 'messages'],
      currentChannelOnly: true,
    },
  })

  assert.equal(nullOriginRuntime.getTargetOrigin(), '*')
  assert.equal(nullOriginSent[0]?.origin, '*')
  assert.equal(nullOriginSent[1]?.origin, '*')

  Object.defineProperty(globalThis, 'location', {
    value: originalLocation,
    configurable: true,
  })

  console.log('sealchat bridge runtime tests passed')
}

void run()
