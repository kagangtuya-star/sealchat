<script setup lang="tsx">
import AdminSettingsBase from './admin-settings-base.vue'
import AdminSettingsBot from './admin-settings-bot.vue'
import AdminSettingsUser from './admin-settings-user.vue'
import { onMounted } from 'vue';
import { useUtilsStore } from '@/stores/utils';

const utils = useUtilsStore();

const emit = defineEmits(['close']);

const cancel = () => {
  emit('close');
}

onMounted(async () => {
})
</script>

<template>
  <div class="sc-admin-settings-shell pointer-events-auto md:w-2/3 w-5/6 border p-4 py-4" style="margin-top: -5rem; min-height: 70vh;">
    <h2 class="text-lg mb-2">平台管理</h2>
    <n-tabs type="line" animated>
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
