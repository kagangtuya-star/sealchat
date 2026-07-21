import {
  MAX_THEATER_PORTRAIT_DECORATIONS,
  createDefaultTheaterDialogueStyle,
  createDefaultTheaterNarrationStyle,
  createDefaultTheaterPresentation,
  applyWorldTheaterPresentationTemplate,
  normalizeTheaterPresentation,
  normalizeTheaterTextTransform,
  normalizeTheaterTransform,
  resolveTheaterPresentation,
  theaterPresentationSchema,
  type TheaterDialogueStyle,
  type TheaterMediaRef,
  type TheaterPresentation,
  type TheaterPresentationPatch,
  type TheaterTransform,
  type TheaterVisualLayer,
  type WorldTheaterPresentationTemplate,
} from '@/types/theaterPresentation'

export type TheaterSection = 'portrait' | 'speaker' | 'content' | 'decorations' | 'dialogue' | 'narration'
export type TheaterSectionMode = 'inherit' | 'custom' | 'clear'
export type TheaterSelection =
  | { kind: 'portrait' }
  | { kind: 'speaker' }
  | { kind: 'content' }
  | { kind: 'decoration'; id: string }
  | { kind: 'dialogue' }
  | { kind: 'dialogue-frame' }

export type TheaterEditorCommand =
  | { type: 'select'; target: TheaterSelection }
  | { type: 'set-transform'; target: TheaterSelection; transform: Partial<TheaterTransform> }
  | { type: 'set-media'; target: TheaterSelection; media: TheaterMediaRef | null }
  | { type: 'set-layer-property'; target: TheaterSelection; property: 'enabled' | 'fit' | 'blendMode' | 'playbackRate' | 'fontScale'; value: boolean | string | number }
  | { type: 'add-decoration'; layer: TheaterVisualLayer }
  | { type: 'remove-decoration'; id: string }
  | { type: 'reorder-decoration'; id: string; beforeId: string | null }
  | { type: 'set-dialogue-padding'; padding: Partial<TheaterDialogueStyle['padding']> }
  | { type: 'set-dialogue-property'; property: 'nameGap' | 'textAlign' | 'contentColor' | 'charactersPerSecond'; value: number | string }
  | { type: 'set-narration-property'; property: 'enabled' | 'backdropColor' | 'backdropOpacity'; value: boolean | string | number }
  | { type: 'reset-section'; section: TheaterSection }
  | { type: 'set-section-mode'; section: TheaterSection; mode: TheaterSectionMode }

export interface TheaterEditorSnapshot {
  draft: TheaterPresentation
  selection: TheaterSelection
  sectionModes: Record<TheaterSection, TheaterSectionMode>
}

export interface TheaterEditorHistory {
  past: TheaterEditorSnapshot[]
  future: TheaterEditorSnapshot[]
}

export interface TheaterEditorState extends TheaterEditorSnapshot {
  mode: 'base' | 'variant'
  base: TheaterPresentation
  worldTemplate: WorldTheaterPresentationTemplate
  revision: number
  history: TheaterEditorHistory
}

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

const sectionForSelection = (selection: TheaterSelection): TheaterSection => {
  if (selection.kind === 'portrait') return 'portrait'
  if (selection.kind === 'speaker') return 'speaker'
  if (selection.kind === 'content') return 'content'
  if (selection.kind === 'decoration') return 'decorations'
  return 'dialogue'
}

const inferSectionMode = (patch: TheaterPresentationPatch | null | undefined, section: TheaterSection): TheaterSectionMode => {
  if (!patch) return 'inherit'
  if (section === 'speaker' || section === 'content') return inferSectionMode(patch, 'dialogue')
  const key = section === 'decorations' ? 'portraitDecorations' : section
  if (!(key in patch)) return 'inherit'
  return patch[key] === null ? 'clear' : 'custom'
}

