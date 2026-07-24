const MAX_STRUCTURED_VALUE_DEPTH = 8;

const isContainer = (value: unknown): value is Record<string, unknown> | unknown[] => (
  typeof value === 'object' && value !== null
);

/**
 * Expose JSON-encoded character-card values as containers without mutating attrs.
 * Some character APIs return nested VM values as JSON strings.
 */
export const resolveCharacterCardValueContainer = (
  value: unknown,
): Record<string, unknown> | unknown[] | null => {
  let current = value;
  for (let depth = 0; depth < MAX_STRUCTURED_VALUE_DEPTH; depth += 1) {
    if (isContainer(current)) return current;
    if (typeof current !== 'string') return null;
    const trimmed = current.trim();
    if (!trimmed || !['{', '[', '"'].includes(trimmed[0])) return null;
    try {
      current = JSON.parse(trimmed);
    } catch {
      return null;
    }
  }
  return isContainer(current) ? current : null;
};
