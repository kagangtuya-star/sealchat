import { computed, ref, shallowRef } from 'vue'
import {
  theaterPresentationSchema,
  type TheaterPresentation,
  type TheaterPresentationPatch,
  type WorldTheaterPresentationTemplate,
} from '@/types/theaterPresentation'
import {
  buildTheaterPresentationPatch,
  captureTheaterEditorSnapshot,
  commitTheaterEditorTransaction,
  createTheaterPresentationEditorState,
  dispatchTheaterEditorCommand,
  redoTheaterEditor,
  undoTheaterEditor,
  type TheaterEditorCommand,
  type TheaterEditorSnapshot,
} from '@/components/theater-presentation/theaterPresentationEditorState'

export const useTheaterPresentationEditor = (input: {
  mode: 'base' | 'variant'
  presentation?: TheaterPresentation | null
  base?: TheaterPresentation | null
  patch?: TheaterPresentationPatch | null
  worldTemplate?: WorldTheaterPresentationTemplate | null
}) => {
  const state = shallowRef(createTheaterPresentationEditorState(input))
  const transaction = ref<TheaterEditorSnapshot | null>(null)

  const dispatch = (command: TheaterEditorCommand, options: { transient?: boolean } = {}) => {
    state.value = dispatchTheaterEditorCommand(state.value, command, {
      recordHistory: !options.transient && command.type !== 'select',
      historySnapshot: options.transient ? undefined : transaction.value || undefined,
    })
    if (!options.transient) transaction.value = null
  }
  const beginTransaction = () => {
    if (!transaction.value) transaction.value = captureTheaterEditorSnapshot(state.value)
  }
  const commitTransaction = () => {
    const snapshot = transaction.value
    transaction.value = null
    if (!snapshot) return
    state.value = commitTheaterEditorTransaction(state.value, snapshot)
  }
  const undo = () => { state.value = undoTheaterEditor(state.value) }
  const redo = () => { state.value = redoTheaterEditor(state.value) }

  return {
    state,
    draft: computed(() => state.value.draft),
    selection: computed(() => state.value.selection),
    revision: computed(() => state.value.revision),
    history: computed(() => state.value.history),
    sectionModes: computed(() => state.value.sectionModes),
    result: computed<TheaterPresentation | TheaterPresentationPatch>(() => (
      state.value.mode === 'variant'
        ? buildTheaterPresentationPatch(state.value)
        : theaterPresentationSchema.parse(state.value.draft)
    )),
    dispatch,
    beginTransaction,
    commitTransaction,
    undo,
    redo,
  }
}
