export interface BotTokenCopyHandlers {
  copyText: (text: string) => Promise<boolean>
  onCopySuccess: () => void
  onManualCopyRequired: (token: string) => void
}

export const copyBotTokenWithPreviewFallback = async (
  value: string | undefined,
  handlers: BotTokenCopyHandlers,
): Promise<void> => {
  const token = String(value || '').trim()
  if (!token) {
    return
  }

  const copied = await handlers.copyText(token)
  if (copied) {
    handlers.onCopySuccess()
    return
  }

  handlers.onManualCopyRequired(token)
}
