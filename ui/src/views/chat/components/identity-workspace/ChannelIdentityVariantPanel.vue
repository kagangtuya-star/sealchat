<script setup lang="ts">
import { computed } from 'vue';
import AvatarVue from '@/components/avatar.vue';
import type { ChannelIdentityVariant } from '@/types';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import {
  describeIdentityMatchRule,
  identityVariantKeywordMatchOptions,
  identityVariantMatchModeOptions,
  identityVariantRegexMatchOptions,
  resolveVariantNote,
  resolveVariantSelectorEmojiSrc,
  type IdentityVariantDraft,
  type IdentityVariantMatchMode,
  type IdentityVariantResetDraft,
  type IdentityWorkspaceAvatar,
} from './identityWorkspaceTypes';

// 「头像差分」编辑域：左侧（或窄屏顶部）是「默认状态 + 差分」列表，右侧是当前选中项详情。
// 选中「默认状态」时编辑恢复默认规则；选中差分时编辑该差分。`form` / `resetForm` 为 chat.vue 持有的草稿。
const props = withDefaults(defineProps<{
  available: boolean;
  unavailableReason?: 'unsaved' | 'temporary' | '';
  variants: ChannelIdentityVariant[];
  baseAvatar: IdentityWorkspaceAvatar;
  baseName: string;
  baseColor?: string;
  managingBot?: boolean;
  sharedIdentity?: boolean;
  editorActive: boolean;
  editorMode: 'create' | 'edit';
  editingVariantId?: string;
  form: IdentityVariantDraft;
  colorDraft: string;
  avatarPreview?: string;
  dirty?: boolean;
  submitting?: boolean;
  resetForm: IdentityVariantResetDraft;
  resetRule: { mode: IdentityVariantMatchMode; config: string; content: string };
  identitySubmitting?: boolean;
}>(), {
  unavailableReason: '',
  baseColor: '',
  managingBot: false,
  sharedIdentity: false,
  editingVariantId: '',
  avatarPreview: '',
  dirty: false,
  submitting: false,
  identitySubmitting: false,
});

const emit = defineEmits<{
  (e: 'select', variant: ChannelIdentityVariant | null): void;
  (e: 'create'): void;
  (e: 'save'): void;
  (e: 'revert'): void;
  (e: 'delete', variant: ChannelIdentityVariant): void;
  (e: 'avatar-upload'): void;
  (e: 'avatar-remove'): void;
  (e: 'pick-emoji'): void;
  (e: 'open-theater'): void;
  (e: 'update:colorDraft', value: string): void;
  (e: 'color-picker-update', value: string | null): void;
  (e: 'color-commit'): void;
  (e: 'color-clear'): void;
  (e: 'reset-save'): void;
  (e: 'save-identity'): void;
  (e: 'edit-profile'): void;
}>();

const isDefaultSelected = computed(() => !props.editorActive);
const isCreating = computed(() => props.editorActive && props.editorMode === 'create');
const editingVariant = computed(() => (
  props.editorActive && props.editorMode === 'edit'
    ? props.variants.find(item => item.id === props.editingVariantId) || null
    : null
));

const variantAvatar = (avatarAttachmentId: string, color: string | undefined, displayName: string | undefined, preview = ''): IdentityWorkspaceAvatar => {
  const base = props.baseAvatar;
  return {
    ...base,
    src: preview || resolveAttachmentUrl(avatarAttachmentId) || base.src,
    textFallbackAccentColor: base.textFallbackStyle ? (color || props.baseColor) : undefined,
    fallbackText: displayName || base.fallbackText,
  };
};

const draftHasOwnAvatar = computed(() => Boolean(props.avatarPreview || props.form.avatarAttachmentId));
const draftAvatar = computed(() => variantAvatar(
  props.form.avatarAttachmentId,
  props.form.color,
  props.form.displayName,
  props.avatarPreview,
));
const draftEffectiveColor = computed(() => props.form.color || props.baseColor);
const draftEmojiSrc = computed(() => resolveVariantSelectorEmojiSrc(props.form.selectorEmoji));
const draftMatch = computed(() => props.form.matchDrafts[props.form.matchMode]);
const draftTheaterCustomized = computed(() => Object.keys(props.form.theaterPresentation || {}).length > 0);
const resetMatch = computed(() => props.resetForm.matchDrafts[props.resetForm.matchMode]);
const resetDirty = computed(() => {
  const draft = resetMatch.value;
  return props.resetForm.matchMode !== props.resetRule.mode
    || String(draft?.config || '').trim() !== String(props.resetRule.config || '').trim()
    || String(draft?.content || '').trim() !== String(props.resetRule.content || '').trim();
});