export const createTheaterPresentationEditorState = (input: {
  mode: 'base' | 'variant'
  presentation?: TheaterPresentation | null
  base?: TheaterPresentation | null
  patch?: TheaterPresentationPatch | null
  worldTemplate?: WorldTheaterPresentationTemplate | null
}): TheaterEditorState => {
  const worldTemplate = clone(input.worldTemplate || {})
  const base = normalizeTheaterPresentation(input.base || input.presentation || applyWorldTheaterPresentationTemplate(createDefaultTheaterPresentation(), worldTemplate))
  const sectionModes = input.mode === 'variant'
    ? {
        portrait: inferSectionMode(input.patch, 'portrait'),
        speaker: inferSectionMode(input.patch, 'speaker'),
        content: inferSectionMode(input.patch, 'content'),
        decorations: inferSectionMode(input.patch, 'decorations'),
        dialogue: inferSectionMode(input.patch, 'dialogue'),
        narration: inferSectionMode(input.patch, 'narration'),
      }
    : { portrait: 'custom', speaker: 'custom', content: 'custom', decorations: 'custom', dialogue: 'custom', narration: 'custom' } as const
  return {
    mode: input.mode,
    base: clone(base),
    worldTemplate,
    draft: input.mode === 'variant' ? resolveTheaterPresentation(base, input.patch) : clone(base),
    selection: { kind: 'portrait' },
    sectionModes: { ...sectionModes },
    revision: 0,
    history: { past: [], future: [] },
  }
}

export const captureTheaterEditorSnapshot = (state: TheaterEditorState): TheaterEditorSnapshot => ({
  draft: clone(state.draft),
  selection: clone(state.selection),
  sectionModes: { ...state.sectionModes },
})

const findLayer = (draft: TheaterPresentation, target: TheaterSelection): TheaterVisualLayer | null => {
  if (target.kind === 'portrait') return draft.portrait
  if (target.kind === 'decoration') return draft.portraitDecorations.find((item) => item.id === target.id) || null
  if (target.kind === 'dialogue-frame') return draft.dialogue.frame
  return null
}

const replaceLayer = (
  draft: TheaterPresentation,
  target: TheaterSelection,
  replace: (layer: TheaterVisualLayer | null) => TheaterVisualLayer | null,
) => {
  if (target.kind === 'portrait') draft.portrait = replace(draft.portrait)
  if (target.kind === 'decoration') {
    draft.portraitDecorations = draft.portraitDecorations.map((item) => item.id === target.id ? replace(item) || item : item)
  }
  if (target.kind === 'dialogue-frame') draft.dialogue.frame = replace(draft.dialogue.frame)
}

const markCustom = (state: TheaterEditorState, section: TheaterSection) => {
  if (state.mode === 'variant') state.sectionModes[section] = 'custom'
}

