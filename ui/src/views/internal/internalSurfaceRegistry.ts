import { defineAsyncComponent, type Component } from 'vue';
import type { InternalSurfaceType } from '@/utils/internalSurfaceLink';

const registry: Record<InternalSurfaceType, Component> = {
  iform: defineAsyncComponent(() => import('./surfaces/IFormInternalSurface.vue')),
  note: defineAsyncComponent(() => import('./surfaces/StickyNoteInternalSurface.vue')),
  character: defineAsyncComponent(() => import('./surfaces/CharacterInternalSurface.vue')),
};

export const getInternalSurfaceComponent = (type: string): Component | null => (
  registry[type as InternalSurfaceType] || null
);
