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

// Compatibility fallback for servers predating uiCustomization.maxCodeSizeKB.
export const DEFAULT_UI_CUSTOMIZATION_MAX_CODE_SIZE_KB = 1024

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
  maxCodeSizeKB: number
}

export function normalizeUICustomizationPolicy(policy?: Partial<UICustomizationPolicy> | null): UICustomizationPolicy {
  const maxCodeSizeKB = policy?.maxCodeSizeKB
  return {
    allowWorldCss: policy?.allowWorldCss ?? true,
    allowWorldJs: policy?.allowWorldJs ?? false,
    allowPublicWorldCss: policy?.allowPublicWorldCss ?? true,
    maxCodeSizeKB: typeof maxCodeSizeKB === 'number' && Number.isFinite(maxCodeSizeKB) && maxCodeSizeKB > 0
      ? Math.trunc(maxCodeSizeKB)
      : DEFAULT_UI_CUSTOMIZATION_MAX_CODE_SIZE_KB,
  }
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
