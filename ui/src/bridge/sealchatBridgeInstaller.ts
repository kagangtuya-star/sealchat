import { watch } from 'vue'
import type { Pinia } from 'pinia'
import type { Router } from 'vue-router'

import { resolveAttachmentUrl } from '../composables/useAttachmentResolver'
import { chatEvent, useChatStore } from '../stores/chat'
import { buildBridgeMessagePayload, buildRoleSnapshot } from './sealchatBridgeSerializer'
import type { SealChatBridgeMessageEvent } from './sealchatBridgeProtocol'
import { createSealChatBridgeRuntime } from './sealchatBridgeRuntime'
import {
  markBridgeError,
  setSealChatBridgeConnectState,
  syncSealChatBridgeContext,
} from './sealchatBridgeStatus'

export const installSealChatBridgeRuntime = ({
  pinia,
  router,
}: {
  pinia: Pinia
  router?: Router
}) => {
  if (typeof window === 'undefined') {
    return null
  }

  const chat = useChatStore(pinia)
  const resolveRouteWorldId = () => {
    const raw = router?.currentRoute.value?.params?.worldId
    return typeof raw === 'string' ? raw.trim() : ''
  }
  const resolveRouteChannelId = () => {
    const raw = router?.currentRoute.value?.params?.channelId
    return typeof raw === 'string' ? raw.trim() : ''
  }
  const resolveCurrentWorldId = () => String(chat.currentWorldId || resolveRouteWorldId() || '').trim()
  const resolveCurrentChannelId = () => String(chat.curChannel?.id || resolveRouteChannelId() || '').trim()
  const syncBridgeMeta = () => {
    syncSealChatBridgeContext({
      worldId: resolveCurrentWorldId(),
      channelId: resolveCurrentChannelId(),
    })
    setSealChatBridgeConnectState(chat.connectState || '')
  }

  syncBridgeMeta()

  const runtime = createSealChatBridgeRuntime({
    postMessage: (payload, origin) => {
      if (window.parent === window) {
        return
      }
      window.parent.postMessage(payload, origin)
    },
    getCurrentContext: () => ({
      worldId: resolveCurrentWorldId(),
      channelId: resolveCurrentChannelId(),
    }),
    loadRoles: async () => {
      const channelId = resolveCurrentChannelId()
      if (!channelId) {
        return []
      }

      await chat.loadChannelIdentities(channelId, false)
      await chat.loadChannelIdentityVariants(channelId, false)

      const activeIdentityId = chat.getActiveIdentityId(channelId)
      const revision = Date.now()

      return chat.getScopedChannelIdentities(channelId).map((identity) =>
        buildRoleSnapshot({
          identity,
          variant: chat.getActiveIdentityVariant(channelId, identity.id),
          variants: chat.getIdentityVariants(channelId, identity.id),
          isActive: identity.id === activeIdentityId,
          revision,
          updatedAt: revision,
          resolveAttachmentUrl,
        }),
      )
    },
    isParentSource: (source) => source === window.parent,
  })

  const handleWindowMessage = (event: MessageEvent) => {
    void runtime.handleWindowMessage(event)
  }

  const scheduleRolesSnapshot = (() => {
    let timer: ReturnType<typeof setTimeout> | null = null
    return () => {
      if (!runtime.isActive()) {
        return
      }
      if (timer) {
        clearTimeout(timer)
      }
      timer = setTimeout(() => {
        timer = null
        void runtime.publishRolesSnapshot().catch((error) => {
          markBridgeError(error)
        })
      }, 60)
    }
  })()

  const bridgePatchedFlag = '__sealchatBridgePatched__'
  const originalEmit = (chatEvent.emit as any).bind(chatEvent)
  if (!(chatEvent as any)[bridgePatchedFlag]) {
    ;(chatEvent as any)[bridgePatchedFlag] = true
    ;(chatEvent as any).emit = ((eventName: string, ...args: any[]) => {
      if (eventName === 'message-created' || eventName === 'message-updated' || eventName === 'message-deleted') {
        publishGatewayMessage(eventName, args[0])
      } else if (eventName === 'channel-identities-updated' || eventName === 'channel-identity-updated') {
        scheduleRolesSnapshot()
      }
      return originalEmit(eventName, ...args)
    }) as typeof chatEvent.emit
  }

  const publishGatewayMessage = (eventName: SealChatBridgeMessageEvent, gatewayEvent?: any) => {
    if (!runtime.isActive()) {
      return
    }
    const activeChannelId = resolveCurrentChannelId()
    const channelId = String(gatewayEvent?.channel?.id || activeChannelId || '').trim()
    if (!channelId) {
      return
    }
    if (activeChannelId && channelId !== activeChannelId) {
      return
    }
    const message = gatewayEvent?.message || {}
    const identityId = String(
      message?.identity?.id
      || message?.senderRoleId
      || message?.sender_role_id
      || '',
    ).trim()
    const liveIdentity = identityId
      ? (chat.getScopedChannelIdentities(channelId).find((identity) => identity.id === identityId) || null)
      : null
    const liveVariant = identityId ? chat.getActiveIdentityVariant(channelId, identityId) : null
    try {
      runtime.publishMessage(buildBridgeMessagePayload({
        event: eventName,
        worldId: resolveCurrentWorldId(),
        channelId,
        message,
        liveIdentity,
        liveVariant,
        resolveAttachmentUrl,
      }))
    } catch (error) {
      markBridgeError(error)
    }
  }

  window.addEventListener('message', handleWindowMessage)
  chatEvent.on('channel-identities-updated' as any, () => {
    scheduleRolesSnapshot()
  })
  chatEvent.on('channel-identity-updated' as any, () => {
    scheduleRolesSnapshot()
  })

  watch(
    () => {
      const worldId = resolveCurrentWorldId()
      const channelId = resolveCurrentChannelId()
      const scopeKey = chat.resolveChannelIdentityScopeKey(channelId)
      const identities = channelId
        ? chat.getScopedChannelIdentities(channelId).map((identity) => ({
          id: identity.id,
          displayName: identity.displayName,
          color: identity.color,
          avatarAttachmentId: identity.avatarAttachmentId,
          isTemporary: identity.isTemporary,
          icOocOnActivate: identity.icOocOnActivate || '',
        }))
        : []
      const variants = scopeKey ? (chat.activeChannelIdentityVariant[scopeKey] || {}) : {}
      const activeIdentity = scopeKey ? (chat.activeChannelIdentity[scopeKey] || '') : ''
      return JSON.stringify({
        worldId,
        channelId,
        connectState: chat.connectState || '',
        activeIdentity,
        variants,
        identities,
      })
    },
    () => {
      syncBridgeMeta()
      scheduleRolesSnapshot()
    },
  )

  router?.afterEach(() => {
    syncBridgeMeta()
    scheduleRolesSnapshot()
  })

  return runtime
}
