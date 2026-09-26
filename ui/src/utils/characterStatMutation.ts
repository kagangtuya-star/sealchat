import { resolveTemplateValue } from './characterCardTemplate';
import { resolveCharacterCardValueContainer } from './characterCardValueMiddleware';
import { normalizeBotCommandPrefixes } from './botCommand';

type Linear = { coefficient: number; constant: number; references: number; path: string[] | null };
export type CharacterStatMutationTarget =
  | { kind: 'direct'; path: string[]; sourcePath: string }
  | { kind: 'inverse'; path: string[]; sourcePath: string; coefficient: number; constant: number }
  | null;

const forbidden = new Set(['__proto__', 'prototype', 'constructor']);
export const isSafeBotStatRoot = (path: string) => /^[\p{L}\p{N}_-]{1,128}$/u.test(path)
  && !forbidden.has(path) && !Number.isFinite(Number(path));

export const buildBotStatSetCommand = (path: string, value: number, prefixes?: unknown): string | null => {
  if (!isSafeBotStatRoot(path) || !Number.isFinite(value)) return null;
  const prefix = normalizeBotCommandPrefixes(prefixes)[0] || '.';
  return `${prefix}st ${path}=${value}`;
};

const own = (value: unknown, key: string) => value !== null && typeof value === 'object'
  && Object.prototype.propertyIsEnumerable.call(value, key);
const numeric = (value: unknown): number | null => {
  if (value === null || value === undefined || value === '') return null;
  const result = typeof value === 'number' ? value : Number(String(value).trim());
  return Number.isFinite(result) ? result : null;
};

const pathFor = (attrs: Record<string, unknown>, raw: string, allowRootInit: boolean): string[] | null => {
  if (!raw || forbidden.has(raw)) return null;
  if (own(attrs, raw)) return [raw]; // The reader prefers an exact dotted root key.
  const parts = raw.split('.');
  if (parts.some(part => !part || forbidden.has(part))) return null;
  if (parts.length === 1) return allowRootInit && isSafeBotStatRoot(raw)
    && resolveTemplateValue(attrs, raw) == null ? parts : null;
  let cursor: unknown = attrs;
  for (let index = 0; index < parts.length - 1; index += 1) {
    const container = resolveCharacterCardValueContainer(cursor);
    if (!container || !own(container, parts[index])) return null;
    if (Array.isArray(container) && !validArrayIndex(container, parts[index])) return null;
    cursor = (container as Record<string, unknown>)[parts[index]];
  }
  const parent = resolveCharacterCardValueContainer(cursor);
  if (!parent || (Array.isArray(parent) && !validArrayIndex(parent, parts.at(-1)!))) return null;
  return parts;
};

const validArrayIndex = (array: unknown[], key: string) => /^(0|[1-9]\d*)$/.test(key) && Number(key) < array.length;

/** Patch only an existing chain of containers; preserve JSON-string wrappers and numeric-string leaves. */
export const patchCharacterCardValuePath = (
  attrs: Record<string, unknown>, path: string[], value: number,
): Record<string, unknown> | null => {
  if (!attrs || !path.length || !Number.isFinite(value) || path.some(part => !part || forbidden.has(part))) return null;
  const patch = (original: unknown, offset: number): unknown | null => {
    const container = resolveCharacterCardValueContainer(original);
    if (!container) return null;
    const key = path[offset];
    if (Array.isArray(container) && !validArrayIndex(container, key)) return null;
    if (offset < path.length - 1 && !own(container, key)) return null;
    const next = offset === path.length - 1
      ? typeof (container as Record<string, unknown>)[key] === 'string' && numeric((container as Record<string, unknown>)[key]) !== null
        ? String(value) : value
      : patch((container as Record<string, unknown>)[key], offset + 1);
    if (next === null) return null;
    const copy = Array.isArray(container) ? [...container] : { ...container };
    (copy as Record<string, unknown>)[key] = next;
    if (typeof original !== 'string') return copy;
    let wrappers = 0;
    let wrapped: unknown = original;
    while (typeof wrapped === 'string' && wrappers < 8) {
      try { wrapped = JSON.parse(wrapped); } catch { return null; }
      wrappers += 1;
    }
    if (typeof wrapped === 'string') return null;
    let result: unknown = copy;
    for (let count = 0; count < wrappers; count += 1) result = JSON.stringify(result);
    return result;
  };
  return patch(attrs, 0) as Record<string, unknown> | null;
};

