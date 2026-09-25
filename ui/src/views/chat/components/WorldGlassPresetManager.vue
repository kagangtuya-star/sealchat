<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { NButton, NDropdown, NIcon, NInput, NSelect, NSwitch, useDialog, useMessage } from 'naive-ui';
import { Dots } from '@vicons/tabler';
import { api } from '@/stores/_config';
import { useChatStore } from '@/stores/chat';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { normalizeGlassBackgroundSettings, type GlassBackgroundSettings } from '@/composables/useGlassBackground';
import { worldGlassState, worldGlassLoadError, worldGlassURL, loadWorldGlassEffective, invalidateWorldGlass,
  type WorldGlassPreset, type WorldGlassManagement } from '@/composables/useWorldGlassBackground';
import GlassBackgroundEditor from './GlassBackgroundEditor.vue';
import type { GlassBackgroundChannelNode } from './glassBackgroundTypes';

const props = defineProps<{
  worldId?: string;
  channelId?: string;
  channelTree?: GlassBackgroundChannelNode[];
}>();
const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();
const GLASS_POPUP_Z_INDEX = 3300;
const glassSelectMenuProps = {
  class: 'sc-glass-popup-menu',
  style: { zIndex: GLASS_POPUP_Z_INDEX },
};
const glassDropdownMenuProps = () => ({
  class: 'sc-glass-popup-menu',
  style: { zIndex: GLASS_POPUP_Z_INDEX },
});
const worldId = computed(() => {
  if (props.worldId !== undefined) {
    return props.worldId.trim();
  }
  return chat.currentWorldId || '';
});
const effectiveChannelTree = computed<GlassBackgroundChannelNode[]>(() => {
  if (props.channelTree !== undefined) {
    return props.channelTree;
  }
  return chat.channelTreeByWorld[worldId.value] || [];
});
const canManage = computed(() => worldGlassState.value?.worldId === worldId.value && worldGlassState.value.canManage);
const management = ref<WorldGlassManagement | null>(null);
const busy = ref(false);
const uploading = ref(false);
const loading = ref(false);
const error = ref('');
const draft = ref<GlassBackgroundSettings | null>(null);
const draftId = ref('');
const draftName = ref('');
const rulePreset = ref<WorldGlassPreset | null>(null);
const channelIds = ref<string[]>([]);
const ruleEnabled = ref(true);
const channelLoading = ref(false);
const channelEnsurePromises = new Map<string, Promise<void>>();
let generation = 0;
let alive = true;
onBeforeUnmount(() => { alive = false; generation++; });

async function ensureWorldChannels() {
  const id = worldId.value;
  if (!id || props.channelTree !== undefined) return;

  const cached = chat.channelTreeByWorld[id];
  if (Array.isArray(cached) && chat.channelTreeReady[id] === true) {
    return;
  }

  const existing = channelEnsurePromises.get(id);
  if (existing) return existing;

  channelLoading.value = true;
  const promise = (async () => {
    try {
      await chat.channelList(id, false, {
        autoSwitch: false,
        preserveCurrentChannel: true,
        refreshUnread: false,
      });
    } catch {
      // Management loading errors should not prevent editing existing presets.
    } finally {
      channelEnsurePromises.delete(id);
      if (alive && worldId.value === id) channelLoading.value = false;
    }
  })();
  channelEnsurePromises.set(id, promise);
  return promise;
}

async function load() {
  const token = ++generation;
  const id = worldId.value;
  if (!id || !canManage.value) { management.value = null; return; }
  loading.value = true;
  try {
    const { data } = await api.get<WorldGlassManagement>(`${worldGlassURL(id)}/glass-presets`);
    if (token === generation && alive) {
      management.value = data;
      error.value = '';
      void ensureWorldChannels();
    }
  } catch { if (token === generation && alive) error.value = '预设列表加载失败'; }
  finally { if (token === generation && alive) loading.value = false; }
}
watch(worldId, () => { generation++; channelLoading.value = false; management.value = null; draft.value = null; rulePreset.value = null; error.value = ''; });
watch(() => [worldId.value, canManage.value, worldGlassState.value?.revision], () => { void load(); }, { immediate: true });

