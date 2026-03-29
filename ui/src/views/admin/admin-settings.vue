<script setup lang="tsx">
import AdminSettingsBase from './admin-settings-base.vue'
import AdminSettingsBot from './admin-settings-bot.vue'
import AdminSettingsUser from './admin-settings-user.vue'

const emit = defineEmits(['close']);

const cancel = () => {
  emit('close');
}
</script>

<template>
  <div class="sc-admin-settings-shell pointer-events-auto md:w-2/3 w-5/6 border p-4 py-4">
    <h2 class="text-lg mb-2">平台管理</h2>
    <n-tabs type="line" animated class="sc-admin-settings-tabs">
      <n-tab-pane name="basic" tab="基本设置">
        <admin-settings-base @close="cancel" />
      </n-tab-pane>
      <n-tab-pane name="bot" tab="BOT接入">
        <admin-settings-bot @close="cancel" />
      </n-tab-pane>
      <n-tab-pane name="user" tab="用户管理">
        <admin-settings-user @close="cancel" />
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
}
</style>
