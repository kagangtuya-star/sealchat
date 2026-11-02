import type { UIRuntimeSurface, UISurface } from './surfaces'

export interface UICustomizationContext {
  surface: UIRuntimeSurface
  worldId: string
  channelId: string
  slug: string
  observer: boolean
  publicSurface: boolean
}

export type UICustomizationScopeType = 'platform' | 'world'

export interface UICustomizationPackage {
  id: string
  ref: string
  origin: 'user' | 'builtin'
  builtinKey?: string
  version?: string
  readonly: boolean
  scopeType: UICustomizationScopeType
  scopeId: string
  name: string
  description: string
  enabled: boolean
  sortOrder: number
  surfaces: UISurface[]
  css: string
  js: string
  jsHash: string
  channelCapabilities?: string[]
  runtimeFingerprint?: string
  createdBy: string
  updatedBy: string
  createdAt: number
  updatedAt: number
  worldEnabled?: boolean
}

export interface UICustomizationPolicy {
  allowWorldCss: boolean
  allowWorldJs: boolean
  allowPublicWorldCss: boolean
}

export type UICustomizationPackageInput = Partial<Pick<UICustomizationPackage, 'name' | 'description' | 'enabled' | 'surfaces' | 'css' | 'js' | 'channelCapabilities'>>

// js is empty whenever the server decided the script may not run here.
// requiresTrust marks world scripts that also need per-browser trust.
export type RuntimePackage = Pick<UICustomizationPackage, 'id' | 'scopeType' | 'name' | 'sortOrder' | 'createdAt' | 'css'>
  & { ref: string; origin: 'user' | 'builtin'; version?: string; js?: string; jsHash?: string; requiresTrust?: boolean; channelCapabilities?: string[]; runtimeFingerprint?: string }
export interface UICustomizationRuntimeResult {
  surface: UIRuntimeSurface
  worldId: string
  channelId: string
  packages: RuntimePackage[]
}