const ruleSummary = (variant: ChannelIdentityVariant) => describeIdentityMatchRule(variant.matchMode, variant.matchConfig, variant.keyword);
const savedResetSummary = computed(() => describeIdentityMatchRule(props.resetRule.mode, props.resetRule.config, props.resetRule.content));

const contentPlaceholder = (mode: IdentityVariantMatchMode, config: string, kind: 'variant' | 'reset') => {
  if (mode === 'prefix') return kind === 'reset' ? '例如：还原' : '例如：笑';
  if (mode === 'keyword') {
    if (kind === 'reset') return config === 'all' ? '例如：结束&恢复' : '例如：还原|恢复';
    return config === 'all' ? '例如：笑&挥手' : '例如：笑|开心';
  }
  return kind === 'reset' ? '例如：^(还原|恢复)' : '例如：笑|开心|挥手';
};

const configLabel = (mode: IdentityVariantMatchMode) => (
  mode === 'prefix' ? '前缀符号' : mode === 'keyword' ? '关键词匹配类型' : '正则匹配方式'
);

const ruleExample = (mode: IdentityVariantMatchMode, config: string, content: string, target: string) => {
  const text = String(content || '').trim();
  if (!text) return '';
  if (mode === 'prefix') return `发送「${String(config || '').trim() || '='}${text} 正文」时${target}，并移除匹配头。`;
  if (mode === 'keyword') return `消息${config === 'all' ? '同时包含全部关键词' : '包含任一关键词'}时${target}。`;
  return `消息内容匹配该正则时${target}；无效规则不会触发。`;
};

const unavailableText = computed(() => (
  props.unavailableReason === 'temporary'
    ? '临时 NPC 角色不配置头像差分。'
    : '保存角色后即可在这里配置头像差分。'
));
const panelHint = computed(() => {
  if (props.managingBot) return 'BOT 发送消息时由后端按匹配规则自动选择差分或恢复默认头像。';
  if (props.sharedIdentity) return '差分会随跨频道角色同步到全部副本。';
  return '发言时按匹配规则切换差分，或恢复为默认状态。';
});
</script>

