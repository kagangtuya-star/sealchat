import type { PerformanceInstruction } from '@/utils/tiptap-performance-parser';
import type { PerformanceEffect, PerformanceEnterMode, PerformanceScale } from '@/utils/tiptap-performance-mark';
import type { PerformanceCommandType } from '@/utils/tiptap-performance-node';

export type TwinLayerPlaybackChar = {
  char: string;
  effects: {
    effect?: PerformanceEffect;
    enterMode?: PerformanceEnterMode;
    enterSpeed?: number;
    scale?: PerformanceScale;
    toneIntensity?: number;
  };
  marks?: Array<{ type?: string; attrs?: Record<string, any> }>;
  index: number;
};

type TwinLayerPlaybackOptions = {
  charactersPerSecond?: number;
  onChar?: (entry: TwinLayerPlaybackChar) => void;
  onInstantText?: (entries: TwinLayerPlaybackChar[]) => void;
  onBreak?: () => void;
  onStateChange?: () => void;
};

type PlaybackState = 'idle' | 'playing' | 'waiting' | 'completed' | 'cancelled';

const wait = (ms: number) => new Promise<void>((resolve) => {
  setTimeout(resolve, Math.max(0, ms));
});

const isTruthyNumber = (value: unknown) => Number.isFinite(Number(value)) && Number(value) > 0;
const isAnimatedEnterMode = (mode?: PerformanceEnterMode) => mode === 'blur' || mode === 'typewriter';
const isImmediateEnterMode = (mode?: PerformanceEnterMode) => !mode || mode === 'normal';
export const resolveCharactersPerSecondDelay = (charactersPerSecond?: number) => (
  Number.isFinite(charactersPerSecond) && Number(charactersPerSecond) > 0
    ? 1_000 / Math.min(60, Math.max(1, Number(charactersPerSecond)))
    : null
);
export const resolveEnterDelay = (mode?: PerformanceEnterMode, speed?: number) => {
  if (!Number.isFinite(Number(speed))) {
    return mode === 'typewriter' ? 120 : 60;
  }
  const normalized = Math.max(1, Math.min(9, Number(speed)));
  const baseDelay = Math.round(180 - normalized * 16);
  if (mode === 'typewriter') {
    return Math.round(baseDelay * 1.55 + 22);
  }
  return baseDelay;
};

const LIGHT_PUNCTUATION = new Set(['，', ',', '、', '；', ';', '：', ':']);
const HEAVY_PUNCTUATION = new Set(['。', '.', '．', '｡', '！', '!', '？', '?', '…']);

export const resolvePunctuationPauseExtra = (char: string, baseDelayMs: number) => {
  const normalizedDelay = Number.isFinite(baseDelayMs) ? Math.max(0, baseDelayMs) : 0;
  if (LIGHT_PUNCTUATION.has(char)) {
    return Math.round(Math.min(180, Math.max(40, normalizedDelay * 0.6)));
  }
  if (HEAVY_PUNCTUATION.has(char)) {
    return Math.round(Math.max(90, normalizedDelay * 3.0));
  }
  return 0;
};

const findNearestAnimatedContext = (instructions: PerformanceInstruction[], index: number) => {
  for (let cursor = index - 1; cursor >= 0; cursor -= 1) {
    const entry = instructions[cursor];
    if (entry.type === 'char' && isAnimatedEnterMode(entry.effects.enterMode)) {
      return entry as TwinLayerPlaybackChar;
    }
    if (entry.type === 'break') {
      continue;
    }
  }
  for (let cursor = index + 1; cursor < instructions.length; cursor += 1) {
    const entry = instructions[cursor];
    if (entry.type === 'char' && isAnimatedEnterMode(entry.effects.enterMode)) {
      return entry as TwinLayerPlaybackChar;
    }
    if (entry.type === 'break') {
      continue;
    }
    if (entry.type === 'command') {
      continue;
    }
  }
  return null;
};

