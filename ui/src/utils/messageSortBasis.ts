export type MessageSortBasis = 'typing_start' | 'send_time';

export function resolveMessageSortBasis(worldValues: readonly unknown[], legacyValue: unknown): MessageSortBasis {
  for (const value of worldValues) {
    if (value === 'typing_start' || value === 'send_time') {
      return value;
    }
  }
  const legacy = typeof legacyValue === 'string' ? legacyValue.trim().toLowerCase() : '';
  return legacy === 'send_time' ? 'send_time' : 'typing_start';
}
