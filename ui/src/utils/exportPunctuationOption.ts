export const buildAutoCorrectPunctuationExportPayload = (value?: boolean) => ({
  auto_correct_punctuation: value ?? true,
})