function edit(preset?: WorldGlassPreset) {
  rulePreset.value = null;
  draftId.value = preset?.id || '';
  draftName.value = preset?.name || '';
  draft.value = normalizeGlassBackgroundSettings({ ...preset?.settings, attachmentId: preset?.attachmentId || '', enabled: true });
}
function updateDraft(patch: Partial<GlassBackgroundSettings>) {
  if (draft.value && !busy.value) draft.value = normalizeGlassBackgroundSettings({ ...draft.value, ...patch });
}
async function run(operation: (id: string) => Promise<void>) {
  if (busy.value || !canManage.value) return;
  const id = worldId.value;
  busy.value = true;
  try {
    await operation(id);
    if (!alive || id !== worldId.value) return;
    await loadWorldGlassEffective();
    await load();
  } catch (e) {
    if (alive && id === worldId.value) {
      const response = (e as { response?: { data?: { message?: string } } }).response;
      message.error(response?.data?.message || '操作失败，请重试');
      await load();
    }
  } finally { if (alive) busy.value = false; }
}
function savePreset() {
  if (uploading.value || !draft.value || !draftName.value.trim() || !draft.value.attachmentId) return;
  const { enabled: _enabled, attachmentId, ...settings } = draft.value;
  const presetId = draftId.value;
  const body = { name: draftName.value.trim(), attachmentId, settings };
  void run(async id => {
    const url = `${worldGlassURL(id)}/glass-presets`;
    const { data } = presetId ? await api.patch(`${url}/${encodeURIComponent(presetId)}`, body) : await api.post(url, body);
    invalidateWorldGlass(id, data.revision);
    if (alive && id === worldId.value) draft.value = null;
  });
}
function activate(preset: WorldGlassPreset) {
  void run(async id => { const { data } = await api.post(`${worldGlassURL(id)}/glass-background/activate`, { presetId: preset.id }); invalidateWorldGlass(id, data.revision); });
}
function disable() {
  void run(async id => { const { data } = await api.post(`${worldGlassURL(id)}/glass-background/disable`); invalidateWorldGlass(id, data.revision); });
}
function remove(preset: WorldGlassPreset) {
  const id = worldId.value;
  dialog.warning({ title: '删除预设', content: `删除“${preset.name}”及其频道规则？图片附件会保留。`, positiveText: '删除', negativeText: '取消',
    onPositiveClick: () => {
      if (!alive || id !== worldId.value) return;
      return run(async current => { const { data } = await api.delete(`${worldGlassURL(current)}/glass-presets/${encodeURIComponent(preset.id)}`); invalidateWorldGlass(current, data.revision); });
    },
  });
}
function rules(preset: WorldGlassPreset) {
  void ensureWorldChannels();
  draft.value = null; rulePreset.value = preset;
  const triggers = management.value?.triggers.filter(t => t.presetId === preset.id) || [];
  channelIds.value = triggers.map(t => t.triggerKey);
  ruleEnabled.value = triggers.length === 0 || triggers.some(t => t.enabled);
}
const isWorldGlassTriggerChannel = (
  channel: GlassBackgroundChannelNode,
) => (
  Boolean(channel.id)
  && channel.isPrivate !== true
  && String(channel.permType || '').trim().toLowerCase() !== 'private'
);
const channelOptions = computed(() => {
  const result: { label: string; value: string }[] = [];
  const visit = (items: GlassBackgroundChannelNode[], depth = 0) => {
    items.forEach((item) => {
      if (isWorldGlassTriggerChannel(item)) {
        result.push({
          label: `${depth ? `${'· '.repeat(depth)}` : ''}${item.name || item.id}`,
          value: item.id,
        });
      }
      if (item.children?.length) {
        visit(item.children, depth + 1);
      }
    });
  };
  visit(effectiveChannelTree.value);
  return result;
});
function saveRules() {
  const preset = rulePreset.value;
  if (!preset || channelIds.value.length > 100) return;
  const selected = [...new Set(channelIds.value)];
  const enabled = ruleEnabled.value;
  void run(async id => {
    const { data } = await api.put(`${worldGlassURL(id)}/glass-presets/${encodeURIComponent(preset.id)}/channel-triggers`, { channelIds: selected, enabled });
    invalidateWorldGlass(id, data.revision);
    if (alive && id === worldId.value) rulePreset.value = null;
  });
}
const menuOptions = [{ label: '编辑', key: 'edit' }, { label: '自动切换规则', key: 'rules' }, { label: '删除', key: 'delete' }];
function menu(key: string | number, preset: WorldGlassPreset) { if (busy.value) return; if (key === 'edit') edit(preset); else if (key === 'rules') rules(preset); else remove(preset); }
const preview = (attachmentId: string) => ({ backgroundImage: `url(${JSON.stringify(resolveAttachmentUrl(attachmentId))})` });
const triggerCount = (id: string) => management.value?.triggers.filter(t => t.presetId === id).length || 0;
</script>

