import { computed, onBeforeUnmount, readonly, shallowRef, watch } from 'vue';
import { api } from '@/stores/_config';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { GlassBackgroundSettings } from './useGlassBackground';

export type WorldGlassAppearance = Omit<GlassBackgroundSettings, 'enabled' | 'attachmentId'>;
export interface WorldGlassPreset {
  id: string; worldId: string; name: string; attachmentId: string;
  settings: WorldGlassAppearance; sortOrder: number;
}
export interface WorldGlassTrigger {
  id: string; worldId: string; presetId: string; triggerType: 'channel';
  triggerKey: string; enabled: boolean; sortOrder: number;
}
export interface WorldGlassState {
  worldId: string; enabled: boolean; revision: number; activePresetId: string;
}
export interface WorldGlassEffectiveState extends WorldGlassState {
  channelId: string; source: 'default' | 'channel' | 'none'; effectivePresetId: string;
  preset: WorldGlassPreset | null; matchedTrigger: Pick<WorldGlassTrigger, 'id' | 'triggerType'> | null; canManage: boolean;
}
export interface WorldGlassManagement {
  presets: WorldGlassPreset[]; triggers: WorldGlassTrigger[]; state: WorldGlassState; canManage: boolean;
}

const state = shallowRef<WorldGlassEffectiveState | null>(null);
const loadError = shallowRef('');
let context = { worldId: '', channelId: '' };
let generation = 0;
let wantedRevision = 0;
type WorldGlassContext = { worldId: string; channelId: string };
const externalContext = shallowRef<WorldGlassContext | null>(null);

export const worldGlassState = readonly(state);
export const worldGlassLoadError = readonly(loadError);
export const worldGlassURL = (worldId: string) => `api/v1/worlds/${encodeURIComponent(worldId)}`;

export function setWorldGlassExternalContext(worldId: string, channelId: string) {
  externalContext.value = {
    worldId: String(worldId || '').trim(),
    channelId: String(channelId || '').trim(),
  };
}

export function clearWorldGlassExternalContext() {
  externalContext.value = null;
}

export async function loadWorldGlassEffective() {
  const token = ++generation;
  const { worldId, channelId } = context;
  if (!worldId) { state.value = null; return; }
  try {
    const { data } = await api.get<WorldGlassEffectiveState>(`${worldGlassURL(worldId)}/glass-background`, { params: { channelId } });
    if (token !== generation) return;
    if (data.worldId !== worldId || data.channelId !== channelId || data.revision < wantedRevision) return;
    state.value = data;
    loadError.value = '';
  } catch {
    if (token !== generation) return;
    const current = state.value;
    if (!current || current.worldId !== worldId) {
      state.value = null;
    }
    loadError.value = current?.worldId === worldId
      ? '世界背景加载失败，已保留当前世界背景'
      : '世界背景加载失败，暂用个人设置';
  }
}

export function invalidateWorldGlass(worldId: string, revision: number) {
  if (worldId !== context.worldId || !Number.isFinite(revision) || revision <= (state.value?.revision ?? -1)) return;
  wantedRevision = Math.max(wantedRevision, revision);
  void loadWorldGlassEffective();
}

export function useWorldGlassBackgroundRuntime() {
  const chat = useChatStore();
  const runtimeContext = computed<WorldGlassContext>(() => externalContext.value ?? {
    worldId: chat.currentWorldId || '',
    channelId: chat.curChannel?.id || '',
  });
  const stop = watch(() => [runtimeContext.value.worldId, runtimeContext.value.channelId] as const, ([worldId, channelId]) => {
    const previousWorldId = context.worldId;
    const nextWorldId = String(worldId || '').trim();
    const nextChannelId = String(channelId || '').trim();
    const worldChanged = previousWorldId !== nextWorldId;

    context = { worldId: nextWorldId, channelId: nextChannelId };
    generation++;
    if (!nextWorldId) {
      state.value = null;
      wantedRevision = 0;
      loadError.value = '';
      return;
    }
    if (worldChanged) {
      state.value = null;
      wantedRevision = 0;
    }
    loadError.value = '';
    void loadWorldGlassEffective();
  }, { immediate: true, flush: 'sync' });
  const onUpdate = (event: { worldGlass?: { worldId: string; revision: number } }) => {
    if (!event.worldGlass) return;
    invalidateWorldGlass(event.worldGlass.worldId, event.worldGlass.revision);
    if (typeof window !== 'undefined' && window.parent !== window) {
      window.parent.postMessage({
        type: 'sealchat.embed.worldGlassInvalidated',
        worldId: event.worldGlass.worldId,
        revision: event.worldGlass.revision,
      }, window.location.origin);
    }
  };
  const onConnected = () => { void loadWorldGlassEffective(); };
  chatEvent.on('world-glass-background-updated', onUpdate);
  chatEvent.on('connected', onConnected);
  onBeforeUnmount(() => {
    stop(); generation++; context = { worldId: '', channelId: '' }; state.value = null;
    chatEvent.off('world-glass-background-updated', onUpdate);
    chatEvent.off('connected', onConnected);
  });
}