<template>
  <div class="idvp">
    <header class="idvp__header">
      <div class="idvp__header-text">
        <div class="idvp__title">头像差分</div>
        <div class="idvp__hint">{{ panelHint }}</div>
      </div>
      <n-button v-if="available" size="small" type="primary" secondary :disabled="isCreating" @click="emit('create')">
        新增差分
      </n-button>
    </header>

    <div v-if="!available" class="idvp__unavailable">
      <div class="idvp-stack" aria-hidden="true">
        <AvatarVue class="idvp-stack__base" :size="56" :border="false" v-bind="baseAvatar" />
        <span class="idvp-stack__ghost">+</span>
        <span class="idvp-stack__ghost">+</span>
      </div>
      <div class="idvp__unavailable-title">默认状态 + 若干头像差分</div>
      <div class="idvp__unavailable-text">
        {{ unavailableText }}差分以角色资料为默认状态，按匹配规则覆盖头像、昵称、颜色与小剧场演出。
      </div>
      <n-button
        v-if="unavailableReason === 'unsaved'"
        type="primary"
        :loading="identitySubmitting"
        @click="emit('save-identity')"
      >
        创建角色并配置差分
      </n-button>
    </div>

    <div v-else class="idvp__body">
      <nav class="idvp-list" aria-label="差分列表">
        <button
          type="button"
          class="idvp-item idvp-item--default"
          :class="{ 'is-active': isDefaultSelected }"
          :aria-current="isDefaultSelected ? 'true' : undefined"
          @click="emit('select', null)"
        >
          <AvatarVue class="idvp-item__avatar" :size="36" :border="false" v-bind="baseAvatar" />
          <span class="idvp-item__text">
            <span class="idvp-item__name">默认状态</span>
            <span class="idvp-item__sub">恢复：{{ savedResetSummary }}</span>
          </span>
        </button>
        <div class="idvp-list__branch">
          <button
            v-for="variant in variants"
            :key="variant.id"
            type="button"
            class="idvp-item"
            :class="{
              'is-active': editingVariant?.id === variant.id,
              'is-disabled': variant.enabled === false,
            }"
            :aria-current="editingVariant?.id === variant.id ? 'true' : undefined"
            @click="emit('select', variant)"
          >
            <span class="idvp-item__avatar-wrap" :class="{ 'is-inherited': !variant.avatarAttachmentId }">
              <AvatarVue
                class="idvp-item__avatar"
                :size="36"
                :border="false"
                v-bind="variantAvatar(variant.avatarAttachmentId, variant.color, variant.displayName)"
              />
              <span class="idvp-item__emoji">
                <img
                  v-if="resolveVariantSelectorEmojiSrc(variant.selectorEmoji)"
                  :src="resolveVariantSelectorEmojiSrc(variant.selectorEmoji)"
                  :alt="resolveVariantNote(variant)"
                />
                <template v-else>{{ variant.selectorEmoji || '🙂' }}</template>
              </span>
            </span>
            <span class="idvp-item__text">
              <span class="idvp-item__name">
                {{ resolveVariantNote(variant) }}
                <span v-if="variant.enabled === false" class="idvp-item__flag">停用</span>
              </span>
              <span class="idvp-item__sub">{{ ruleSummary(variant) }}</span>
            </span>
          </button>
          <button
            v-if="isCreating"
            type="button"
            class="idvp-item is-active is-pending"
            aria-current="true"
          >
            <span class="idvp-item__avatar-wrap" :class="{ 'is-inherited': !draftHasOwnAvatar }">
              <AvatarVue class="idvp-item__avatar" :size="36" :border="false" v-bind="draftAvatar" />
              <span class="idvp-item__emoji">
                <img v-if="draftEmojiSrc" :src="draftEmojiSrc" alt="" />
                <template v-else>{{ form.selectorEmoji || '＋' }}</template>
              </span>
            </span>
            <span class="idvp-item__text">
              <span class="idvp-item__name">{{ form.note.trim() || '新差分' }}</span>
              <span class="idvp-item__sub">未保存</span>
            </span>
          </button>
          <div v-if="!variants.length && !isCreating" class="idvp-list__empty">
            还没有差分
          </div>
        </div>
      </nav>

      <div class="idvp-detail">
        <template v-if="isDefaultSelected">
          <div class="idvp-detail__scroll">
            <section class="idvp-card idvp-base">
              <AvatarVue :size="56" :border="false" v-bind="baseAvatar" />
              <div class="idvp-base__text">
                <div class="idvp-base__eyebrow">默认状态</div>
                <div class="idvp-base__name" :style="baseColor ? { color: baseColor } : undefined">{{ baseName || '未命名角色' }}</div>
                <div class="idvp-hint">
                  所有差分都以这里为基础覆盖；差分未设置的头像、昵称、颜色与小剧场演出沿用角色资料。
                </div>
                <n-button class="idvp-base__goto" size="tiny" text type="primary" @click="emit('edit-profile')">
                  编辑角色资料
                </n-button>
              </div>
            </section>

            <section class="idvp-card">
              <div class="idvp-card__head">
                <div class="idvp-card__title">恢复默认规则</div>
                <n-popover trigger="hover" placement="bottom-end" :width="260">
                  <template #trigger>
                    <button type="button" class="idvp-help" aria-label="匹配式说明">?</button>
                  </template>
                  <div class="idvp-help__content">
                    <div><strong>前缀：</strong>消息以指定符号和匹配内容开头时恢复默认头像，并移除匹配头。</div>
                    <div><strong>关键词：</strong>消息包含关键词时恢复；| 表示“或”，&amp; 表示“与”。</div>
                    <div><strong>正则：</strong>使用正则规则匹配消息内容；无效规则不会触发。</div>
                  </div>
                </n-popover>
              </div>
              <n-radio-group v-model:value="resetForm.matchMode" size="small">
                <n-radio-button v-for="option in identityVariantMatchModeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </n-radio-button>
              </n-radio-group>
              <div class="idvp-match">
                <div class="idvp-field">
                  <div class="idvp-field__label">{{ configLabel(resetForm.matchMode) }}</div>
                  <n-input v-if="resetForm.matchMode === 'prefix'" v-model:value="resetMatch.config" maxlength="8" placeholder="=" />
                  <n-select v-else-if="resetForm.matchMode === 'keyword'" v-model:value="resetMatch.config" :options="identityVariantKeywordMatchOptions" />
                  <n-select v-else v-model:value="resetMatch.config" :options="identityVariantRegexMatchOptions" />
                </div>
                <div class="idvp-field">
                  <div class="idvp-field__label">匹配内容</div>
                  <n-input
                    v-model:value="resetMatch.content"
                    maxlength="255"
                    :placeholder="contentPlaceholder(resetForm.matchMode, resetMatch.config, 'reset')"
                  />
                </div>
              </div>
              <div v-if="ruleExample(resetForm.matchMode, resetMatch.config, resetMatch.content, '恢复默认状态')" class="idvp-example">
                {{ ruleExample(resetForm.matchMode, resetMatch.config, resetMatch.content, '恢复默认状态') }}
              </div>
              <div class="idvp-card__foot">
                <div class="idvp-reset-status">
                  <span class="idvp-hint">规则属于角色资料，保存时会一并提交角色资料中的当前修改。</span>
                  <span v-if="resetDirty" class="idvp-reset-status__dirty">有未保存的修改</span>
                </div>
                <n-button
                  size="small"
                  type="primary"
                  secondary
                  :loading="identitySubmitting"
                  :disabled="!resetDirty || identitySubmitting"
                  @click="emit('reset-save')"
                >
                  保存规则
                </n-button>
              </div>
            </section>

            <div v-if="!variants.length" class="idvp-empty">
              <div class="idvp-empty__title">还没有头像差分</div>
              <div class="idvp-hint">为战斗、开心、严肃等状态准备不同的头像，发言时用匹配规则快速切换。</div>
              <n-button size="small" type="primary" @click="emit('create')">创建首个差分</n-button>
            </div>
          </div>
        </template>

        <template v-else>
          <div class="idvp-detail__scroll">
            <section class="idvp-card idvp-head">
              <button type="button" class="idvp-head__emoji" aria-label="选择差分表情" @click="emit('pick-emoji')">
                <img v-if="draftEmojiSrc" :src="draftEmojiSrc" alt="差分表情" />
                <span v-else-if="form.selectorEmoji">{{ form.selectorEmoji }}</span>
                <span v-else class="idvp-head__emoji-empty">选择<br>表情</span>
              </button>
              <div class="idvp-head__main">
                <n-input
                  v-model:value="form.note"
                  maxlength="255"
                  placeholder="差分备注，例如 战斗中 / 严肃 / 开心"
                />
                <div class="idvp-head__meta">
                  <span>{{ isCreating ? '新差分 · 尚未保存' : '编辑差分' }}</span>
                  <n-button v-if="form.selectorEmoji" text size="tiny" @click="form.selectorEmoji = ''">清除表情</n-button>
                </div>
              </div>
              <n-switch v-model:value="form.enabled" size="small">
                <template #checked>启用</template>
                <template #unchecked>停用</template>
              </n-switch>
            </section>

            <section class="idvp-card">
              <div class="idvp-card__head">
                <div class="idvp-card__title">触发规则</div>
                <n-popover trigger="hover" placement="bottom-end" :width="260">
                  <template #trigger>
                    <button type="button" class="idvp-help" aria-label="匹配式说明">?</button>
                  </template>
                  <div class="idvp-help__content">
                    <div><strong>前缀：</strong>消息以指定符号和关键词开头时触发，例如 =笑。</div>
                    <div><strong>关键词：</strong>消息包含关键词时触发；| 表示“或”，&amp; 表示“与”。</div>
                    <div><strong>正则：</strong>使用正则规则匹配消息内容；无效规则不会触发。</div>
                  </div>
                </n-popover>
              </div>
              <n-radio-group v-model:value="form.matchMode" size="small">
                <n-radio-button v-for="option in identityVariantMatchModeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </n-radio-button>
              </n-radio-group>
              <div class="idvp-match">
                <div class="idvp-field">
                  <div class="idvp-field__label">{{ configLabel(form.matchMode) }}</div>
                  <n-input v-if="form.matchMode === 'prefix'" v-model:value="draftMatch.config" maxlength="8" placeholder="=" />
                  <n-select v-else-if="form.matchMode === 'keyword'" v-model:value="draftMatch.config" :options="identityVariantKeywordMatchOptions" />
                  <n-select v-else v-model:value="draftMatch.config" :options="identityVariantRegexMatchOptions" />
                </div>
                <div class="idvp-field">
                  <div class="idvp-field__label">匹配内容</div>
                  <n-input
                    v-model:value="draftMatch.content"
                    maxlength="255"
                    :placeholder="contentPlaceholder(form.matchMode, draftMatch.config, 'variant')"
                  />
                </div>
              </div>
              <div v-if="ruleExample(form.matchMode, draftMatch.config, draftMatch.content, '切换到此差分')" class="idvp-example">
                {{ ruleExample(form.matchMode, draftMatch.config, draftMatch.content, '切换到此差分') }}
              </div>
            </section>

            <section class="idvp-card">
              <div class="idvp-card__head">
                <div class="idvp-card__title">覆盖默认状态</div>
                <span class="idvp-hint">留空的项目沿用默认状态</span>
              </div>

              <div class="idvp-override">
                <div class="idvp-override__label">头像</div>
                <div class="idvp-override__body">
                  <div class="idvp-avatar-compare">
                    <AvatarVue :size="36" :border="false" v-bind="baseAvatar" class="idvp-avatar-compare__base" />
                    <span class="idvp-avatar-compare__arrow" aria-hidden="true">→</span>
                    <span class="idvp-avatar-compare__target" :class="{ 'is-inherited': !draftHasOwnAvatar }">
                      <AvatarVue :size="56" :border="false" v-bind="draftAvatar" />
                    </span>
                  </div>
                  <div class="idvp-override__actions">
                    <n-tag size="small" :bordered="false" :type="draftHasOwnAvatar ? 'success' : 'default'">
                      {{ draftHasOwnAvatar ? (avatarPreview && !form.avatarAttachmentId ? '新图片 · 保存后上传' : '已覆盖') : '继承默认头像' }}
                    </n-tag>
                    <div class="idvp-override__buttons">
                      <n-button size="small" secondary type="primary" @click="emit('avatar-upload')">
                        {{ draftHasOwnAvatar ? '更换图片' : '上传差分头像' }}
                      </n-button>
                      <n-button v-if="draftHasOwnAvatar" size="small" tertiary @click="emit('avatar-remove')">改为继承</n-button>
                    </div>
                  </div>
                </div>
              </div>

              <div class="idvp-override">
                <div class="idvp-override__label">昵称</div>
                <div class="idvp-override__body idvp-override__body--stack">
                  <n-input v-model:value="form.displayName" maxlength="32" :placeholder="baseName ? `继承：${baseName}` : '留空则沿用频道角色昵称'" />
                </div>
              </div>

              <div class="idvp-override">
                <div class="idvp-override__label">颜色</div>
                <div class="idvp-override__body">
                  <div class="idvp-color">
                    <n-color-picker
                      :value="form.color"
                      :modes="['hex']"
                      :show-alpha="false"
                      size="small"
                      class="idvp-color__picker"
                      @update:value="emit('color-picker-update', $event)"
                    />
                    <n-input
                      :value="colorDraft"
                      size="small"
                      :placeholder="baseColor ? `继承 ${baseColor}` : '#RRGGBB'"
                      class="idvp-color__input"
                      @update:value="emit('update:colorDraft', $event)"
                      @blur="emit('color-commit')"
                      @keyup.enter="emit('color-commit')"
                    />
                    <n-button v-if="form.color || colorDraft" tertiary size="small" @click="emit('color-clear')">改为继承</n-button>
                  </div>
                  <span class="idvp-preview-name" :style="draftEffectiveColor ? { color: draftEffectiveColor } : undefined">
                    {{ form.displayName || baseName || '角色名' }}
                  </span>
                </div>
              </div>

              <div class="idvp-override">
                <div class="idvp-override__label">小剧场</div>
                <div class="idvp-override__body">
                  <n-tag size="small" :bordered="false" :type="draftTheaterCustomized ? 'success' : 'default'">
                    {{ draftTheaterCustomized ? '已设置差分' : '全部继承' }}
                  </n-tag>
                  <span class="idvp-hint idvp-override__grow">各部分可独立选择继承、自定义或清除。</span>
                  <n-button size="small" secondary @click="emit('open-theater')">编辑差分演出</n-button>
                </div>
              </div>
            </section>
          </div>
          <footer class="idvp__footer">
            <n-button v-if="editingVariant" text type="error" size="small" @click="emit('delete', editingVariant)">删除差分</n-button>
            <span class="idvp__footer-state" :class="{ 'is-dirty': dirty }">
              {{ dirty ? '有未保存的修改' : (isCreating ? '' : '已保存') }}
            </span>
            <div class="idvp__footer-actions">
              <n-button size="small" :disabled="submitting || (!dirty && !isCreating)" @click="emit('revert')">
                {{ isCreating ? '取消新增' : '撤销修改' }}
              </n-button>
              <n-button size="small" type="primary" :loading="submitting" @click="emit('save')">
                保存差分
              </n-button>
            </div>
          </footer>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.idvp {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  container-type: inline-size;
  container-name: identity-variant-panel;
}

