type TiptapCoreModule = typeof import('@tiptap/core');

export const PERFORMANCE_EFFECTS = [
  'shake',
  'wave',
  'rainbow',
  'glitch',
  'blink',
  'glow',
  'pulse',
  'float',
  'sway',
  'heartbeat',
  'wobble',
] as const;
export const PERFORMANCE_ENTER_MODES = [
  'normal',
  'blur',
  'typewriter',
  'fade',
  'rise',
  'drop',
  'zoom',
  'tracking',
  'flash',
  'glitch',
  'slam',
] as const;
const PERFORMANCE_SCALES = ['shout', 'whisper'] as const;

export type PerformanceEffect = typeof PERFORMANCE_EFFECTS[number];
export type PerformanceEnterMode = typeof PERFORMANCE_ENTER_MODES[number];
export type PerformanceScale = typeof PERFORMANCE_SCALES[number];

export interface PerformanceMarkAttrs {
  effect?: PerformanceEffect | null;
  enterMode?: PerformanceEnterMode | null;
  enterSpeed?: number | null;
  toneIntensity?: number | null;
  scale?: PerformanceScale | null;
}

const pickAllowed = <T extends string>(allowed: readonly T[], value: unknown): T | null => {
  const raw = String(value || '').trim();
  return (allowed as readonly string[]).includes(raw) ? raw as T : null;
};

export const normalizePerformanceEffect = (value: unknown): PerformanceEffect | null => {
  if (String(value || '').trim() === 'blur-in') {
    return 'blink';
  }
  return pickAllowed(PERFORMANCE_EFFECTS, value);
};

export const normalizePerformanceEnterMode = (value: unknown): PerformanceEnterMode | null => (
  pickAllowed(PERFORMANCE_ENTER_MODES, value)
);

export const normalizePerformanceScale = (value: unknown): PerformanceScale | null => (
  pickAllowed(PERFORMANCE_SCALES, value)
);

// 除 normal 外的出现效果都走逐字播放节奏；CSS 只负责单个字符怎么动。
export const isAnimatedPerformanceEnterMode = (mode?: PerformanceEnterMode | null) => !!mode && mode !== 'normal';

// 进入动画元素统一带 performance-enter，供 reduced-motion 等场景整体关闭进入动作。
export const resolvePerformanceEnterClassNames = (value: unknown): string[] => {
  const mode = normalizePerformanceEnterMode(value);
  if (!mode) {
    return [];
  }
  return isAnimatedPerformanceEnterMode(mode) ? ['performance-enter', `enter-${mode}`] : [`enter-${mode}`];
};

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    performance: {
      setPerformance: (attrs: PerformanceMarkAttrs) => ReturnType;
      unsetPerformance: () => ReturnType;
    };
  }
}

export const createPerformanceExtension = ({
  Mark,
  mergeAttributes,
}: Pick<TiptapCoreModule, 'Mark' | 'mergeAttributes'>) => Mark.create({
  name: 'performance',

  addAttributes() {
    return {
      effect: {
        default: null,
        parseHTML: (element: HTMLElement) => normalizePerformanceEffect(element.getAttribute('data-performance-effect')),
        renderHTML: (attributes: PerformanceMarkAttrs) => {
          const effect = normalizePerformanceEffect(attributes.effect);
          return effect ? { 'data-performance-effect': effect } : {};
        },
      },
      enterMode: {
        default: null,
        parseHTML: (element: HTMLElement) => normalizePerformanceEnterMode(element.getAttribute('data-performance-enter-mode')),
        renderHTML: (attributes: PerformanceMarkAttrs) => {
          const mode = normalizePerformanceEnterMode(attributes.enterMode);
          return mode ? { 'data-performance-enter-mode': mode } : {};
        },
      },
      enterSpeed: {
        default: null,
        parseHTML: (element: HTMLElement) => {
          const raw = element.getAttribute('data-performance-enter-speed');
          const numeric = raw == null || raw === '' ? NaN : Number(raw);
          return Number.isFinite(numeric) ? numeric : null;
        },
        renderHTML: (attributes: PerformanceMarkAttrs) => {
          const numeric = Number(attributes.enterSpeed);
          return Number.isFinite(numeric) ? { 'data-performance-enter-speed': String(numeric) } : {};
        },
      },
      toneIntensity: {
        default: null,
        parseHTML: (element: HTMLElement) => {
          const raw = element.getAttribute('data-performance-tone-intensity');
          const numeric = raw == null || raw === '' ? NaN : Number(raw);
          return Number.isFinite(numeric) ? numeric : null;
        },
        renderHTML: (attributes: PerformanceMarkAttrs) => {
          const numeric = Number(attributes.toneIntensity);
          return Number.isFinite(numeric) ? { 'data-performance-tone-intensity': String(numeric) } : {};
        },
      },
      scale: {
        default: null,
        parseHTML: (element: HTMLElement) => normalizePerformanceScale(element.getAttribute('data-performance-scale')),
        renderHTML: (attributes: PerformanceMarkAttrs) => {
          const scale = normalizePerformanceScale(attributes.scale);
          return scale ? { 'data-performance-scale': scale } : {};
        },
      },
    };
  },

  parseHTML() {
    return [
      { tag: 'span[data-performance-effect]' },
      { tag: 'span[data-performance-enter-mode]' },
      { tag: 'span[data-performance-enter-speed]' },
      { tag: 'span[data-performance-tone-intensity]' },
      { tag: 'span[data-performance-scale]' },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    const effect = normalizePerformanceEffect(HTMLAttributes.effect);
    const enterSpeed = Number(HTMLAttributes.enterSpeed);
    const toneIntensity = Number(HTMLAttributes.toneIntensity);
    const scale = normalizePerformanceScale(HTMLAttributes.scale);
    const classNames = ['tiptap-performance'];
    const styleVars: Record<string, string> = {};
    if (effect) {
      classNames.push(`fx-${effect}`);
    }
    classNames.push(...resolvePerformanceEnterClassNames(HTMLAttributes.enterMode));
    if (Number.isFinite(enterSpeed)) {
      styleVars['--performance-enter-speed'] = String(enterSpeed);
    }
    if (Number.isFinite(toneIntensity)) {
      styleVars['--performance-tone-intensity'] = String(toneIntensity);
    }
    if (scale) {
      classNames.push(`scale-${scale}`);
    }
    return [
      'span',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, {
        class: classNames.join(' '),
        style: Object.entries(styleVars).map(([key, value]) => `${key}: ${value}`).join('; '),
      }),
      0,
    ];
  },

  addCommands() {
    return {
      setPerformance:
        (attrs: PerformanceMarkAttrs) =>
        ({ commands }) => {
          const nextAttrs = {
            effect: normalizePerformanceEffect(attrs.effect),
            enterMode: normalizePerformanceEnterMode(attrs.enterMode),
            enterSpeed: Number.isFinite(Number(attrs.enterSpeed)) ? Number(attrs.enterSpeed) : null,
            toneIntensity: Number.isFinite(Number(attrs.toneIntensity)) ? Number(attrs.toneIntensity) : null,
            scale: normalizePerformanceScale(attrs.scale),
          };
          return commands.setMark(this.name, nextAttrs);
        },
      unsetPerformance:
        () =>
        ({ commands }) =>
          commands.unsetMark(this.name),
    };
  },
});
