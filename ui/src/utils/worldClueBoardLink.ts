import {
  buildInternalSurfaceResourceKey,
  generateInternalSurfaceLink,
  openInternalSurfaceLink,
} from './internalSurfaceLink'

export interface WorldClueBoardLinkContext {
  worldId: string
  channelId: string
}

export function generateWorldClueBoardLink(context: WorldClueBoardLinkContext) {
  const params = { type: 'clue-board' as const, id: 'main', worldId: context.worldId, channelId: context.channelId }
  return {
    key: buildInternalSurfaceResourceKey(params),
    // Board embeds stay on the current origin and deployment base path. The
    // externally configured domain is intentionally not used for this
    // lifecycle-sensitive same-origin iframe.
    url: generateInternalSurfaceLink(params),
    title: '线索板',
    presentation: { chrome: 'minimal' as const, width: 1100, height: 760 },
  }
}

export function openWorldClueBoardLink(context: WorldClueBoardLinkContext) {
  const resource = generateWorldClueBoardLink(context)
  return { resource, window: openInternalSurfaceLink(resource.url, resource.presentation) }
}