.idvp__header {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.85rem 1.1rem;
  border-bottom: 1px solid var(--idws-border);
}

.idvp__header-text {
  min-width: 0;
}

.idvp__title {
  font-size: 0.95rem;
  font-weight: 600;
}

.idvp__hint,
.idvp-hint {
  font-size: 0.76rem;
  line-height: 1.5;
  color: var(--idws-muted);
}

.idvp__unavailable {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  padding: 2rem 1.5rem;
  text-align: center;
}

.idvp__unavailable-title {
  font-weight: 600;
}

.idvp__unavailable-text {
  max-width: 360px;
  font-size: 0.82rem;
  line-height: 1.6;
  color: var(--idws-muted);
}

.idvp-stack {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.25rem;
}

.idvp-stack__ghost {
  width: 40px;
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1.5px dashed var(--idws-border-strong);
  border-radius: 12px;
  color: var(--idws-muted);
}

.idvp__body {
  flex: 1 1 auto;
  min-height: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  grid-template-rows: auto minmax(0, 1fr);
}

/* 列表：窄面板为横向滚动条，宽面板为左侧纵向列表。 */
.idvp-list {
  display: flex;
  gap: 0.4rem;
  padding: 0.6rem 0.75rem;
  overflow-x: auto;
  border-bottom: 1px solid var(--idws-border);
  scrollbar-width: thin;
}