/** Parse only one-reference linear arithmetic from the existing template expression syntax. */
export const resolveCharacterStatMutationTarget = (
  sourcePath: string, attrs: Record<string, unknown>, options: { allowRootInit?: boolean; directOnly?: boolean } = {},
): CharacterStatMutationTarget => {
  const source = String(sourcePath || '').trim();
  const wrapped = source.match(/^\{([^{}]+)\}$/);
  const expression = String(wrapped?.[1] ?? source).trim();
  if (!expression || expression.length > 512) return null;
  const directPath = pathFor(attrs, expression, options.allowRootInit === true);
  if (directPath && (!/[+\-*/()]/.test(expression) || own(attrs, expression))) {
    return { kind: 'direct', path: directPath, sourcePath: source };
  }
  if (options.directOnly) return null;
  let index = 0;
  let depth = 0;
  const skip = () => { while (/\s/.test(expression[index] || '')) index += 1; };
  const constant = (value: number): Linear => ({ coefficient: 0, constant: value, references: 0, path: null });
  const parsePrimary = (): Linear | null => {
    skip();
    if (expression[index] === '(') {
      if (++depth > 32) return null;
      index += 1;
      const result = parseSum();
      depth -= 1;
      skip();
      if (expression[index++] !== ')') return null;
      return result;
    }
    const number = expression.slice(index).match(/^(?:\d+(?:\.\d*)?|\.\d+)/);
    if (number) { index += number[0].length; return constant(Number(number[0])); }
    const start = index;
    while (index < expression.length && !/[+\-*/()\s]/.test(expression[index])) index += 1;
    if (start === index) return null;
    const path = pathFor(attrs, expression.slice(start, index), options.allowRootInit === true);
    return path ? { coefficient: 1, constant: 0, references: 1, path } : null;
  };
  const parseUnary = (): Linear | null => {
    skip();
    if (expression[index] !== '+' && expression[index] !== '-') return parsePrimary();
    const negative = expression[index++] === '-';
    const value = parseUnary();
    return value && negative ? { ...value, coefficient: -value.coefficient, constant: -value.constant } : value;
  };
  const combine = (a: Linear, b: Linear, op: string): Linear | null => {
    if (op === '+' || op === '-') {
      const sign = op === '+' ? 1 : -1;
      return { coefficient: a.coefficient + sign * b.coefficient, constant: a.constant + sign * b.constant,
        references: a.references + b.references, path: a.path || b.path };
    }
    if (op === '*') {
      if (a.references && b.references) return null;
      const variable = a.references ? a : b;
      const fixed = a.references ? b.constant : a.constant;
      return { coefficient: variable.coefficient * fixed, constant: variable.constant * fixed,
        references: a.references + b.references, path: variable.path };
    }
    if (b.references || b.constant === 0) return null;
    return { coefficient: a.coefficient / b.constant, constant: a.constant / b.constant,
      references: a.references, path: a.path };
  };
  const parseProduct = (): Linear | null => {
    let value = parseUnary();
    while (value) {
      skip();
      const op = expression[index];
      if (op !== '*' && op !== '/') break;
      index += 1;
      const right = parseUnary();
      value = right ? combine(value, right, op) : null;
    }
    return value;
  };
  const parseSum = (): Linear | null => {
    let value = parseProduct();
    while (value) {
      skip();
      const op = expression[index];
      if (op !== '+' && op !== '-') break;
      index += 1;
      const right = parseProduct();
      value = right ? combine(value, right, op) : null;
    }
    return value;
  };
  const linear = parseSum();
  skip();
  if (!linear || index !== expression.length || linear.references !== 1 || !linear.path
    || !Number.isFinite(linear.coefficient) || linear.coefficient === 0 || !Number.isFinite(linear.constant)) return null;
  return { kind: 'inverse', path: linear.path, sourcePath: source,
    coefficient: linear.coefficient, constant: linear.constant };
};

export const resolveCharacterStatInverseValue = (
  target: Exclude<CharacterStatMutationTarget, null>, attrs: Record<string, unknown>, displayValue: number,
): number | null => {
  if (!Number.isFinite(displayValue)) return null;
  const result = target.kind === 'direct' ? displayValue : (displayValue - target.constant) / target.coefficient;
  const raw = Object.is(result, -0) ? 0 : result;
  if (!Number.isFinite(raw)) return null;
  const patched = patchCharacterCardValuePath(attrs, target.path, raw);
  if (!patched) return null;
  const forward = numeric(resolveTemplateValue(patched, target.sourcePath));
  return forward !== null && Math.abs(forward - displayValue) <= 1e-8 * Math.max(1, Math.abs(displayValue)) ? raw : null;
};
