import { defineAsyncComponent, type Component } from 'vue';
import type { InternalSurfaceType } from '@/utils/internalSurfaceLink';

const registry: Partial<Record<InternalSurfaceType, Component>> = {
  iform: defineAsyncComponent(() => import('./surfaces/IFormInternalSurface.vue')),
  note: defineAsyncComponent(() => import('./surfaces/StickyNoteInternalSurface.vue')),
  character: defineAsyncComponent(() => import('./surfaces/CharacterInternalSurface.vue')),
  'clue-board': defineAsyncComponent(() => import('./surfaces/ClueBoardInternalSurface.vue')),
};

export const getInternalSurfaceComponent = (type: string): Component | null => (
  registry[type as InternalSurfaceType] || null
);
