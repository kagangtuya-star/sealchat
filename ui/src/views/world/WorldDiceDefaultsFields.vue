<script setup lang="ts">
const mode = defineModel<'builtin' | 'bot'>('mode', { default: 'builtin' });
const botId = defineModel<string>('botId', { default: '' });

defineProps<{
  botOptionsLoading?: boolean;
  botSelectOptions: Array<{ label: string; value: string }>;
  title?: string;
  hint?: string;
  botPlaceholder?: string;
}>();

const diceModeOptions = [
  { label: '内置掷骰', value: 'builtin' },
  { label: 'BOT掷骰', value: 'bot' },
] as const;
</script>

<template>
  <div class="manager-permission-group">
    <div class="manager-permission-block">
      <div v-if="title" class="manager-permission-title">{{ title }}</div>
      <n-radio-group v-model:value="mode">
        <n-space>
          <n-radio
            v-for="item in diceModeOptions"
            :key="item.value"
            :value="item.value"
          >
            {{ item.label }}
          </n-radio>
        </n-space>
      </n-radio-group>
    </div>
    <div v-if="mode === 'bot'" class="manager-permission-block">
      <div class="manager-permission-title">默认 BOT</div>
      <n-select
        v-model:value="botId"
        :options="botSelectOptions"
        :loading="botOptionsLoading"
        :placeholder="botPlaceholder || '选择当前已添加的 BOT'"
        clearable
      />
    </div>
    <div v-if="hint" class="manager-permission-hint">{{ hint }}</div>
  </div>
</template>

<style scoped>
.manager-permission-group {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.manager-permission-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.manager-permission-title {
  color: var(--sc-text-primary);
  font-size: 13px;
}

.manager-permission-hint {
  color: var(--sc-text-secondary);
  font-size: 12px;
}
</style>
