import type { RuntimePackage, UICustomizationContext, UICustomizationRuntimeResult } from './types'

export function comparePackages(a: RuntimePackage, b: RuntimePackage) {
  return Number(a.scopeType === 'world') - Number(b.scopeType === 'world')
    || a.sortOrder - b.sortOrder || a.createdAt - b.createdAt
    || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
}

export function createUICustomizationRuntime({
  document: doc,
  loadPackages,
  onPackages,
}: {
  document: Document
  loadPackages: (context: UICustomizationContext) => Promise<UICustomizationRuntimeResult>
  // Receives every package set the CSS layer applies (including the empty set
  // for Safe Mode, internal, failures and dispose) so scripts follow CSS.
  // interim marks the provisional unload before a context's request returns.
  onPackages?: (packages: RuntimePackage[], interim: boolean, result?: UICustomizationRuntimeResult) => void
}) {
  const styles = new Map<string, HTMLStyleElement>()
  let active: RuntimePackage[] = []
  let installed: RuntimePackage[] = []
  let generation = 0
  let disposed = false
  let previous: UICustomizationContext | undefined
  let previousAuthority = ''

  const keepOrder = () => {
    const desired = installed.map(p => styles.get(p.id)).filter((s): s is HTMLStyleElement => !!s)
    const tail = Array.from(doc.head.children).slice(-desired.length)
    if (desired.length && desired.some((style, i) => tail[i] !== style)) {
      for (const style of desired) doc.head.appendChild(style)
    }
  }
  const apply = (packages: RuntimePackage[], interim = false, result?: UICustomizationRuntimeResult) => {
    active = [...packages].sort(comparePackages)
    onPackages?.(active, interim, result)
    installed = active.filter(p => p.css !== '')
    // UI Packages are independent of display's traditional custom color theme.
    // Detached overlays need a root-level activation hook for their shared baseline.
    if (installed.length) doc.documentElement.dataset.scUiCustomized = 'true'
    else delete doc.documentElement.dataset.scUiCustomized
    const ids = new Set(installed.map(p => p.id))
    for (const [id, style] of styles) {
      if (!ids.has(id)) { style.remove(); styles.delete(id) }
    }
    for (const p of installed) {
      let style = styles.get(p.id)
      if (!style) {
        style = doc.createElement('style')
        style.dataset.scCustomizationId = p.id
        style.dataset.scCustomizationScope = p.scopeType
        styles.set(p.id, style)
      }
      if (style.textContent !== p.css) style.textContent = p.css
    }
    keepOrder()
  }
  // Lazy route styles and dynamically installed theme styles must remain below
  // the customization layer. Only child insertions are observed, not CSS text.
  const Observer = doc.defaultView?.MutationObserver
  const observer = Observer ? new Observer(keepOrder) : null
  observer?.observe(doc.head, { childList: true })

  return {
    async reconcile(ctx: UICustomizationContext, safe: boolean, authority = '') {
      if (disposed) return
      const epoch = ++generation
      if (safe || ctx.surface === 'internal') {
        apply([]); previous = ctx; previousAuthority = authority; return
      }
      // A different user must not inherit scripts activated for the previous one.
      if (previous && previousAuthority !== authority) apply([])
      else if (previous && previous.surface !== ctx.surface) apply([], true)
      else if (previous && (previous.worldId !== ctx.worldId || previous.slug !== ctx.slug || previous.channelId !== ctx.channelId)) {
        apply(active.filter(p => p.scopeType === 'platform'), true)
      }
      previous = ctx; previousAuthority = authority
      try {
        const result = await loadPackages(ctx)
        if (disposed || epoch !== generation) return
        apply(result.packages, false, result)
      } catch {
        // A failed permission refresh must not leave old world CSS installed.
        if (!disposed && epoch === generation) apply([])
      }
    },
    dispose() {
      disposed = true; generation++; observer?.disconnect(); apply([])
    },
  }
}
