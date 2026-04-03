import assert from 'node:assert/strict'
import { resolveIdentityMetaStyle } from '../src/utils/identityMetaContrast'

const disabled = resolveIdentityMetaStyle({
  enabled: false,
  kind: 'remark',
  identityColor: '#667085',
  backgroundColor: 'rgb(255, 255, 255)',
})

assert.equal(disabled.mode, 'disabled', '关闭自动适配时应返回 disabled 模式')
assert.equal(disabled.style.color, '#667085', '关闭自动适配时应保留原始文字色')

const preserved = resolveIdentityMetaStyle({
  enabled: true,
  kind: 'badge',
  identityColor: '#2563eb',
  backgroundColor: 'rgb(248, 250, 252)',
})

assert.equal(preserved.mode, 'normal', '高对比颜色应保持 normal 模式')
assert.ok(
  preserved.contrastRatio >= 4.5,
  `高对比模式下文字对比度应不低于 4.5，实际为 ${preserved.contrastRatio}`,
)

const adjusted = resolveIdentityMetaStyle({
  enabled: true,
  kind: 'remark',
  identityColor: '#475569',
  backgroundColor: 'rgb(51, 65, 85)',
})

assert.notEqual(adjusted.style.color, '#475569', '低对比颜色应调整文字色')
assert.ok(
  adjusted.contrastRatio >= 4.5,
  `低对比模式下文字对比度应提升到 4.5 以上，实际为 ${adjusted.contrastRatio}`,
)
assert.ok(
  adjusted.mode === 'adjusted' || adjusted.mode === 'fallback',
  `低对比颜色应进入 adjusted 或 fallback，实际为 ${adjusted.mode}`,
)

console.log('identity meta contrast regressions passed')
