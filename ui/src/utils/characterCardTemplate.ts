import DOMPurify from 'dompurify';

export const DEFAULT_CARD_TEMPLATE = 'HP{生命值} SAN{理智} 闪避{闪避}';

const TEMPLATE_STORAGE_KEY_PREFIX = 'sealchat_card_template_';
const UNSAFE_TEMPLATE_PATH_SEGMENTS = new Set(['__proto__', 'prototype', 'constructor']);
const MAX_ADAPTIVE_TEMPLATE_SEARCH_NODES = 10_000;

type TemplateValueLookup =
  | { found: true; value: unknown }
  | { found: false };

function isObjectLike(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function readOwnEnumerableValue(source: unknown, key: string): TemplateValueLookup {
  if (!isObjectLike(source) || !Object.prototype.propertyIsEnumerable.call(source, key)) {
    return { found: false };
  }
  return { found: true, value: source[key] };
}

function isSafeTemplatePathSegment(segment: string): boolean {
  return !!segment && !UNSAFE_TEMPLATE_PATH_SEGMENTS.has(segment);
}

function resolveExplicitTemplatePath(data: Record<string, any>, key: string): unknown {
  const segments = key.split('.');
  if (!segments.every(isSafeTemplatePathSegment)) return undefined;

  let current: unknown = data;
  for (const segment of segments) {
    const result = readOwnEnumerableValue(current, segment);
    if (!result.found) return undefined;
    current = result.value;
  }
  return current;
}

function findAdaptiveTemplateValue(data: Record<string, any>, key: string): unknown {
  if (!isSafeTemplatePathSegment(key)) return undefined;

  const visited = new WeakSet<object>();
  const pending: Array<{ key: string; value: unknown }> = [];
  if (isObjectLike(data)) {
    visited.add(data);
    const rootKeys = Object.keys(data);
    for (let index = rootKeys.length - 1; index >= 0; index -= 1) {
      const childKey = rootKeys[index];
      if (isSafeTemplatePathSegment(childKey)) {
        pending.push({ key: childKey, value: data[childKey] });
      }
    }
  }

  let scanned = 0;
  while (pending.length > 0 && scanned < MAX_ADAPTIVE_TEMPLATE_SEARCH_NODES) {
    const current = pending.pop()!;
    scanned += 1;
    if (current.key === key) return current.value;
    if (!isObjectLike(current.value) || visited.has(current.value)) continue;

    visited.add(current.value);
    const childKeys = Object.keys(current.value);
    for (let index = childKeys.length - 1; index >= 0; index -= 1) {
      const childKey = childKeys[index];
      if (isSafeTemplatePathSegment(childKey)) {
        pending.push({ key: childKey, value: current.value[childKey] });
      }
    }
  }

  return undefined;
}

/**
 * Resolve a badge placeholder from character attributes.
 * Exact root keys win. Dotted placeholders use an explicit path; unresolved
 * single-key placeholders fall back to a deterministic depth-first JSON search.
 */
export function resolveTemplateValue(data: Record<string, any>, rawKey: string): unknown {
  const key = String(rawKey).trim();
  if (!key || !isSafeTemplatePathSegment(key)) return undefined;

  const direct = readOwnEnumerableValue(data, key);
  if (direct.found) return direct.value;
  if (key.includes('.')) return resolveExplicitTemplatePath(data, key);
  return findAdaptiveTemplateValue(data, key);
}

const escapeHtmlText = (input: unknown): string => String(input)
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;');

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

  let html = template.replace(/\{([^{}]+)\}/g, (_match, rawKey) => {
    const key = String(rawKey).trim();
    if (!key) return '';
    const val = resolveTemplateValue(data, key);
    return val !== undefined && val !== null ? escapeHtmlText(val) : '';
  });

  // Remove any remaining unmatched placeholders
  html = html.replace(/\{[^{}]+\}/g, '');

  return DOMPurify.sanitize(html.trim(), {
    ALLOWED_TAGS: ['span', 'b', 'i', 'strong', 'em'],
    ALLOWED_ATTR: ['class', 'style'],
  });
}

/**
 * Extract placeholder keys from a character card template.
 */
export function extractTemplateKeys(template: string): string[] {
  if (!template) return [];
  const keys: string[] = [];
  const seen = new Set<string>();
  template.replace(/\{([^{}]+)\}/g, (_match, rawKey) => {
    const key = String(rawKey).trim();
    if (!key || seen.has(key)) return '';
    seen.add(key);
    keys.push(key);
    return '';
  });
  return keys;
}

export function hasRenderableBadgeData(template: string, data?: Record<string, any>): boolean {
  if (!template || !data) return false;
  const keys = extractTemplateKeys(template);
  if (keys.length === 0) return false;
  return keys.some((key) => {
    const value = resolveTemplateValue(data, key);
    return value !== undefined && value !== null && value !== '';
  });
}
