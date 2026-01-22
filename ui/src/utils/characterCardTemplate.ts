import DOMPurify from 'dompurify';

export const DEFAULT_CARD_TEMPLATE = 'HP {hp}/{hpmax} SAN {san}';

const TEMPLATE_STORAGE_KEY_PREFIX = 'sealchat_card_template_';

/**
 * Get the character card template for a specific world.
 */
export function getWorldCardTemplate(worldId: string): string {
  if (!worldId) return DEFAULT_CARD_TEMPLATE;
  try {
    const stored = localStorage.getItem(TEMPLATE_STORAGE_KEY_PREFIX + worldId);
    return stored || DEFAULT_CARD_TEMPLATE;
  } catch {
    return DEFAULT_CARD_TEMPLATE;
  }
}

/**
 * Save the character card template for a specific world.
 */
export function setWorldCardTemplate(worldId: string, template: string) {
  if (!worldId) return;
  try {
    localStorage.setItem(TEMPLATE_STORAGE_KEY_PREFIX + worldId, template);
  } catch (e) {
    console.warn('Failed to save character card template', e);
  }
}

/**
 * Render a character card template with data.
 * Sanitizes the output to prevent XSS.
 */
export function renderCardTemplate(template: string, data: Record<string, any>): string {
  if (!template || !data) return '';

  let html = template.replace(/\{([a-zA-Z0-9_]+)\}/g, (_match, key) => {
    const val = data[key];
    return val !== undefined && val !== null ? String(val) : '';
  });

  // Remove any remaining unmatched placeholders
  html = html.replace(/\{[a-zA-Z0-9_]+\}/g, '');

  return DOMPurify.sanitize(html.trim(), {
    ALLOWED_TAGS: ['span', 'b', 'i', 'strong', 'em'],
    ALLOWED_ATTR: ['class', 'style'],
  });
}
