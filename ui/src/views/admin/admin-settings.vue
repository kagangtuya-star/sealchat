<script setup lang="tsx">
import AdminSettingsBase from './admin-settings-base.vue'
import AdminSettingsBot from './admin-settings-bot.vue'
import AdminSettingsExternalGlossary from './admin-settings-external-glossary.vue'
import AdminSettingsUser from './admin-settings-user.vue'
import { computed, ref } from 'vue'

type AdminTab = 'basic' | 'bot' | 'user' | 'external-glossary'

type AdminSettingsBaseExpose = {
  save: () => Promise<void>
  isModified: () => boolean
}

const emit = defineEmits(['close']);
const activeTab = ref<AdminTab>('basic');
const basicSettingsRef = ref<AdminSettingsBaseExpose | null>(null);

const showSaveAction = computed(() => activeTab.value === 'basic');
const basicSettingsModified = computed(() => {
  if (!showSaveAction.value) return false;
  return basicSettingsRef.value?.isModified() ?? false;
});

const cancel = () => {
  emit('close');
}

const saveBasicSettings = async () => {
  await basicSettingsRef.value?.save();
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
          :disabled="!basicSettingsModified"
          @click="saveBasicSettings"
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
    </n-tabs>
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
.sc-admin-settings-tabs :deep(.n-tab-pane),
.sc-admin-settings-tabs :deep(.n-tab-pane > *) {
  min-height: 0;
}

.sc-admin-settings-tabs :deep(.n-tab-pane) {
  height: 100%;
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
}
</style>