.idvp-list__branch {
  display: flex;
  gap: 0.4rem;
  padding-left: 0.6rem;
  margin-left: 0.2rem;
  border-left: 1px solid var(--idws-border);
}

.idvp-list__empty {
  align-self: center;
  padding: 0 0.5rem;
  font-size: 0.78rem;
  color: var(--idws-muted);
  white-space: nowrap;
}

.idvp-item {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 0.55rem;
  max-width: 220px;
  padding: 0.4rem 0.6rem 0.4rem 0.4rem;
  border: 1px solid transparent;
  border-radius: 10px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.idvp-item:hover {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
}

.idvp-item.is-active {
  border-color: color-mix(in srgb, var(--idws-accent) 45%, transparent);
  background: var(--idws-accent-soft);
}

.idvp-item.is-pending {
  border-style: dashed;
  cursor: default;
}

.idvp-item:focus-visible {
  outline: 2px solid var(--idws-accent);
  outline-offset: 1px;
}

.idvp-item.is-disabled .idvp-item__avatar-wrap,
.idvp-item.is-disabled .idvp-item__name {
  opacity: 0.55;
}

.idvp-item__avatar-wrap {
  position: relative;
  flex-shrink: 0;
  border-radius: 10px;
}

.idvp-item__avatar-wrap.is-inherited :deep(img),
.idvp-avatar-compare__target.is-inherited :deep(img) {
  opacity: 0.6;
}

.idvp-item__avatar-wrap.is-inherited::after,
.idvp-avatar-compare__target.is-inherited::after {
  content: '';
  position: absolute;
  inset: -3px;
  border: 1.5px dashed var(--idws-border-strong);
  border-radius: 12px;
  pointer-events: none;
}

.idvp-item__emoji {
  position: absolute;
  right: -6px;
  bottom: -6px;
  min-width: 20px;
  height: 20px;
  padding: 0 2px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--idws-border);
  border-radius: 7px;
  background: var(--idws-card);
  font-size: 0.75rem;
  line-height: 1;
}

