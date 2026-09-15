import { isTipTapJson, plainTextToTiptapJson, tiptapJsonToPlainText } from './tiptap-render'

export type TextContentFormat = 'plain' | 'tiptap'

export function convertTextContentFormat(
  content: string,
  from: TextContentFormat,
  to: TextContentFormat,
): string {
  if (from === to) return content

  if (from === 'plain' && to === 'tiptap') {
    return JSON.stringify(
      plainTextToTiptapJson(content, {
        preserveWhitespaceOnly: true,
      }),
    )
  }

  if (!isTipTapJson(content)) return content
  return tiptapJsonToPlainText(content, { trimTrailingNewlines: false })
}
