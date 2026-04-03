export interface KeywordTooltipCandidate {
  keywordId: string
  matchedVia?: string
}

export interface KeywordTooltipSessionState {
  activeIndex: number
  candidates: KeywordTooltipCandidate[]
}

const KEYWORD_CANDIDATES_DATASET_KEY = 'keywordCandidates'
const KEYWORD_ACTIVE_INDEX_DATASET_KEY = 'keywordActiveIndex'

function normalizeCandidates(candidates: KeywordTooltipCandidate[]) {
  const seen = new Set<string>()
  return candidates.filter((candidate) => {
    const keywordId = String(candidate.keywordId || '').trim()
    if (!keywordId || seen.has(keywordId)) {
      return false
    }
    seen.add(keywordId)
    candidate.keywordId = keywordId
    return true
  })
}

export function serializeKeywordTooltipCandidates(candidates: KeywordTooltipCandidate[]) {
  return JSON.stringify(
    normalizeCandidates(
      candidates.map((candidate) => ({
        keywordId: candidate.keywordId,
        matchedVia: candidate.matchedVia,
      })),
    ),
  )
}

export function parseKeywordTooltipSession(
  target: HTMLElement,
  fallbackKeywordId?: string | null,
  fallbackMatchedVia?: string,
): KeywordTooltipSessionState {
  const rawCandidates = target.dataset[KEYWORD_CANDIDATES_DATASET_KEY]
  let candidates: KeywordTooltipCandidate[] = []

  if (rawCandidates) {
    try {
      const parsed = JSON.parse(rawCandidates)
      if (Array.isArray(parsed)) {
        candidates = normalizeCandidates(
          parsed.map((candidate) => ({
            keywordId: String(candidate?.keywordId || ''),
            matchedVia: typeof candidate?.matchedVia === 'string' ? candidate.matchedVia : undefined,
          })),
        )
      }
    } catch {
      candidates = []
    }
  }

  if (!candidates.length && fallbackKeywordId) {
    candidates = [{ keywordId: fallbackKeywordId, matchedVia: fallbackMatchedVia }]
  }

  const fallbackIndex = fallbackKeywordId
    ? candidates.findIndex((candidate) => candidate.keywordId === fallbackKeywordId)
    : -1
  const parsedIndex = Number.parseInt(target.dataset[KEYWORD_ACTIVE_INDEX_DATASET_KEY] || '', 10)
  const activeIndex = Number.isInteger(parsedIndex) && parsedIndex >= 0 && parsedIndex < candidates.length
    ? parsedIndex
    : fallbackIndex >= 0
      ? fallbackIndex
      : 0

  return {
    candidates,
    activeIndex,
  }
}

export function resolveActiveKeywordTooltipCandidate(session: KeywordTooltipSessionState) {
  return session.candidates[session.activeIndex] || null
}

export function applyKeywordTooltipSessionToElement(
  target: HTMLElement,
  session: KeywordTooltipSessionState,
) {
  const normalizedCandidates = normalizeCandidates([...session.candidates])
  const nextActiveIndex = normalizedCandidates.length === 0
    ? 0
    : Math.max(0, Math.min(session.activeIndex, normalizedCandidates.length - 1))
  const activeCandidate = normalizedCandidates[nextActiveIndex]

  target.dataset[KEYWORD_CANDIDATES_DATASET_KEY] = serializeKeywordTooltipCandidates(normalizedCandidates)
  target.dataset[KEYWORD_ACTIVE_INDEX_DATASET_KEY] = String(nextActiveIndex)

  if (activeCandidate) {
    target.dataset.keywordId = activeCandidate.keywordId
    if (activeCandidate.matchedVia) {
      target.dataset.keywordSource = activeCandidate.matchedVia
    } else {
      delete target.dataset.keywordSource
    }
  }
}

export function resetKeywordTooltipSessionOnElement(target: HTMLElement) {
  const session = parseKeywordTooltipSession(target, target.dataset.keywordId, target.dataset.keywordSource)
  if (!session.candidates.length) {
    return
  }
  applyKeywordTooltipSessionToElement(target, {
    candidates: session.candidates,
    activeIndex: 0,
  })
}