const applyCommand = (state: TheaterEditorState, command: TheaterEditorCommand): boolean => {
  if (command.type === 'select') {
    state.selection = clone(command.target)
    return true
  }
  if (command.type === 'set-section-mode') {
    if (state.mode !== 'variant' || state.sectionModes[command.section] === command.mode) return false
    state.sectionModes[command.section] = command.mode
    if (command.mode === 'inherit' || command.mode === 'clear') {
      if (command.section === 'portrait') state.draft.portrait = command.mode === 'inherit' ? clone(state.base.portrait) : null
      if (command.section === 'speaker') state.draft.dialogue.speaker = command.mode === 'inherit'
        ? clone(state.base.dialogue.speaker)
        : { ...clone(createDefaultTheaterDialogueStyle().speaker), enabled: command.mode !== 'clear' }
      if (command.section === 'content') state.draft.dialogue.content = command.mode === 'inherit'
        ? clone(state.base.dialogue.content)
        : { ...clone(createDefaultTheaterDialogueStyle().content), enabled: command.mode !== 'clear' }
      if (command.section === 'decorations') state.draft.portraitDecorations = command.mode === 'inherit' ? clone(state.base.portraitDecorations) : []
      if (command.section === 'dialogue') state.draft.dialogue = command.mode === 'inherit' ? clone(state.base.dialogue) : createDefaultTheaterDialogueStyle()
      if (command.section === 'narration') state.draft.narration = command.mode === 'inherit' ? clone(state.base.narration) : createDefaultTheaterNarrationStyle()
    }
    return true
  }
  if (command.type === 'set-transform') {
    if (command.target.kind === 'speaker') {
      state.draft.dialogue.speaker.transform = normalizeTheaterTextTransform(command.transform, state.draft.dialogue.speaker.transform)
    } else if (command.target.kind === 'content') {
      state.draft.dialogue.content.transform = normalizeTheaterTextTransform(command.transform, state.draft.dialogue.content.transform)
    } else if (command.target.kind === 'dialogue') {
      state.draft.dialogue.transform = normalizeTheaterTransform(command.transform, state.draft.dialogue.transform)
    } else {
      const layer = findLayer(state.draft, command.target)
      if (!layer) return false
      replaceLayer(state.draft, command.target, (current) => current ? ({ ...current, transform: normalizeTheaterTransform(command.transform, current.transform) }) : null)
    }
    markCustom(state, sectionForSelection(command.target))
    return true
  }
  if (command.type === 'set-media') {
    const current = findLayer(state.draft, command.target)
    if (!current && command.media === null) return false
    if (!current && command.media) {
      const space = command.target.kind === 'portrait' ? 'viewport' : command.target.kind === 'dialogue-frame' ? 'dialogue' : 'portrait'
      const layer = createTheaterVisualLayer(command.media, space, command.target.kind === 'decoration' ? command.target.id : command.target.kind)
      const style = command.target.kind === 'portrait'
        ? state.worldTemplate.portrait
        : null
      if (style) {
        layer.enabled = style.enabled
        layer.transform = clone(style.transform)
        layer.fit = style.fit
        layer.playbackRate = style.playbackRate
        layer.blendMode = style.blendMode
      }
      replaceLayer(state.draft, command.target, () => layer)
    } else {
      replaceLayer(state.draft, command.target, (layer) => command.media && layer ? { ...layer, media: clone(command.media) } : null)
    }
    markCustom(state, sectionForSelection(command.target))
    return true
  }
  if (command.type === 'set-layer-property') {
    if (command.target.kind === 'speaker' || command.target.kind === 'content') {
      if (command.property === 'enabled') {
        state.draft.dialogue[command.target.kind].enabled = Boolean(command.value)
      } else if (command.property === 'fontScale' && typeof command.value === 'number') {
        state.draft.dialogue[command.target.kind].fontScale = command.value
      } else {
        return false
      }
      markCustom(state, command.target.kind)
      markCustom(state, 'dialogue')
      return true
    }
    const layer = findLayer(state.draft, command.target)
    if (!layer) return false
    replaceLayer(state.draft, command.target, (current) => current ? ({ ...current, [command.property]: command.value }) as TheaterVisualLayer : null)
    markCustom(state, sectionForSelection(command.target))
    return true
  }
  if (command.type === 'add-decoration') {
    if (state.draft.portraitDecorations.length >= MAX_THEATER_PORTRAIT_DECORATIONS) return false
    if (state.draft.portraitDecorations.some((item) => item.id === command.layer.id)) return false
    state.draft.portraitDecorations.push(clone(command.layer))
    state.selection = { kind: 'decoration', id: command.layer.id }
    markCustom(state, 'decorations')
    return true
  }
  if (command.type === 'remove-decoration') {
    const next = state.draft.portraitDecorations.filter((item) => item.id !== command.id)
    if (next.length === state.draft.portraitDecorations.length) return false
    state.draft.portraitDecorations = next
    if (state.selection.kind === 'decoration' && state.selection.id === command.id) state.selection = { kind: 'portrait' }
    markCustom(state, 'decorations')
    return true
  }
  if (command.type === 'reorder-decoration') {
    const index = state.draft.portraitDecorations.findIndex((item) => item.id === command.id)
    if (index < 0) return false
    const [layer] = state.draft.portraitDecorations.splice(index, 1)
    const beforeIndex = command.beforeId ? state.draft.portraitDecorations.findIndex((item) => item.id === command.beforeId) : -1
    state.draft.portraitDecorations.splice(beforeIndex < 0 ? state.draft.portraitDecorations.length : beforeIndex, 0, layer)
    state.draft.portraitDecorations.forEach((item, order) => { item.transform.zIndex = order })
    markCustom(state, 'decorations')
    return true
  }
  if (command.type === 'set-dialogue-padding') {
    state.draft.dialogue.padding = { ...state.draft.dialogue.padding, ...command.padding }
    markCustom(state, 'dialogue')
    return true
  }
  if (command.type === 'set-dialogue-property') {
    state.draft.dialogue = { ...state.draft.dialogue, [command.property]: command.value } as TheaterDialogueStyle
    markCustom(state, 'dialogue')
    return true
  }
  if (command.type === 'set-narration-property') {
    state.draft.narration = { ...state.draft.narration, [command.property]: command.value }
    markCustom(state, 'narration')
    return true
  }
  if (command.type === 'reset-section') {
    const defaults = applyWorldTheaterPresentationTemplate(createDefaultTheaterPresentation(), state.worldTemplate)
    if (command.section === 'portrait') {
      if (!state.worldTemplate.portrait || !state.draft.portrait) {
        state.draft.portrait = null
      } else {
        const style = state.worldTemplate.portrait
        state.draft.portrait = {
          ...state.draft.portrait,
          enabled: style.enabled,
          transform: clone(style.transform),
          fit: style.fit,
          playbackRate: style.playbackRate,
          blendMode: style.blendMode,
        }
      }
    }
    if (command.section === 'speaker') state.draft.dialogue.speaker = clone(defaults.dialogue.speaker)
    if (command.section === 'content') state.draft.dialogue.content = clone(defaults.dialogue.content)
    if (command.section === 'decorations') state.draft.portraitDecorations = []
    if (command.section === 'dialogue') {
      const speaker = state.draft.dialogue.speaker
      const content = state.draft.dialogue.content
      state.draft.dialogue = clone(defaults.dialogue)
      state.draft.dialogue.speaker = speaker
      state.draft.dialogue.content = content
    }
    if (command.section === 'narration') state.draft.narration = createDefaultTheaterNarrationStyle()
    markCustom(state, command.section)
    return true
  }
  return false
}

