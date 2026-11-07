import type { UICustomizationContext } from './types'

interface RouteContext {
  name?: unknown
  params: Record<string, unknown>
  query: Record<string, unknown>
  meta: Record<string, unknown>
}

export interface CustomizationChatContext {
  currentWorldId: string
  channelId: string
  observerMode: boolean
  observerWorldId: string
  observerChannelId: string
  observerSlug: string
  embedReady?: boolean
  // Captured centrally after consumeWorldInvite completes, scoped to its slug.
  invite?: { slug: string; worldId: string }
}

const text = (value: unknown) => typeof value === 'string' ? value.trim() : ''

export function resolveCustomizationContext(route: RouteContext, chat: CustomizationChatContext): UICustomizationContext {
  const ctx: UICustomizationContext = {
    surface: 'lobby', worldId: '', channelId: '', slug: '', observer: false, publicSurface: false,
  }
  const routeWorld = text(route.params.worldId)
  const routeChannel = text(route.params.channelId)
  const slug = text(route.params.slug)
  if (route.meta.internalSurface === true) return { ...ctx, surface: 'internal' }
  switch (route.name) {
    case 'user-signin': case 'user-signup': case 'user-password-reset': case 'password-recovery': case 'mcp-oauth-consent':
      ctx.surface = 'auth'; break
    case 'world-detail':
      ctx.surface = 'world-detail'; ctx.worldId = routeWorld; break
    case 'world-private-hint':
      // The hint may represent a private world the visitor cannot resolve.
      // Keep the surface styling but never use the route ID as world authorization.
      ctx.surface = 'world-detail'; break
    case 'home':
      // `/` renders HomeView (the chat shell), not the world lobby. Before
      // HomeView canonicalizes the URL, the remembered chat selection is the
      // authoritative context for world-scoped customization.
      ctx.surface = 'chat'; ctx.worldId = chat.currentWorldId; ctx.channelId = chat.channelId; break
    case 'world-channel':
      ctx.surface = chat.observerMode ? 'observer' : 'chat'
      ctx.worldId = routeWorld || (chat.observerMode ? chat.observerWorldId : chat.currentWorldId)
      // Never pair B's route with A's still-active channel during navigation.
      ctx.channelId = routeChannel || (ctx.worldId === chat.currentWorldId ? chat.channelId : '')
      ctx.observer = chat.observerMode
      ctx.publicSurface = chat.observerMode
      // ObserverEntry resolves the public slug and then redirects into the
      // normal world-channel route. Keep carrying that verified slug so a
      // private world's OB session is authorized by the observer link rather
      // than falling back to ordinary world visibility.
      if (chat.observerMode) ctx.slug = chat.observerSlug
      break
    case 'split':
      ctx.surface = 'split'
      // Split's host belongs to its fixed scope, not whichever iframe is active.
      // IC/OOC presets use a synthetic storage scope, not a world ID.
      ctx.worldId = text(route.query.scopeWorldId).includes('::preset:ic-ooc')
        ? text(route.query.worldId)
        : text(route.query.scopeWorldId) || text(route.query.worldId)
      break
    case 'embed':
      ctx.surface = 'embed'; ctx.publicSurface = true
      // Embed can change worlds via its verified parent bridge. Once the chat
      // session is ready it is authoritative over initial URL preferences.
      ctx.worldId = chat.embedReady ? chat.currentWorldId : text(route.query.worldId)
      ctx.channelId = chat.embedReady ? chat.channelId || text(route.query.channelId) : text(route.query.channelId)
      ctx.observer = chat.observerMode
      break
    case 'theater':
      ctx.surface = 'theater'
      ctx.worldId = text(route.query.worldId)
      ctx.channelId = text(route.query.channelId)
      break
    case 'observer-entry':
      ctx.surface = 'observer'; ctx.observer = true; ctx.publicSurface = true
      if (chat.observerMode && chat.observerSlug === slug) {
        ctx.worldId = chat.observerWorldId; ctx.channelId = chat.observerChannelId; ctx.slug = slug
      }
      break
    case 'invite-consume':
      ctx.surface = 'invite'
      if (chat.invite?.slug === slug) ctx.worldId = chat.invite.worldId
      break
    case 'log-layout':
      ctx.surface = 'log-layout'; ctx.slug = slug
      // The server resolves the project and its permissions, never chat state.
      break
    case 'log-public':
      ctx.surface = 'log-public'; ctx.slug = slug; ctx.publicSurface = true
      break
    case 'status': case 'status-perf':
      ctx.surface = 'status'; break
    // world-lobby and about have no world, even if chat remembers one.
  }
  return ctx
}

export function hasSafeUIParameter(url: Pick<Location, 'search' | 'hash'>): boolean {
  return new URLSearchParams(url.search).get('safe-ui') === '1'
    || new URLSearchParams(url.hash.split('?')[1] || '').get('safe-ui') === '1'
}
