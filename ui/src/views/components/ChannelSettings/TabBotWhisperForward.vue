<script lang="ts" setup>
import { computed, ref, watch, type PropType } from 'vue';
import { cloneDeep } from 'lodash-es';
import { useDialog, useMessage } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { dialogAskConfirm } from '@/utils/dialog';
import type {
  BotWhisperForwardConfig,
  BotWhisperForwardRule,
  BotWhisperForwardRuleLogic,
  BotWhisperForwardRuleType,
  SChannel,
} from '@/types';

const props = defineProps({
  channel: {
    type: Object as PropType<SChannel>,
  },
});

const message = useMessage();
const dialog = useDialog();
const chat = useChatStore();
const userStore = useUserStore();

const ruleTypeOptions = [
  { label: '暗骰兼容规则', value: 'legacy_hidden_dice' },
  { label: '关键字匹配', value: 'keyword' },
  { label: '正则匹配', value: 'regex' },
  { label: '全部消息', value: 'all' },
];

const ruleLogicOptions = [
  { label: '任一规则命中', value: 'any' },
  { label: '全部规则命中', value: 'all' },
];

const createDefaultConfig = (): BotWhisperForwardConfig => ({
  enabled: true,
  asWhisper: true,
  appendAtTargetsWhenWhisper: false,
  ruleLogic: 'any',
  rules: [
    {
      id: 'legacy-hidden-dice',
      type: 'legacy_hidden_dice',
      enabled: true,
    },
  ],
});