export const createTwinLayerPlayback = (
  instructions: PerformanceInstruction[],
  options: TwinLayerPlaybackOptions = {},
) => {
  let characterDelay = resolveCharactersPerSecondDelay(options.charactersPerSecond);
  let visibleText = '';
  let state: PlaybackState = 'idle';
  let fastForward = false;
  let disposed = false;
  let waitingForClick = false;
  let continueResolver: (() => void) | null = null;
  let currentRun: Promise<void> | null = null;

  const notifyStateChange = () => {
    options.onStateChange?.();
  };

  const reset = () => {
    visibleText = '';
    waitingForClick = false;
    fastForward = false;
    disposed = false;
    state = 'idle';
    continueResolver = null;
    notifyStateChange();
  };

  const skip = () => {
    fastForward = true;
    waitingForClick = false;
    continueResolver?.();
    continueResolver = null;
    state = 'playing';
    notifyStateChange();
  };

  const dispose = () => {
    if (disposed) {
      return;
    }
    disposed = true;
    waitingForClick = false;
    continueResolver?.();
    continueResolver = null;
    state = 'cancelled';
    notifyStateChange();
  };

  const setCharactersPerSecond = (charactersPerSecond?: number) => {
    characterDelay = resolveCharactersPerSecondDelay(charactersPerSecond);
  };

  const continuePlayback = () => {
    if (waitingForClick && continueResolver) {
      waitingForClick = false;
      state = 'playing';
      continueResolver();
      continueResolver = null;
      notifyStateChange();
    }
  };

  const handleCommand = async (command: PerformanceCommandType, value?: number) => {
    switch (command) {
      case 'delay':
        if (disposed) {
          break;
        }
        if (!fastForward && isTruthyNumber(value)) {
          await wait(Number(value));
        }
        break;
      case 'pause':
        if (fastForward || disposed) {
          break;
        }
        waitingForClick = true;
        state = 'waiting';
        notifyStateChange();
        await new Promise<void>((resolve) => {
          continueResolver = resolve;
        });
        break;
    }
  };

  const appendInstantChars = (entries: TwinLayerPlaybackChar[]) => {
    entries.forEach((entry) => {
      visibleText += entry.char;
    });
    if (options.onInstantText) {
      options.onInstantText(entries);
      return;
    }
    entries.forEach((entry) => {
      options.onChar?.(entry);
    });
  };

  const play = async () => {
    if (currentRun) {
      return currentRun;
    }
    state = 'playing';
    notifyStateChange();

    currentRun = (async () => {
      for (const entry of instructions) {
        if (disposed) {
          break;
        }
        if (entry.type === 'char') {
          const mode = entry.effects.enterMode;
          if (isImmediateEnterMode(mode) && characterDelay === null) {
            appendInstantChars([entry as TwinLayerPlaybackChar]);
            continue;
          }
          visibleText += entry.char;
          options.onChar?.(entry as TwinLayerPlaybackChar);
          if (!fastForward) {
            const baseDelay = characterDelay ?? resolveEnterDelay(mode, entry.effects.enterSpeed);
            const extraDelay = resolvePunctuationPauseExtra(entry.char, baseDelay);
            await wait(baseDelay + extraDelay);
          }
          continue;
        }
        if (entry.type === 'break') {
          if (disposed) {
            break;
          }
          visibleText += '\n';
          options.onBreak?.();
          continue;
        }
        const animatedContext = findNearestAnimatedContext(instructions, entry.index);
        if (!animatedContext) {
          continue;
        }
        await handleCommand(entry.command, entry.value);
      }
      if (!disposed) {
        state = waitingForClick ? 'waiting' : 'completed';
      }
      notifyStateChange();
    })();

    try {
      await currentRun;
    } finally {
      currentRun = null;
    }
  };

  return {
    play,
    skip,
    reset,
    dispose,
    setCharactersPerSecond,
    continuePlayback,
    isWaiting: () => waitingForClick,
    getVisibleText: () => visibleText,
    getState: () => state,
  };
};