.idvp-item__emoji img {
  width: 14px;
  height: 14px;
  object-fit: contain;
}

.idvp-item__text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.idvp-item__name {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.86rem;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.idvp-item__flag {
  flex-shrink: 0;
  padding: 0 0.3rem;
  border-radius: 4px;
  background: color-mix(in srgb, #f59e0b 16%, transparent);
  color: color-mix(in srgb, #b45309 80%, var(--idws-text));
  font-size: 0.68rem;
  font-weight: 500;
  line-height: 1.4;
}

.idvp-item__sub {
  font-size: 0.72rem;
  color: var(--idws-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.idvp-detail {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}

.idvp-detail__scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1rem 1.1rem 1.25rem;
}

.idvp-card {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.9rem 1rem;
  border: 1px solid var(--idws-border);
  border-radius: var(--idws-radius, 12px);
  background: var(--idws-card);
}

.idvp-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.idvp-card__title {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--idws-muted);
  letter-spacing: 0.02em;
}

.idvp-card__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.idvp-reset-status {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.idvp-reset-status__dirty {
  font-size: 0.74rem;
  color: var(--idws-accent);
}

.idvp-help {
  width: 18px;
  height: 18px;
  padding: 0;
  border: 1px solid var(--idws-border-strong);
  border-radius: 50%;
  background: transparent;
  color: var(--idws-muted);
  font-size: 12px;
  line-height: 16px;
  cursor: help;
}

.idvp-help__content {
  display: grid;
  gap: 0.55rem;
  line-height: 1.6;
}

.idvp-base {
  flex-direction: row;
  align-items: center;
  gap: 0.8rem;
  padding: 0.75rem 0.9rem;
  background: var(--idws-pane);
}

.idvp-base__text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.2rem;
  min-width: 0;
}

.idvp-base__eyebrow {
  font-size: 0.72rem;
  color: var(--idws-muted);
}

.idvp-base__name {
  font-size: 1rem;
  font-weight: 600;
}

.idvp-match {
  display: grid;
  grid-template-columns: minmax(0, 35fr) minmax(0, 65fr);
  gap: 0.75rem;
}

.idvp-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 0;
}

.idvp-field__label {
  font-size: 0.78rem;
  color: var(--idws-muted);
}

.idvp-example {
  padding: 0.45rem 0.65rem;
  border-radius: 8px;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
  font-size: 0.76rem;
  color: var(--idws-muted);
}

.idvp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.45rem;
  padding: 1.25rem 1rem;
  border: 1.5px dashed var(--idws-border-strong);
  border-radius: var(--idws-radius, 12px);
  text-align: center;
}

.idvp-empty__title {
  font-weight: 600;
}

.idvp-head {
  flex-direction: row;
  align-items: center;
  gap: 0.85rem;
}

.idvp-head__emoji {
  flex-shrink: 0;
  width: 52px;
  height: 52px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--idws-border-strong);
  border-radius: 14px;
  background: var(--idws-pane);
  color: inherit;
  font-size: 1.6rem;
  cursor: pointer;
  overflow: hidden;
}