const createRule = (type: BotWhisperForwardRuleType = 'keyword'): BotWhisperForwardRule => {
  const id = `rule-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
  if (type === 'keyword') {
    return { id, type, enabled: true, keyword: '' };
  }
  if (type === 'regex') {
    return { id, type, enabled: true, pattern: '', flags: '' };
  }
  return { id, type, enabled: true };
};

const toBoolean = (value: unknown, fallback: boolean): boolean => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'string') {
    const text = value.trim().toLowerCase();
    if (text === 'true' || text === '1') return true;
    if (text === 'false' || text === '0') return false;
  }
  if (typeof value === 'number') {
    if (value === 1) return true;
    if (value === 0) return false;
  }
  return fallback;
};

const normalizeRule = (rule: BotWhisperForwardRule, index: number): BotWhisperForwardRule => {
  const source = (rule || {}) as any;
  const type = (source.type || source.rule_type || 'keyword') as BotWhisperForwardRuleType;
  const id = String(source.id || source.rule_id || '').trim() || `rule-${index + 1}`;
  const normalized: BotWhisperForwardRule = {
    id,
    type,
    enabled: toBoolean(source.enabled ?? source.is_enabled, true),
  };
  if (type === 'keyword') {
    normalized.keyword = String(source.keyword || source.key_word || '').trim();
  }
  if (type === 'regex') {
    normalized.pattern = String(source.pattern || source.regex || '').trim();
    normalized.flags = String(source.flags || source.regex_flags || '').trim().toLowerCase();
  }
  return normalized;
};

const normalizeConfig = (cfg?: BotWhisperForwardConfig): BotWhisperForwardConfig => {
  const source = (cfg || {}) as any;
  const base = createDefaultConfig();
  const rules = Array.isArray(source.rules) ? source.rules : [];
  const normalizedRules = rules
    .map((rule, index) => normalizeRule(rule, index))
    .filter((rule) => {
      if (rule.type === 'keyword') {
        return !!rule.keyword;
      }
      if (rule.type === 'regex') {
        return !!rule.pattern;
      }
      return true;
    });
  return {
    enabled: toBoolean(source.enabled ?? source.is_enabled, base.enabled),
    asWhisper: toBoolean(source.asWhisper ?? source.as_whisper, base.asWhisper),
    appendAtTargetsWhenWhisper: toBoolean(
      source.appendAtTargetsWhenWhisper ?? source.append_at_targets_when_whisper,
      base.appendAtTargetsWhenWhisper,
    ),
    ruleLogic: ((source.ruleLogic ?? source.rule_logic) === 'all' ? 'all' : 'any') as BotWhisperForwardRuleLogic,
    rules: normalizedRules.length > 0 ? normalizedRules : base.rules,
  };
};

const parseConfig = (raw?: unknown): BotWhisperForwardConfig => {
  if (raw == null) {
    return createDefaultConfig();
  }
  if (typeof raw === 'object') {
    return normalizeConfig(raw as BotWhisperForwardConfig);
  }
  const text = String(raw).trim();
  if (!text) {
    return createDefaultConfig();
  }
  try {
    const parsed = JSON.parse(text);
    if (typeof parsed === 'string') {
      try {
        return normalizeConfig(JSON.parse(parsed));
      } catch {
        return normalizeConfig();
      }
    }
    return normalizeConfig(parsed);
  } catch {
    return createDefaultConfig();
  }
};

const configModel = ref<BotWhisperForwardConfig>(createDefaultConfig());
const initialSnapshot = ref(JSON.stringify(normalizeConfig(configModel.value)));
const saving = ref(false);

const currentWorldId = computed(() => String((props.channel as any)?.worldId || '').trim());

watch(
  () => currentWorldId.value,
  async (worldId) => {
    if (!worldId) return;
    try {
      await chat.worldDetail(worldId);
    } catch (err) {
      console.warn('加载世界详情失败', err);
    }
  },
  { immediate: true },
);

watch(
  () => [
    props.channel?.id,
    props.channel?.botWhisperForwardConfig,
    (props.channel as any)?.bot_whisper_forward_config,
  ] as const,
  () => {
    const rawConfig = props.channel?.botWhisperForwardConfig ?? (props.channel as any)?.bot_whisper_forward_config;
    const parsed = parseConfig(rawConfig);
    configModel.value = cloneDeep(parsed);
    initialSnapshot.value = JSON.stringify(normalizeConfig(parsed));
  },
  { immediate: true },
);

const canEdit = computed(() => {
  if (userStore.checkPerm('mod_admin')) return true;
  const worldId = currentWorldId.value;
  if (!worldId) return false;
  const detail = chat.worldDetailMap[worldId];
  const role = detail?.memberRole;
  const ownerId = detail?.world?.ownerId || chat.worldMap[worldId]?.ownerId;
  const selfId = userStore.info?.id;
  if (ownerId && selfId && ownerId === selfId) return true;
  return role === 'owner' || role === 'admin';
});

const readOnly = computed(() => !canEdit.value);
const isDirty = computed(() => JSON.stringify(normalizeConfig(configModel.value)) !== initialSnapshot.value);

const addRule = (type: BotWhisperForwardRuleType = 'keyword') => {
  configModel.value.rules.push(createRule(type));
};

const removeRule = (index: number) => {
  if (configModel.value.rules.length <= 1) {
    message.warning('至少保留一条规则');
    return;
  }
  configModel.value.rules.splice(index, 1);
};

const resetToDefault = async () => {
  if (readOnly.value) return;
  const confirmed = await dialogAskConfirm(dialog, '恢复默认配置', '将恢复为“暗骰兼容规则”默认行为，是否继续？');
  if (!confirmed) return;
  configModel.value = createDefaultConfig();
};

const saveConfig = async (applyToWorld = false) => {
  if (!props.channel?.id) {
    message.error('频道不存在');
    return;
  }
  if (readOnly.value) {
    message.warning('仅世界管理员可修改配置');
    return;
  }
  if (applyToWorld) {
    const confirmed = await dialogAskConfirm(
      dialog,
      '应用到当前世界所有频道',
      '将把当前配置覆盖到该世界全部频道，是否继续？',
    );
    if (!confirmed) return;
  }
  const payload = normalizeConfig(configModel.value);
  saving.value = true;
  try {
    const result = await chat.updateChannelBotWhisperForwardConfig(
      props.channel.id,
      payload,
      { applyToWorld },
    );
    configModel.value = cloneDeep(payload);
    initialSnapshot.value = JSON.stringify(payload);
    if (applyToWorld) {
      message.success(`已应用到 ${result?.updated_count ?? 0} 个频道`);
    } else {
      message.success('配置已保存');
    }
  } catch (err: any) {
    message.error(err?.message || '保存失败');
  } finally {
    saving.value = false;
  }
};
</script>

<template>
  <div class="tab-bot-whisper-forward">
    <n-alert type="info" :show-icon="false" class="mb-3">
      优先识别消息中的 <code>SEALCHAT-Group:&lt;channelId&gt;</code>，若不存在则回退到当前用户所在频道。
    </n-alert>
    <n-alert v-if="readOnly" type="warning" :show-icon="false" class="mb-3">
      当前为只读模式：仅世界管理员及以上可修改本配置。
    </n-alert>

    <n-space vertical :size="16">
      <n-card size="small" title="基础设置">
        <n-form label-placement="left" label-width="160" class="base-form">
          <n-form-item label="启用转发">
            <n-switch v-model:value="configModel.enabled" :disabled="readOnly" />
          </n-form-item>
          <n-form-item label="以悄悄话转发">
            <n-switch v-model:value="configModel.asWhisper" :disabled="readOnly || !configModel.enabled" />
          </n-form-item>
          <n-form-item label="@ 追加到悄悄话接收者">
            <n-switch
              v-model:value="configModel.appendAtTargetsWhenWhisper"
              :disabled="readOnly || !configModel.enabled || !configModel.asWhisper"
            />
          </n-form-item>
          <n-form-item label="规则逻辑">
            <n-radio-group v-model:value="configModel.ruleLogic" :disabled="readOnly || !configModel.enabled">
              <n-radio-button v-for="item in ruleLogicOptions" :key="item.value" :value="item.value">
                {{ item.label }}
              </n-radio-button>
            </n-radio-group>
          </n-form-item>
        </n-form>
      </n-card>

      <n-card size="small" title="规则列表">
        <div class="rules-scroll">
          <div v-for="(rule, index) in configModel.rules" :key="rule.id || index" class="rule-item">
            <div class="rule-header">
              <n-space align="center" :wrap="true">
                <n-switch v-model:value="rule.enabled" :disabled="readOnly || !configModel.enabled" />
                <n-select
                  v-model:value="rule.type"
                  :options="ruleTypeOptions"
                  style="min-width: 220px;"
                  :disabled="readOnly || !configModel.enabled"
                />
                <n-button
                  tertiary
                  type="error"
                  :disabled="readOnly || !configModel.enabled || configModel.rules.length <= 1"
                  @click="removeRule(index)"
                >
                  删除
                </n-button>
              </n-space>
            </div>
            <div class="rule-body">
              <n-input
                v-if="rule.type === 'keyword'"
                v-model:value="rule.keyword"
                type="textarea"
                :autosize="{ minRows: 2, maxRows: 5 }"
                placeholder="输入关键字，包含即命中（支持多行文本）"
                :disabled="readOnly || !configModel.enabled"
              />
              <template v-if="rule.type === 'regex'">
                <n-input
                  v-model:value="rule.pattern"
                  type="textarea"
                  :autosize="{ minRows: 3, maxRows: 8 }"
                  placeholder="输入正则表达式（示例：暗骰|hidden\\s*dice）"
                  :disabled="readOnly || !configModel.enabled"
                />
                <n-input
                  v-model:value="rule.flags"
                  placeholder="flags（可选）：i/m"
                  :disabled="readOnly || !configModel.enabled"
                />
              </template>
            </div>
          </div>
        </div>
        <n-space class="rule-add-actions" :wrap="true">
            <n-button :disabled="readOnly || !configModel.enabled" @click="addRule('keyword')">
              新增关键字规则
            </n-button>
            <n-button :disabled="readOnly || !configModel.enabled" @click="addRule('regex')">
              新增正则规则
            </n-button>
            <n-button :disabled="readOnly || !configModel.enabled" @click="addRule('all')">
              新增全部消息规则
            </n-button>
            <n-button :disabled="readOnly || !configModel.enabled" @click="addRule('legacy_hidden_dice')">
              新增暗骰兼容规则
            </n-button>
        </n-space>
      </n-card>

      <n-space class="footer-actions" :wrap="true">
        <n-button :disabled="readOnly || !isDirty" :loading="saving" type="primary" @click="saveConfig(false)">
          保存当前频道配置
        </n-button>
        <n-button :disabled="readOnly || saving" :loading="saving" @click="saveConfig(true)">
          应用到当前世界所有频道
        </n-button>
        <n-button :disabled="readOnly || saving" @click="resetToDefault">
          恢复默认
        </n-button>
      </n-space>
    </n-space>
  </div>
</template>

<style scoped>
.tab-bot-whisper-forward {
  padding-top: 8px;
}

.rule-item {
  width: 100%;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  padding: 12px;
  background: var(--n-color);
}

.rule-header {
  margin-bottom: 10px;
}

.rule-body {
  display: grid;
  gap: 10px;
}

.rules-scroll {
  max-height: 46vh;
  overflow: auto;
  display: grid;
  gap: 12px;
  padding-right: 6px;
}

.rules-scroll::-webkit-scrollbar {
  width: 6px;
}

.rules-scroll::-webkit-scrollbar-thumb {
  background: rgba(120, 120, 120, 0.35);
  border-radius: 999px;
}

.rules-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.rule-add-actions {
  margin-top: 12px;
}

.footer-actions {
  width: 100%;
}

@media (max-width: 768px) {
  .base-form :deep(.n-form-item) {
    grid-template-columns: 1fr;
  }

  .base-form :deep(.n-form-item-label) {
    margin-bottom: 6px;
  }

  .rule-header :deep(.n-space) {
    width: 100%;
  }

  .rule-header :deep(.n-select),
  .rule-header :deep(.n-button) {
    width: 100%;
  }

  .rule-add-actions :deep(.n-button),
  .footer-actions :deep(.n-button) {
    width: 100%;
  }
}
</style>