<template>
  <div class="world-glass">
    <p v-if="!worldId">进入世界后可查看世界预设。</p>
    <template v-else>
      <p v-if="worldGlassLoadError || error" role="alert">{{ worldGlassLoadError || error }} <NButton text @click="loadWorldGlassEffective(); load()">重试</NButton></p>
      <div v-if="worldGlassState?.enabled && worldGlassState.preset" class="world-glass__current">
        <span>当前由世界预设统一设置</span>
        <div class="world-glass__preview" :style="preview(worldGlassState.preset.attachmentId)" />
        <strong>{{ worldGlassState.preset.name }}</strong>
        <small>{{ worldGlassState.source === 'channel' ? '当前频道自动应用' : '世界默认' }}</small>
      </div>
      <p v-else>世界未应用统一背景，正在使用个人设置。</p>
      <template v-if="canManage">
        <div v-if="draft" class="world-glass__editor">
          <strong>{{ draftId ? '编辑预设' : '新建预设' }}</strong>
          <NInput v-model:value="draftName" :disabled="busy" placeholder="预设名称" :maxlength="80" />
          <GlassBackgroundEditor :key="`${worldId}:${draftId}`" :settings="draft" :disabled="busy" :upload-channel-id="props.channelId" @update="updateDraft" @uploading="uploading = $event" />
          <div class="world-glass__actions"><NButton :loading="busy" :disabled="uploading || !draftName.trim() || !draft.attachmentId" @click="savePreset">保存</NButton><NButton :disabled="busy" @click="draft = null">取消</NButton></div>
        </div>
        <div v-else-if="rulePreset" class="world-glass__editor">
          <strong>{{ rulePreset.name }} · 自动切换</strong>
          <label>启用 <NSwitch v-model:value="ruleEnabled" :disabled="busy" /></label>
          <label>频道<NSelect v-model:value="channelIds" multiple filterable :disabled="busy" :loading="channelLoading" :options="channelOptions" :menu-props="glassSelectMenuProps" placeholder="选择频道" /></label>
          <small v-if="!channelLoading && channelOptions.length === 0">当前世界没有可绑定的频道。</small>
          <small>最多 100 个频道。保存后，所选频道会改为绑定此预设。</small>
          <div class="world-glass__actions"><NButton :loading="busy" :disabled="channelIds.length > 100" @click="saveRules">保存</NButton><NButton :disabled="busy" @click="rulePreset = null">取消</NButton></div>
        </div>
        <template v-else>
          <div class="world-glass__actions"><NButton size="small" :disabled="busy || !management || management.presets.length >= 20" @click="edit()">＋ 新建预设</NButton><NButton size="small" :disabled="busy || !management?.state.enabled" @click="disable">停用世界背景</NButton></div>
          <span v-if="loading">正在加载预设…</span>
          <div class="world-glass__grid">
            <article v-for="preset in management?.presets || []" :key="preset.id" class="world-glass__card">
              <button class="world-glass__activate" :disabled="busy" :aria-label="`设为世界默认：${preset.name}`" @click="activate(preset)">
                <div class="world-glass__preview" :style="preview(preset.attachmentId)" />
                <strong>{{ preset.name }}</strong>
                <small v-if="management?.state.activePresetId === preset.id">{{ management.state.enabled ? '● 当前默认' : '默认（已停用）' }}</small>
                <small v-if="triggerCount(preset.id)">{{ triggerCount(preset.id) }} 个频道</small>
              </button>
              <NDropdown :options="menuOptions" trigger="click" :menu-props="glassDropdownMenuProps" @select="menu($event, preset)">
                <NButton class="world-glass__menu" quaternary circle size="small" :disabled="busy" :aria-label="`${preset.name} 操作`" title="更多操作">
                  <template #icon><NIcon><Dots /></NIcon></template>
                </NButton>
              </NDropdown>
            </article>
          </div>
        </template>
      </template>
    </template>
  </div>
</template>

<style scoped>
.world-glass, .world-glass__editor, .world-glass__current { display: flex; flex-direction: column; gap: 12px; }
.world-glass p { margin: 0; font-size: 13px; }
.world-glass small { color: var(--sc-text-secondary); }
.world-glass__actions { display: flex; flex-wrap: wrap; gap: 8px; }
.world-glass__grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.world-glass__card { position: relative; border: 1px solid var(--sc-border-mute); border-radius: 8px; overflow: hidden; }
.world-glass__preview { aspect-ratio: 16 / 9; width: 100%; background-size: cover; background-position: center; background-color: var(--sc-bg-page); border-radius: 6px; }
.world-glass__activate { width: 100%; padding: 0 0 10px; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; display: flex; flex-direction: column; gap: 6px; }
.world-glass__activate strong, .world-glass__activate small { padding: 0 8px; max-width: 100%; overflow-wrap: anywhere; }
.world-glass__menu { position: absolute; right: 8px; top: 8px; width: 30px; height: 30px; opacity: .42; transition: opacity 160ms ease, box-shadow 160ms ease; --n-color: var(--sc-bg-elevated) !important; --n-color-hover: var(--sc-bg-page) !important; --n-color-pressed: var(--sc-bg-page) !important; --n-color-focus: var(--sc-bg-elevated) !important; --n-border: 1px solid var(--sc-border-strong) !important; --n-border-hover: 1px solid var(--sc-text-primary) !important; --n-border-pressed: 1px solid var(--sc-text-primary) !important; --n-border-focus: 1px solid var(--sc-text-primary) !important; --n-text-color: var(--sc-text-primary) !important; box-shadow: 0 2px 8px #0005; }
.world-glass__card:has(.world-glass__preview:hover) .world-glass__menu, .world-glass__menu:hover, .world-glass__menu:focus-visible { opacity: 1; box-shadow: 0 3px 10px #0007; }
.world-glass label { display: flex; flex-direction: column; gap: 6px; }
:global(.v-binder-follower-container:has(.sc-glass-popup-menu)) { z-index: 3300 !important; }
</style>