export const dispatchTheaterEditorCommand = (
  state: TheaterEditorState,
  command: TheaterEditorCommand,
  options: { recordHistory?: boolean; historySnapshot?: TheaterEditorSnapshot } = {},
): TheaterEditorState => {
  const next = clone(state)
  const previous = options.historySnapshot ? clone(options.historySnapshot) : captureTheaterEditorSnapshot(state)
  if (!applyCommand(next, command)) return state
  const parsed = theaterPresentationSchema.safeParse(next.draft)
  if (!parsed.success) return state
  next.draft = parsed.data
  next.revision += 1
  if (options.recordHistory !== false) {
    next.history.past.push(previous)
    next.history.future = []
  }
  return next
}

export const undoTheaterEditor = (state: TheaterEditorState): TheaterEditorState => {
  const previous = state.history.past.at(-1)
  if (!previous) return state
  const current = captureTheaterEditorSnapshot(state)
  return {
    ...clone(state),
    ...clone(previous),
    revision: state.revision + 1,
    history: { past: state.history.past.slice(0, -1), future: [current, ...state.history.future] },
  }
}

export const redoTheaterEditor = (state: TheaterEditorState): TheaterEditorState => {
  const nextSnapshot = state.history.future[0]
  if (!nextSnapshot) return state
  const current = captureTheaterEditorSnapshot(state)
  return {
    ...clone(state),
    ...clone(nextSnapshot),
    revision: state.revision + 1,
    history: { past: [...state.history.past, current], future: state.history.future.slice(1) },
  }
}

export const commitTheaterEditorTransaction = (
  state: TheaterEditorState,
  snapshot: TheaterEditorSnapshot,
): TheaterEditorState => {
  const current = captureTheaterEditorSnapshot(state)
  if (JSON.stringify(snapshot) === JSON.stringify(current)) return state
  return {
    ...state,
    history: { past: [...state.history.past, clone(snapshot)], future: [] },
  }
}

export const buildTheaterPresentationPatch = (state: TheaterEditorState): TheaterPresentationPatch => {
  const patch: TheaterPresentationPatch = {}
  if (state.sectionModes.portrait === 'custom') patch.portrait = clone(state.draft.portrait)
  if (state.sectionModes.portrait === 'clear') patch.portrait = null
  if (state.sectionModes.decorations === 'custom') patch.portraitDecorations = clone(state.draft.portraitDecorations)
  if (state.sectionModes.decorations === 'clear') patch.portraitDecorations = null
  const dialogueClear = state.sectionModes.dialogue === 'clear'
    && state.sectionModes.speaker === 'clear'
    && state.sectionModes.content === 'clear'
  const dialogueCustom = !dialogueClear && (
    state.sectionModes.dialogue === 'custom'
    || state.sectionModes.speaker !== 'inherit'
    || state.sectionModes.content !== 'inherit'
  )
  if (dialogueCustom) patch.dialogue = clone(state.draft.dialogue)
  if (dialogueClear) patch.dialogue = null
  if (state.sectionModes.narration === 'custom') patch.narration = clone(state.draft.narration)
  if (state.sectionModes.narration === 'clear') patch.narration = null
  return patch
}

export const createTheaterVisualLayer = (
  media: TheaterMediaRef,
  space: TheaterVisualLayer['space'],
  id = `layer-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
): TheaterVisualLayer => ({
  id,
  enabled: true,
  media: clone(media),
  space,
  transform: space === 'viewport'
    ? { x: 0.13, y: 0.22, width: 0.27, height: 0.54, rotation: 0, opacity: 1, zIndex: 0 }
    : { x: 0, y: 0, width: 1, height: 1, rotation: 0, opacity: 1, zIndex: 0 },
  fit: 'cover',
  playbackRate: 1,
  blendMode: 'normal',
})
