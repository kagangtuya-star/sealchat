function normalizeLookupText(text: string) {
  return String(text || '').trim().toLowerCase()
}

type KeywordLikeItem = {
  keyword: string
  aliases?: string[]
}

export function dedupeEffectiveKeywordsByKeyword<T extends KeywordLikeItem>(items: T[]) {
  const seen = new Set<string>()
  return items.filter((item) => {
    const dedupeKey = normalizeLookupText(item.keyword)
    if (!dedupeKey || seen.has(dedupeKey)) {
      return false
    }
    seen.add(dedupeKey)
    return true
  })
}

export function filterExactEffectiveKeywordCandidates<T extends KeywordLikeItem>(
  items: T[],
  matchedText: string,
): T[] {
  const normalizedMatchedText = normalizeLookupText(matchedText)
  if (!normalizedMatchedText) {
    return []
  }
  return items.filter((item) => {
    if (normalizeLookupText(item.keyword) === normalizedMatchedText) {
      return true
    }
    return (item.aliases || []).some((alias) => normalizeLookupText(alias) === normalizedMatchedText)
  })
}
