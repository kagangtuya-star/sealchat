export function formatWorldKeywordSourceLabel(
  sourceName?: string,
  category?: string,
  hasExternalLibrarySource = true,
) {
  if (!hasExternalLibrarySource) {
    return ''
  }
  const normalizedSourceName = String(sourceName || '').trim()
  const normalizedCategory = String(category || '').trim()

  if (!normalizedSourceName) {
    return normalizedCategory
  }
  if (!normalizedCategory) {
    return normalizedSourceName
  }
  return `${normalizedSourceName}-${normalizedCategory}`
}
