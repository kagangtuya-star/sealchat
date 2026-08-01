import { watchEffect } from 'vue'
import { useRoute } from 'vue-router'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { useChatStore } from '@/stores/chat'
import { useUtilsStore } from '@/stores/utils'
import { CURSOR_SLOTS } from './cursorTypes'
import type { CursorAssetConfig, CursorSlot, CursorThemeConfig } from './cursorTypes'

export const normalizeCursorTheme = (
  value: CursorThemeConfig | null | undefined,
  scope: 'platform' | 'world',
): CursorThemeConfig => {
  const slots: CursorThemeConfig['slots'] = {}
  for (const slot of CURSOR_SLOTS) {
    const source = value?.slots?.[slot]
    let mode = source?.mode || (scope === 'world' ? 'inherit' : 'browser')
    if (scope === 'platform' && mode === 'inherit') mode = 'browser'
    slots[slot] = {
      mode,
      attachmentId: source?.attachmentId?.replace(/^id:/, '') || '',
      hotspotX: Math.max(0, Math.min(127, Number(source?.hotspotX) || 0)),
      hotspotY: Math.max(0, Math.min(127, Number(source?.hotspotY) || 0)),
      width: Number(source?.width) || 0,
      height: Number(source?.height) || 0,
      size: Math.max(16, Math.min(128, Number(source?.size) || 32)),
      animated: !!source?.animated,
    }
  }
  return { version: 1, slots }
}

const resolveSlot = (
  slot: CursorSlot,
  platform: CursorThemeConfig,
  world?: CursorThemeConfig | null,
): CursorAssetConfig | null => {
  const worldAsset = world?.slots?.[slot]
  if (worldAsset?.mode === 'browser') return null
  if (worldAsset?.mode === 'custom') return worldAsset
  const platformAsset = platform.slots?.[slot]
  return platformAsset?.mode === 'custom' ? platformAsset : null
}

export const applyCursorTheme = (
  platformValue?: CursorThemeConfig | null,
  worldValue?: CursorThemeConfig | null,
) => {
  if (typeof document === 'undefined') return
  const platform = normalizeCursorTheme(platformValue, 'platform')
  const world = worldValue ? normalizeCursorTheme(worldValue, 'world') : null
  const root = document.documentElement
  for (const slot of CURSOR_SLOTS) {
    const asset = resolveSlot(slot, platform, world)
    const property = `--sc-cursor-${slot}`
    if (!asset?.attachmentId) {
      root.style.setProperty(property, slot)
      continue
    }
    const url = resolveAttachmentUrl(`id:${asset.attachmentId}`)
    const hotspotX = Math.max(0, Math.min((asset.width || 128) - 1, asset.hotspotX || 0))
    const hotspotY = Math.max(0, Math.min((asset.height || 128) - 1, asset.hotspotY || 0))
    root.style.setProperty(property, `url("${url}") ${hotspotX} ${hotspotY}, ${slot}`)
  }
}

export const useCursorThemeRuntime = () => {
  const route = useRoute()
  const chat = useChatStore()
  const utils = useUtilsStore()
  watchEffect(() => {
    const routeWorldId = typeof route.params.worldId === 'string' ? route.params.worldId.trim() : ''
    const world = routeWorldId
      ? chat.worldDetailMap[routeWorldId]?.world || chat.worldMap[routeWorldId]
      : null
    applyCursorTheme(utils.config?.cursorTheme, world?.cursorTheme)
  })
}
