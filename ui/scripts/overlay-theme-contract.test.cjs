// Run with: node --test scripts/overlay-theme-contract.test.cjs
// Compile the real styles: protect the public surface and vendor adapter boundary
// without depending on browser timing, third-party DOM snapshots, or theme artwork.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const { test } = require('node:test')
const sass = require('sass')
const postcss = require('postcss')
const { parse, compileStyle } = require('@vue/compiler-sfc')
const root = path.resolve(__dirname, '..')
const read = file => fs.readFileSync(path.join(root, file), 'utf8')
const compile = (source, scoped = false) => {
  const result = compileStyle({ source, filename: 'surface.scss', id: 'data-v-contract', scoped, preprocessLang: 'scss' })
  assert.deepEqual(result.errors, [])
  return postcss.parse(result.code)
}
const componentStyles = file => {
  const { descriptor, errors } = parse(read(file))
  assert.deepEqual(errors, [])
  return descriptor.styles.map(style => compile(style.content, style.scoped)).filter(Boolean)
}
const declarations = (css, selectorPart) => {
  const values = new Map()
  css.walkRules(rule => {
    const matches = selectorPart.endsWith(' {')
      ? rule.selector === selectorPart.slice(0, -2)
      : rule.selector.includes(selectorPart)
    if (matches) rule.nodes.forEach(node => {
      if (node.type === 'decl') values.set(node.prop, node.value)
    })
  })
  return values
}
const overlay = postcss.parse(sass.compile(path.join(root, 'src/assets/overlays.scss')).css)

test('default day and night palettes supply the shared surface and interaction tokens', () => {
  const defaults = postcss.parse(read('src/assets/main.css'))
  for (const selector of [':root {', ":root[data-display-palette='night'] {"]) {
    const vars = declarations(defaults, selector)
    for (const name of ['bg-elevated', 'bg-hover', 'bg-active', 'text-primary', 'text-secondary', 'border-mute']) {
      assert.ok(vars.get(`--sc-${name}`), `${selector} missing --sc-${name}`)
    }
  }
})