.idvp-head__emoji:hover {
  border-color: var(--idws-accent);
}

.idvp-head__emoji:focus-visible {
  outline: 2px solid var(--idws-accent);
  outline-offset: 1px;
}

.idvp-head__emoji img {
  width: 30px;
  height: 30px;
  object-fit: contain;
}

.idvp-head__emoji-empty {
  font-size: 0.68rem;
  line-height: 1.25;
  color: var(--idws-muted);
}

.idvp-head__main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.idvp-head__meta {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  font-size: 0.74rem;
  color: var(--idws-muted);
}

.idvp-override {
  display: grid;
  grid-template-columns: 4rem minmax(0, 1fr);
  align-items: center;
  gap: 0.75rem;
}

.idvp-override + .idvp-override {
  padding-top: 0.75rem;
  border-top: 1px solid var(--idws-border);
}

.idvp-override__label {
  font-size: 0.82rem;
  color: var(--idws-muted);
}

.idvp-override__body {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.6rem 0.9rem;
  min-width: 0;
}

.idvp-override__body--stack {
  display: block;
}

.idvp-override__grow {
  flex: 1 1 8rem;
}

.idvp-override__actions {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.4rem;
}

.idvp-override__buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.idvp-avatar-compare {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.idvp-avatar-compare__base {
  opacity: 0.75;
}

.idvp-avatar-compare__arrow {
  color: var(--idws-muted);
}

.idvp-avatar-compare__target {
  position: relative;
  border-radius: 12px;
}

.idvp-color {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.idvp-color__picker {
  width: 36px;
  height: 32px;
  flex-shrink: 0;
}

.idvp-color__picker :deep(.n-color-picker-trigger) {
  padding: 0;
  border-radius: 8px;
  justify-content: center;
}

.idvp-color__picker :deep(.n-color-picker-trigger__value) {
  display: none;
}

.idvp-color__input {
  width: 128px;
}

.idvp-preview-name {
  font-size: 0.86rem;
  font-weight: 600;
}

.idvp__footer {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.7rem 1.1rem;
  border-top: 1px solid var(--idws-border);
  background: var(--idws-surface);
}

.idvp__footer-state {
  flex: 1 1 auto;
  font-size: 0.76rem;
  color: var(--idws-muted);
}

.idvp__footer-state.is-dirty {
  color: var(--idws-accent);
}

.idvp__footer-actions {
  display: flex;
  gap: 0.5rem;
}

/* 差分面板足够宽时，列表转为左侧纵向主从结构。 */
@container identity-variant-panel (min-width: 600px) {
  .idvp__body {
    grid-template-columns: minmax(200px, 240px) minmax(0, 1fr);
    grid-template-rows: minmax(0, 1fr);
  }

  .idvp-list {
    flex-direction: column;
    overflow-x: hidden;
    overflow-y: auto;
    padding: 0.75rem 0.6rem;
    border-bottom: 0;
    border-right: 1px solid var(--idws-border);
  }

  .idvp-list__branch {
    flex-direction: column;
    margin-left: 1.45rem;
    padding-left: 0.5rem;
    padding-top: 0.25rem;
  }

  .idvp-list__empty {
    align-self: flex-start;
    padding: 0.4rem 0.5rem;
  }

  .idvp-item {
    max-width: none;
    width: 100%;
  }
}

/* 宽工作区里角色资料已并排显示，无需跳转按钮。 */
@container identity-workspace (min-width: 860px) {
  .idvp-base__goto {
    display: none;
  }
}

@container identity-variant-panel (max-width: 420px) {
  .idvp-match {
    grid-template-columns: minmax(0, 1fr);
  }

  .idvp-override {
    grid-template-columns: minmax(0, 1fr);
    gap: 0.4rem;
  }

  .idvp-card__foot {
    flex-direction: column;
    align-items: stretch;
  }

  .idvp__footer {
    flex-wrap: wrap;
  }
}

@media (max-width: 640px) {
  .idvp__header,
  .idvp-detail__scroll {
    padding-left: 0.75rem;
    padding-right: 0.75rem;
  }

  .idvp__footer {
    padding: 0.6rem 0.75rem calc(0.6rem + env(safe-area-inset-bottom, 0px));
  }
}
</style>
