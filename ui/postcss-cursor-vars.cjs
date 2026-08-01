const cursorKeywords = new Set(['default', 'pointer', 'text', 'grab', 'grabbing', 'not-allowed'])

module.exports = () => ({
  postcssPlugin: 'sealchat-cursor-vars',
  Declaration(declaration) {
    if (declaration.prop !== 'cursor') return
    const value = declaration.value.trim()
    if (!cursorKeywords.has(value)) return
    declaration.value = `var(--sc-cursor-${value}, ${value})`
  },
})

module.exports.postcss = true
