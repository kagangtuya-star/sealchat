export interface ParenOocSendOverrideInput {
  content: string
  inputMode: string
  enabled: boolean
  useOocRole: boolean
  hasExplicitIdentityOverride?: boolean
  oocRoleId?: string | null
  isReedit?: boolean
}

export interface ParenOocSendOverride {
  icMode: 'ooc'
  identityId?: string
}

export const startsWithOocParen = (content: string) => {
  const normalized = String(content || '').trimStart()
  return normalized.startsWith('(') || normalized.startsWith('（')
}

export const resolveParenOocSendOverride = (
  input: ParenOocSendOverrideInput,
): ParenOocSendOverride | null => {
  if (!input.enabled || input.inputMode !== 'plain' || input.isReedit) {
    return null
  }
  if (!startsWithOocParen(input.content)) {
    return null
  }

  const oocRoleId = String(input.oocRoleId || '').trim()
  if (input.useOocRole && !input.hasExplicitIdentityOverride && oocRoleId) {
    return { icMode: 'ooc', identityId: oocRoleId }
  }
  return { icMode: 'ooc' }
}