test('Naive baseline is activated by either color themes or UI Packages and excludes theater surfaces', () => {
  let count = 0
  overlay.walkRules(rule => {
    if (!rule.selector.includes('.n-dropdown-menu')) return
    count++
    assert.match(rule.selector, /data-custom-theme=['"]?true/)
    assert.match(rule.selector, /data-sc-ui-customized=['"]?true/)
    assert.match(rule.selector, /:not\(:where\([^)]*\.theater-stage-app/)
    assert.match(rule.selector, /\.theater-secondary-surface \*/)
    assert.match(rule.selector, /:has\(\.theater-secondary-surface\)/)
    assert.doesNotMatch(rule.selector, /v-binder|option-body|\[class\*/)
  })
  assert.equal(count, 1)
  const vars = declarations(overlay, '.n-dropdown-menu')
  for (const state of ['hover', 'pending', 'active', 'active-pending']) {
    assert.equal(vars.get(`--n-option-color-${state}`), `var(--sc-bg-${state.includes('active') ? 'active' : 'hover'})`)
  }
  assert.equal(vars.get('--n-group-header-text-color'), 'var(--sc-text-secondary)')
  assert.equal(vars.get('--n-color-modal'), 'var(--sc-bg-elevated)')
  assert.equal(vars.get('--n-panel-color'), 'var(--sc-bg-elevated)')
  assert.equal(vars.get('--n-panel-text-color'), 'var(--sc-text-primary)')
  assert.equal(vars.get('--n-item-text-color'), 'var(--sc-text-primary)')
  assert.equal(vars.get('--n-item-color-hover'), 'var(--sc-bg-hover)')
  assert.equal(vars.get('--n-item-color-active'), 'var(--sc-bg-active)')
  for (const surface of ['.n-date-panel', '.n-time-picker-panel', '.n-color-picker-panel']) {
    assert.equal(declarations(overlay, surface).get('--n-panel-color'), 'var(--sc-bg-elevated)')
  }
  assert.equal(vars.has('--n-option-opacity-disabled'), false)
})

test('Teleported context menus use a shared unscoped adapter and retain submenu and picker contracts', () => {
  const vars = declarations(overlay, '.sc-context-menu')
  for (const prop of ['backgroud', 'hover-backgroud', 'active-backgroud', 'text', 'divider']) {
    assert.match(vars.get(`--mx-menu-${prop}`), /^var\(--sc-/)
  }
  assert.equal(vars.get('font-family'), 'var(--sc-font-family)')
  assert.doesNotMatch(overlay.toString(), /data-v-|:deep\(/)
  for (const [file, hook] of [['ChatRightClickMenu', 'sc-message-context-menu'], ['AvatarClickMenu', 'sc-avatar-context-menu']]) {
    const source = read(`src/views/chat/components/${file}.vue`)
    assert.match(source, new RegExp(`customClass: .*sc-context-menu ${hook}`))
    assert.match(source, /theme: .*Theme\.value/)
    assert.doesNotMatch(source, /:deep\(\.(?:mx-)?context-menu/)
  }
  // Vendor renders the same injected options on each submenu, even after Teleport.
  const vendor = read('node_modules/@imengyu/vue3-context-menu/src/ContextSubMenu.vue')
  assert.match(vendor, /options\.customClass/)
  assert.match(vendor, /options = inject\('globalOptions'\)/)
  assert.match(read('src/views/chat/components/ChatRightClickMenu.vue'), /min-height: 57px/)
})

test('owned floating surfaces compile to semantic colors, with real global tooltip selectors', () => {
  const ownedOverlay = declarations(overlay, '.sc-overlay {')
  assert.equal(ownedOverlay.has('border-radius'), false, 'shared owned-surface hook must not own geometry')
  assert.equal(ownedOverlay.has('box-shadow'), false, 'shared owned-surface hook must not own component shadow')
  const global = compile(read('src/views/chat/styles/chat.global.scss'))
  global.walkRules(rule => {
    if (rule.selector.includes('keyword-tooltip')) assert.doesNotMatch(rule.selector, /:global\(/)
  })
  for (const [selector, property, value] of [
    ['.keyword-tooltip {', 'background', 'var(--sc-bg-elevated)'],
    ['.keyword-tooltip__header', 'color', 'var(--sc-text-primary)'],
    ['.keyword-tooltip__body {', 'color', 'var(--sc-text-secondary)'],
    ['.keyword-tooltip__nav-btn:not(:disabled):hover', 'background', 'var(--sc-bg-hover)'],
  ]) {
    const values = declarations(global, selector)
    assert.equal(values.get(property), value)
  }
  const scoped = compile(read('src/views/chat/styles/chat.scoped.scss'), true)
  assert.equal(declarations(scoped, '.selection-floating-bar[').get('background'), 'var(--sc-bg-elevated)')
  assert.equal(declarations(scoped, '.selection-floating-bar[').get('position'), 'fixed')
  for (const [file, surface] of [
    ['components/chat/MentionSuggestPanel.vue', 'mention-suggest__content'],
    ['components/chat/KeywordSuggestPanel.vue', 'keyword-suggest__content'],
    ['components/chat/SlashSuggestPanel.vue', 'slash-suggest__content'],
    ['views/chat/components/MultiSelectFloatingBar.vue', 'multi-select-bar'],
    ['views/chat/components/ChatSearchPanel.vue', 'chat-search-panel'],
  ]) {
    const css = componentStyles(`src/${file}`)[0]
    const values = declarations(css, `.${surface}[`)
    assert.equal(values.get('background'), 'var(--sc-bg-elevated)', file)
    assert.doesNotMatch(css.toString(), new RegExp(`data-display-palette[^}]*\\.${surface}\\[.*background`))
  }
  const runtime = read('src/utils/keywordTooltip.ts')
  assert.match(runtime, /setAttribute\('data-sc-ui', 'keyword-tooltip'\)/)
  assert.match(runtime, /className = 'keyword-tooltip sc-overlay'/)
})

test('built-in packages delegate overlay internals to the baseline and match their sources', () => {
  for (const theme of ['rlyeh-archive', 'triangle-agency']) {
    const css = read(`../doc/ui-customization-package/theme-${theme}.css`)
    const pkg = JSON.parse(read(`../doc/ui-customization-package/sealchat-ui-package-${theme}.json`))
    assert.equal(pkg.css, css)
    for (const token of ['background', 'border-color', 'radius', 'shadow']) {
      assert.match(css, new RegExp(`--sc-overlay-${token}:`), `${theme} missing overlay ${token} token`)
    }
    assert.doesNotMatch(css, /mx-context-menu|v-binder-follower|n-dropdown-option-body|\.n-base-select-option/)
    assert.doesNotMatch(css, /:is\(\.n-modal,/)
  }
})
