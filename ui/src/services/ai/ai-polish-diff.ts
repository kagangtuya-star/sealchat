export type AIPolishDiffTokenType = 'equal' | 'insert' | 'delete'

export interface AIPolishDiffToken {
  type: AIPolishDiffTokenType
  text: string
  subtle?: boolean
}

function getCommonPrefixLength(source: string, result: string): number {
  const limit = Math.min(source.length, result.length)
  let index = 0
  while (index < limit && source[index] === result[index]) {
    index += 1
  }
  return index
}

function getCommonSuffixLength(source: string, result: string, prefixLength: number): number {
  const sourceRemaining = source.length - prefixLength
  const resultRemaining = result.length - prefixLength
  const limit = Math.min(sourceRemaining, resultRemaining)
  let count = 0
  while (
    count < limit
    && source[source.length - 1 - count] === result[result.length - 1 - count]
  ) {
    count += 1
  }
  return count
}

function isSubtleText(value: string): boolean {
  return /^[\s，。！？；：、“”‘’（）《》〈〉【】『』〔〕…,.!?;:'"()[\]{}\-—]*$/u.test(value)
}

function pushToken(tokens: AIPolishDiffToken[], token: AIPolishDiffToken) {
  if (!token.text) {
    return
  }
  const subtle = token.type === 'equal' ? false : !!token.subtle
  const last = tokens[tokens.length - 1]
  if (last && last.type === token.type && !!last.subtle === subtle) {
    last.text += token.text
    return
  }
  tokens.push({
    type: token.type,
    text: token.text,
    ...(subtle ? { subtle: true } : {}),
  })
}

function diffChars(source: string, result: string): AIPolishDiffToken[] {
  const sourceChars = Array.from(source)
  const resultChars = Array.from(result)
  const rows = sourceChars.length + 1
  const cols = resultChars.length + 1
  const matrix: number[][] = Array.from({ length: rows }, () => Array(cols).fill(0))

  for (let i = 1; i < rows; i += 1) {
    for (let j = 1; j < cols; j += 1) {
      if (sourceChars[i - 1] === resultChars[j - 1]) {
        matrix[i][j] = matrix[i - 1][j - 1] + 1
      } else {
        matrix[i][j] = Math.max(matrix[i - 1][j], matrix[i][j - 1])
      }
    }
  }

  const reversed: AIPolishDiffToken[] = []
  let i = sourceChars.length
  let j = resultChars.length

  while (i > 0 && j > 0) {
    if (sourceChars[i - 1] === resultChars[j - 1]) {
      pushToken(reversed, { type: 'equal', text: sourceChars[i - 1] })
      i -= 1
      j -= 1
      continue
    }
    if (matrix[i - 1][j] >= matrix[i][j - 1]) {
      pushToken(reversed, {
        type: 'delete',
        text: sourceChars[i - 1],
        subtle: isSubtleText(sourceChars[i - 1]),
      })
      i -= 1
      continue
    }
    pushToken(reversed, {
      type: 'insert',
      text: resultChars[j - 1],
      subtle: isSubtleText(resultChars[j - 1]),
    })
    j -= 1
  }

  while (i > 0) {
    pushToken(reversed, {
      type: 'delete',
      text: sourceChars[i - 1],
      subtle: isSubtleText(sourceChars[i - 1]),
    })
    i -= 1
  }

  while (j > 0) {
    pushToken(reversed, {
      type: 'insert',
      text: resultChars[j - 1],
      subtle: isSubtleText(resultChars[j - 1]),
    })
    j -= 1
  }

  return reversed.reverse().map((token) => ({
    ...token,
    text: Array.from(token.text).reverse().join(''),
  }))
}

function normalizeTokens(tokens: AIPolishDiffToken[]): AIPolishDiffToken[] {
  const merged: AIPolishDiffToken[] = []
  tokens.forEach((token) => {
    pushToken(merged, {
      ...token,
      subtle: token.type === 'equal' ? false : token.subtle || isSubtleText(token.text),
    })
  })

  const normalized: AIPolishDiffToken[] = []
  for (let index = 0; index < merged.length; index += 1) {
    const current = merged[index]
    const next = merged[index + 1]

    if (current?.type === 'insert' && next?.type === 'delete') {
      normalized.push(next, current)
      index += 1
      continue
    }

    normalized.push(current)
  }

  return normalized
}

export function buildAIPolishDiffTokens(sourceText: string, resultText: string): AIPolishDiffToken[] {
  if (!resultText) {
    return []
  }
  if (!sourceText) {
    return [{ type: 'equal', text: resultText }]
  }
  if (sourceText === resultText) {
    return [{ type: 'equal', text: resultText }]
  }

  const prefixLength = getCommonPrefixLength(sourceText, resultText)
  const suffixLength = getCommonSuffixLength(sourceText, resultText, prefixLength)
  const sourceMiddleEnd = sourceText.length - suffixLength
  const resultMiddleEnd = resultText.length - suffixLength
  const tokens: AIPolishDiffToken[] = []

  if (prefixLength > 0) {
    tokens.push({ type: 'equal', text: sourceText.slice(0, prefixLength) })
  }

  tokens.push(...diffChars(
    sourceText.slice(prefixLength, sourceMiddleEnd),
    resultText.slice(prefixLength, resultMiddleEnd),
  ))

  if (suffixLength > 0) {
    tokens.push({ type: 'equal', text: resultText.slice(resultMiddleEnd) })
  }

  return normalizeTokens(tokens)
}
