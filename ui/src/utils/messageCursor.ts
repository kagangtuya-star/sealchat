export interface MessageCursorLike {
  id?: string | null;
  createdAt?: unknown;
  created_at?: unknown;
  displayOrder?: unknown;
  display_order?: unknown;
}

const normalizeNumericValue = (value: unknown): number | null => {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : null;
  }
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) {
      return null;
    }
    const numeric = Number(trimmed);
    return Number.isFinite(numeric) ? numeric : null;
  }
  if (value instanceof Date) {
    const ts = value.getTime();
    return Number.isFinite(ts) ? ts : null;
  }
  return null;
};

export const buildMessageCursor = (message?: MessageCursorLike | null): string => {
  const id = String(message?.id || '').trim();
  if (!id) {
    return '';
  }

  const createdAt = normalizeNumericValue(message?.createdAt ?? message?.created_at);
  if (createdAt === null) {
    return '';
  }

  const displayOrder = normalizeNumericValue(
    message?.displayOrder ?? message?.display_order ?? createdAt,
  );
  if (displayOrder === null) {
    return '';
  }

  return `${displayOrder.toFixed(8)}|${Math.floor(createdAt)}|${id}`;
};
