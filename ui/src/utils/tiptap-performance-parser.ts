import { normalizePerformanceEffect, type PerformanceEffect, type PerformanceEnterMode, type PerformanceScale } from './tiptap-performance-mark';
import type { PerformanceCommandType } from './tiptap-performance-node';
import { MESSAGE_ACTION_NODE_TYPE, normalizeMessageActionAttrs } from './tiptap-message-action';

export type PerformanceInstruction =
  | {
      type: 'char';
      char: string;
      effects: {
        effect?: PerformanceEffect;
        scale?: PerformanceScale;
        toneIntensity?: number;
        enterMode?: PerformanceEnterMode;
        enterSpeed?: number;
      };
      marks?: Array<{ type?: string; attrs?: Record<string, any> }>;
      index: number;
    }
  | {
      type: 'break';
      index: number;
    }
  | {
      type: 'command';
      command: PerformanceCommandType;
      value?: number;
      index: number;
    };

type TipTapNode = {
  type?: string;
  text?: string;
  marks?: Array<{ type?: string; attrs?: Record<string, any> }>;
  attrs?: Record<string, any>;
  content?: TipTapNode[];
};

const readPerformanceAttrs = (marks: TipTapNode['marks']) => {
  const mark = (marks || []).find((item) => item?.type === 'performance');
  const toneIntensity = Number(mark?.attrs?.toneIntensity);
  const enterSpeed = Number(mark?.attrs?.enterSpeed);
  return {
    effect: normalizePerformanceEffect(mark?.attrs?.effect) || undefined,
    scale: mark?.attrs?.scale as PerformanceScale | undefined,
    toneIntensity: Number.isFinite(toneIntensity)
      ? toneIntensity
      : (mark?.attrs?.scale === 'shout' ? 3 : mark?.attrs?.scale === 'whisper' ? -3 : undefined),
    enterMode: mark?.attrs?.enterMode as PerformanceEnterMode | undefined,
    enterSpeed: Number.isFinite(enterSpeed) ? enterSpeed : undefined,
  };
};

export const hasPerformanceContent = (json: unknown): boolean => {
  if (typeof json === 'string') {
    try {
      return hasPerformanceContent(JSON.parse(json));
    } catch {
      return false;
    }
  }
  if (!json || typeof json !== 'object') {
    return false;
  }

  const node = json as TipTapNode;
  if (node.type === 'performanceCommand') {
    return true;
  }
  if (Array.isArray(node.marks) && node.marks.some((mark) => mark?.type === 'performance')) {
    return true;
  }
  return Array.isArray(node.content) && node.content.some((child) => hasPerformanceContent(child));
};

export const parsePerformanceInstructions = (input: TipTapNode | string): PerformanceInstruction[] => {
  const root = typeof input === 'string' ? JSON.parse(input) as TipTapNode : input;
  const instructions: PerformanceInstruction[] = [];

  const visit = (node: TipTapNode | undefined) => {
    if (!node || typeof node !== 'object') {
      return;
    }

    if (node.type === 'text' && typeof node.text === 'string') {
      const effects = readPerformanceAttrs(node.marks);
      Array.from(node.text).forEach((char) => {
        instructions.push({
          type: 'char',
          char,
          effects,
          marks: node.marks,
          index: instructions.length,
        });
      });
      return;
    }

    if (node.type === 'hardBreak') {
      instructions.push({
        type: 'break',
        index: instructions.length,
      });
      return;
    }

    if (node.type === MESSAGE_ACTION_NODE_TYPE) {
      const label = normalizeMessageActionAttrs(node.attrs)?.label || '';
      Array.from(label).forEach((char) => {
        instructions.push({
          type: 'char',
          char,
          effects: {},
          index: instructions.length,
        });
      });
      return;
    }

    if (node.type === 'performanceCommand') {
      const rawValue = node.attrs?.value;
      const numericValue = rawValue == null || rawValue === '' ? undefined : Number(rawValue);
      const rawCommand = String(node.attrs?.command || 'delay').trim();
      const command = rawCommand === 'pause' ? 'pause' : 'delay';
      instructions.push({
        type: 'command',
        command: command as PerformanceCommandType,
        value: Number.isFinite(numericValue) ? numericValue : undefined,
        index: instructions.length,
      });
      return;
    }

    if (Array.isArray(node.content)) {
      node.content.forEach(visit);
    }
  };

  visit(root);
  return instructions;
};
