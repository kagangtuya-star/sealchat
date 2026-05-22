import { buildGlobalFontFamilyStack, sanitizeFontFamilyName } from './fontUtils'
import { registerFontFromBlob } from './fontLoader'
import {
  buildPlatformFontFileUrl,
  buildPlatformFontManifestUrl,
  buildPlatformFontSubsetUrl,
  getPlatformFontManifest,
  getPlatformFontMeta,
} from './platformFontApi'

const inflightLoads = new Map<string, Promise<string>>()
const loadedFamilies = new Map<string, string>()
const failedLoads = new Set<string>()
const loadedSubsetStyles = new Map<string, string>()

const fetchFontBlob = async (url: string): Promise<Blob> => {
  const resp = await fetch(url, { credentials: 'include' })
  if (!resp.ok) {
    throw new Error(`字体请求失败（HTTP ${resp.status}）`)
  }
  return await resp.blob()
}

const loadSinglePlatformFont = async (fontId: string, family: string): Promise<string> => {
  const blob = await fetchFontBlob(buildPlatformFontFileUrl(fontId))
  await registerFontFromBlob(family, blob)
  return family
}

const attachSubsetStyle = (fontId: string, cssText: string, baseUrl: string): void => {
  if (typeof document === 'undefined') return
  if (loadedSubsetStyles.has(fontId)) return
  const resolvedCss = cssText.replace(
    /url\((['"]?)([^)'"]+)\1\)/gu,
    (_all, quote: string, rawUrl: string) => {
      const trimmed = String(rawUrl || '').trim()
      if (!trimmed || /^(data:|blob:|https?:|\/\/)/iu.test(trimmed)) {
        return `url(${quote || ''}${trimmed}${quote || ''})`
      }
      const normalized = trimmed.replace(/^\.?\//u, '')
      const absolute = new URL(normalized, `${baseUrl.replace(/\/+$/u, '')}/`).toString()
      return `url(${quote || ''}${absolute}${quote || ''})`
    },
  )
  const style = document.createElement('style')
  style.dataset.platformFontSubset = fontId
  style.textContent = resolvedCss
  document.head.appendChild(style)
  loadedSubsetStyles.set(fontId, resolvedCss)
}

const tryLoadSubsetPlatformFont = async (fontId: string, family: string): Promise<boolean> => {
  try {
    const manifest = await getPlatformFontManifest(fontId)
    const cssName = String(manifest?.cssName || manifest?.entry || '').trim()
    if (!cssName) {
      return false
    }
    const cssUrl = manifest?.cssUrl || buildPlatformFontSubsetUrl(fontId, cssName)
    const resp = await fetch(cssUrl, { credentials: 'include' })
    if (!resp.ok) {
      return false
    }
    const cssText = await resp.text()
    attachSubsetStyle(fontId, cssText, buildPlatformFontManifestUrl(fontId).replace(/\/subset-manifest$/u, '/subset/'))
    if (typeof document !== 'undefined' && document.fonts) {
      await document.fonts.load(`1em "${family}"`)
    }
    return true
  } catch {
    return false
  }
}

export const ensurePlatformFontLoaded = async (
  fontId: string,
  preferredFamily?: string,
): Promise<string> => {
  const normalizedId = String(fontId || '').trim()
  if (!normalizedId) {
    throw new Error('缺少平台字体 ID')
  }
  const cached = loadedFamilies.get(normalizedId)
  if (cached) {
    return cached
  }
  const inflight = inflightLoads.get(normalizedId)
  if (inflight) {
    return inflight
  }
  const task = (async () => {
    const meta = await getPlatformFontMeta(normalizedId)
    const family = sanitizeFontFamilyName(preferredFamily || meta.family || meta.displayName)
    if (!family) {
      throw new Error('平台字体缺少可用字体名')
    }
    if (meta.deliveryMode === 'subset') {
      const subsetLoaded = await tryLoadSubsetPlatformFont(normalizedId, family)
      if (!subsetLoaded) {
        await loadSinglePlatformFont(normalizedId, family)
      }
    } else {
      await loadSinglePlatformFont(normalizedId, family)
    }
    loadedFamilies.set(normalizedId, family)
    failedLoads.delete(normalizedId)
    return family
  })()
  inflightLoads.set(normalizedId, task)
  try {
    return await task
  } catch (error) {
    failedLoads.add(normalizedId)
    throw error
  } finally {
    inflightLoads.delete(normalizedId)
  }
}

export const resolvePlatformFontFamily = async (fontId: string, preferredFamily?: string): Promise<string> => {
  const family = await ensurePlatformFontLoaded(fontId, preferredFamily)
  return buildGlobalFontFamilyStack(family)
}

export const preloadPlatformFontsFromDom = async (root?: ParentNode | null): Promise<void> => {
  if (typeof document === 'undefined') return
  const host = root || document
  const nodes = host.querySelectorAll<HTMLElement>('[data-platform-font-id]')
  const tasks: Promise<unknown>[] = []
  nodes.forEach((node) => {
    const fontId = node.dataset.platformFontId?.trim()
    if (!fontId || failedLoads.has(fontId)) return
    const family = sanitizeFontFamilyName(node.dataset.platformFontFamily || '')
    tasks.push(
      ensurePlatformFontLoaded(fontId, family).then((loadedFamily) => {
        node.style.fontFamily = buildGlobalFontFamilyStack(loadedFamily)
      }).catch((error) => {
        console.warn('平台字体预加载失败', fontId, error)
      }),
    )
  })
  if (tasks.length > 0) {
    await Promise.allSettled(tasks)
  }
}

export const clearPlatformFontRegistry = () => {
  inflightLoads.clear()
  loadedFamilies.clear()
  failedLoads.clear()
  if (typeof document !== 'undefined') {
    document.querySelectorAll<HTMLStyleElement>('style[data-platform-font-subset]').forEach((node) => node.remove())
  }
  loadedSubsetStyles.clear()
}
