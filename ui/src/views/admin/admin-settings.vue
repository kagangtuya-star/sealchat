<script setup lang="tsx">
import AdminSettingsBase from './admin-settings-base.vue'
import AdminSettingsBot from './admin-settings-bot.vue'
import AdminSettingsAudio from './admin-settings-audio.vue'
import AdminSettingsExternalGlossary from './admin-settings-external-glossary.vue'
import AdminSettingsThemeStyle from './admin-settings-theme-style.vue'
import AdminSettingsUser from './admin-settings-user.vue'
import { computed, ref, watch } from 'vue'

type AdminTab = 'basic' | 'bot' | 'user' | 'external-glossary' | 'audio' | 'theme-style'

type AdminSettingsTabExpose = {
  save: () => Promise<void>
  isModified: () => boolean
}

const emit = defineEmits(['close']);
const activeTab = ref<AdminTab>('basic');
const basicSettingsRef = ref<AdminSettingsTabExpose | null>(null);
const themeStyleSettingsRef = ref<AdminSettingsTabExpose | null>(null);
const audioDrawerVisible = ref(false);
const lastNonAudioTab = ref<Exclude<AdminTab, 'audio'>>('basic');

const currentSettingsRef = computed<AdminSettingsTabExpose | null>(() => {
  if (activeTab.value === 'basic') {
    return basicSettingsRef.value;
  }
  if (activeTab.value === 'theme-style') {
    return themeStyleSettingsRef.value;
  }
  return null;
});
const showSaveAction = computed(() => !!currentSettingsRef.value);
const currentTabModified = computed(() => {
  if (!showSaveAction.value) return false;
  return currentSettingsRef.value?.isModified() ?? false;
});

watch(activeTab, (value, previous) => {
  if (value === 'audio') {
    audioDrawerVisible.value = true;
    activeTab.value = previous === 'audio' ? lastNonAudioTab.value : (previous as Exclude<AdminTab, 'audio'> | undefined) || 'basic';
    return;
  }
  lastNonAudioTab.value = value as Exclude<AdminTab, 'audio'>;
});

const cancel = () => {
  emit('close');
}

const closeAudioDrawer = () => {
  audioDrawerVisible.value = false;
}

const saveCurrentTab = async () => {
  await currentSettingsRef.value?.save();
}
</script>

<template>
  <div class="sc-admin-settings-shell pointer-events-auto md:w-2/3 w-5/6 border p-4 py-4">
    <div class="sc-admin-settings-header">
      <h2 class="text-lg mb-0">平台管理</h2>
      <div class="sc-admin-settings-actions">
        <n-button @click="cancel">关闭</n-button>
        <n-button
          v-if="showSaveAction"
          type="primary"
          :disabled="!currentTabModified"
          @click="saveCurrentTab"
        >
          保存
        </n-button>
      </div>
    </div>
    <n-tabs v-model:value="activeTab" type="line" animated class="sc-admin-settings-tabs">
      <n-tab-pane name="basic" tab="基本设置">
        <admin-settings-base ref="basicSettingsRef" />
      </n-tab-pane>
      <n-tab-pane name="bot" tab="BOT接入">
        <admin-settings-bot @close="cancel" />
      </n-tab-pane>
      <n-tab-pane name="user" tab="用户管理">
        <admin-settings-user @close="cancel" />
      </n-tab-pane>
      <n-tab-pane name="external-glossary" tab="外挂世界术语">
        <admin-settings-external-glossary />
      </n-tab-pane>
      <n-tab-pane name="theme-style" tab="主题与样式管理">
        <admin-settings-theme-style ref="themeStyleSettingsRef" />
      </n-tab-pane>
      <n-tab-pane name="audio" tab="音频素材管理" />
    </n-tabs>

    <n-drawer
      v-model:show="audioDrawerVisible"
      :width="'min(1280px, 96vw)'"
      placement="right"
      class="sc-admin-settings-audio-drawer"
    >
      <n-drawer-content closable body-content-style="padding: 0;">
        <template #header>
          <div class="sc-admin-settings-audio-drawer__header">
            <span class="sc-admin-settings-audio-drawer__title">音频素材管理</span>
            <n-button size="small" quaternary @click="closeAudioDrawer">退出</n-button>
          </div>
        </template>
        <div class="sc-admin-settings-audio-drawer__body">
          <admin-settings-audio />
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.sc-admin-settings-shell {
  margin-top: -5rem;
  min-height: 70vh;
  max-height: 78vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sc-admin-settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.sc-admin-settings-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sc-admin-settings-tabs {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sc-admin-settings-tabs :deep(.n-tabs-pane-wrapper),
.sc-admin-settings-tabs :deep(.n-tabs-content),
.sc-admin-settings-tabs :deep(.n-tab-pane),
.sc-admin-settings-tabs :deep(.n-tab-pane > *) {
  min-height: 0;
}

.sc-admin-settings-tabs :deep(.n-tabs-content),
.sc-admin-settings-tabs :deep(.n-tabs-pane-wrapper) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.sc-admin-settings-tabs :deep(.n-tab-pane) {
  height: 100%;
  overflow: hidden;
}

.sc-admin-settings-audio-drawer__header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.sc-admin-settings-audio-drawer__title {
  font-size: 16px;
  font-weight: 600;
}

.sc-admin-settings-audio-drawer__body {
  height: calc(100vh - 96px);
  min-height: 0;
  padding: 16px;
  overflow: hidden;
}

.sc-admin-settings-audio-drawer :deep(.n-drawer-content) {
  overflow: hidden;
}

@media (max-width: 768px) {
  .sc-admin-settings-shell {
    margin-top: -2rem;
    max-height: 85vh;
  }

  .sc-admin-settings-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .sc-admin-settings-audio-drawer__body {
    height: calc(100vh - 88px);
    padding: 12px;
  }
}
</style>
