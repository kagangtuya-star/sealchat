// SealChat UI Package 公开契约常量。
// 来源（2026-11-12 核实）：
//   ui/src/services/ui-customization/surfaces.ts      UI_SURFACES / UI_PUBLIC_SURFACES
//   ui/src/services/ui-customization/jsRuntime.ts     UI_CUSTOMIZATION_API_VERSION
//   service/ui_customization_transfer.go              format / formatVersion / runtimeApiVersion
//   service/ui_customization.go                       名称、说明、Surface 数量限制
//   service/channel_extension.go                      ChannelExtensionCapabilities
// SealChat 升级后先按 references/implementation-reference.md §8 复核。

export const PACKAGE_FORMAT = 'sealchat.ui-package'
export const BUNDLE_FORMAT = 'sealchat.ui-bundle'
export const FORMAT_VERSION = 1
export const RUNTIME_API_VERSION = 2
export const MAX_NAME = 100
export const MAX_DESCRIPTION = 500
export const MAX_SURFACES = 32
export const DEFAULT_MAX_CODE_KB = 1024

export const SURFACES = [
  'auth', 'lobby', 'world-detail', 'chat', 'split', 'embed', 'theater',
  'observer', 'invite', 'log-layout', 'log-public', 'status', 'internal', 'global',
]
export const PUBLIC_SURFACES = ['observer', 'embed', 'log-public']

export const CHANNEL_CAPABILITIES = [
  'context.read', 'user.read', 'members.read', 'world.admins.read', 'characters.read',
  'characterCard.read', 'characterCard.write', 'permissions.read', 'storage.read', 'storage.write',
  'events.subscribe', 'events.publish', 'messages.send', 'attachments.upload',
  'theater.dialogue.subscribe', 'theater.character.read',
]
export const WRITE_CAPABILITIES = ['characterCard.write', 'attachments.upload', 'storage.write', 'events.publish', 'messages.send']
export const PROVIDER_CAPABILITIES = ['theater.dialogue.subscribe', 'theater.character.read']

// 21 个传统颜色字段对应的 CSS 变量（开发指南 §8）。
export const LEGACY_THEME_VARIABLES = [
  '--sc-bg-surface', '--sc-bg-elevated', '--sc-bg-input', '--sc-bg-header',
  '--sc-text-primary', '--sc-text-secondary',
  '--custom-chat-ic-bg', '--custom-chat-ooc-bg', '--custom-chat-stage-bg',
  '--custom-chat-preview-bg', '--custom-chat-preview-dot',
  '--sc-border-mute', '--sc-border-strong',
  '--primary-color', '--primary-color-hover', '--sc-action-ribbon-hover-text',
  '--custom-keyword-bg', '--custom-keyword-border',
  '--chat-inline-code-bg', '--chat-inline-code-fg', '--chat-inline-code-border',
]

export const PROTECTED_PERFORMANCE_CLASSES = ['tiptap-performance', 'twin-layer-message__char', 'twin-layer-message__char-glyph']
export const PROTECTED_PERFORMANCE_PROPERTIES = /^(animation(-[a-z-]+)?|display|transform|transform-origin|filter|opacity)$/
