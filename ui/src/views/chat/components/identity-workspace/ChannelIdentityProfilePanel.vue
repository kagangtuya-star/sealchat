<script setup lang="ts">
import { computed } from 'vue';
import AvatarVue from '@/components/avatar.vue';
import type { GeneratedAvatarStyle } from '@/utils/generatedAvatarImage';
import type {
  IdentityGeneratedAvatarStyleOption,
  IdentityProfileDraft,
  IdentityWorkspaceAvatar,
} from './identityWorkspaceTypes';

// 「角色资料」编辑域。`form` 是 chat.vue 持有的角色草稿，本组件只做就地编辑；
// 上传、保存、确认弹窗等带副作用的操作全部通过事件交回 chat.vue 原有链路。
const props = withDefaults(defineProps<{
  form: IdentityProfileDraft;
  colorDraft: string;
  mode: 'create' | 'edit';
  avatar: IdentityWorkspaceAvatar;
  avatarPending?: boolean;
  generatedStyle: GeneratedAvatarStyle;
  generatedStyleOptions?: IdentityGeneratedAvatarStyleOption[];
  showGeneratedStyles?: boolean;
  characterCardOptions?: Array<{ label: string; value: string }>;
  managingBot?: boolean;
  managingOtherUser?: boolean;
  delegatedShared?: boolean;
  sharedFieldsDisabled?: boolean;
  editingTemporary?: boolean;
  sharedIdentity?: boolean;
  canPromoteShared?: boolean;
  submitting?: boolean;
  submitText: string;
  chatSkinConfigured?: boolean;
}>(), {
  avatarPending: false,
  generatedStyleOptions: () => [],
  showGeneratedStyles: false,
  characterCardOptions: () => [],
  managingBot: false,
  managingOtherUser: false,
  delegatedShared: false,
  sharedFieldsDisabled: false,
  editingTemporary: false,
  sharedIdentity: false,
  canPromoteShared: false,
  submitting: false,
  chatSkinConfigured: false,
});

const emit = defineEmits<{
  (e: 'update:colorDraft', value: string): void;
  (e: 'color-picker-update', value: string | null): void;
  (e: 'color-commit'): void;
  (e: 'color-clear'): void;
  (e: 'avatar-upload'): void;
  (e: 'avatar-text'): void;
  (e: 'avatar-remove'): void;
  (e: 'avatar-style', style: GeneratedAvatarStyle): void;
  (e: 'promote-shared', checked: boolean): void;
  (e: 'open-decoration'): void;
  (e: 'open-theater'): void;
  (e: 'open-chat-skin'): void;
  (e: 'submit'): void;
}>();

const hasAvatarContent = computed(() => Boolean(props.form.avatarAttachmentId || props.avatarPending || props.form.avatarTextStyle));
const avatarSourceLabel = computed(() => {
  const { form } = props;
  if (props.managingBot && form.botAppearanceMode !== 'custom') return '跟随 BOT 全局资料';
  if (props.avatarPending) return '新图片 · 保存后上传';
  if (form.isTemporary) return '临时角色 · 保存时按昵称生成';
  if (form.avatarTextStyle) return '文字头像';
  if (form.avatarAttachmentId) return '图片头像';
  return props.managingOtherUser ? '未设置 · 使用该成员头像' : '未设置 · 使用账号头像';
});
const hasDecorations = computed(() => props.form.avatarDecorations.some(item => item.enabled && item.resourceAttachmentId));
const showCharacterCard = computed(() => !props.editingTemporary && !props.managingBot);
const showSharedSection = computed(() => (
  props.mode === 'edit' && !props.editingTemporary && !props.managingBot && !props.managingOtherUser
));
const activateModeLabel = computed(() => (props.form.icOocOnActivate === 'ooc' ? '场外' : '场内'));

const updateBotInherit = (inherit: boolean) => {
  props.form.botAppearanceMode = inherit ? 'inherit' : 'custom';
};
</script>

<template>
  <div class="idpp">
    <div class="idpp__scroll">
      <section v-if="managingBot" class="idpp-card idpp-card--banner">
        <div class="idpp-row">
          <div class="idpp-row__text">
            <div class="idpp-row__title">资料来源</div>
            <div class="idpp-hint">BOT 角色设置会在当前世界的所有频道同步；“跟随”仅控制昵称、颜色、头像来源。</div>
          </div>
          <n-switch :value="form.botAppearanceMode !== 'custom'" @update:value="updateBotInherit">
            <template #checked>跟随 BOT 全局资料</template>
            <template #unchecked>使用自定义资料</template>
          </n-switch>
        </div>
      </section>

      <section class="idpp-card idpp-hero">
        <div class="idpp-hero__avatar">
          <AvatarVue :size="88" :border="false" v-bind="avatar" />
        </div>
        <div class="idpp-hero__main">
          <div class="idpp-hero__caption">{{ avatarSourceLabel }}</div>
          <div class="idpp-hero__actions">
            <n-button size="small" type="primary" secondary :disabled="sharedFieldsDisabled" @click="emit('avatar-upload')">上传图片</n-button>
            <n-button
              size="small"
              :type="form.avatarTextStyle ? 'primary' : 'default'"
              secondary
              :disabled="sharedFieldsDisabled"
              @click="emit('avatar-text')"
            >
              文字头像
            </n-button>
            <n-button v-if="hasAvatarContent" size="small" tertiary :disabled="sharedFieldsDisabled" @click="emit('avatar-remove')">移除</n-button>
          </div>
          <div
            v-if="showGeneratedStyles"
            class="idpp-style-picker"
            role="radiogroup"
            aria-label="文字头像样式"
          >
            <n-tooltip v-for="option in generatedStyleOptions" :key="option.style">
              <template #trigger>
                <button
                  type="button"
                  role="radio"
                  class="idpp-style-picker__item"
                  :class="{ 'is-active': generatedStyle === option.style }"
                  :aria-checked="generatedStyle === option.style"
                  :aria-label="option.label"
                  :disabled="sharedFieldsDisabled"
                  @click="emit('avatar-style', option.style)"
                >
                  <img :src="option.src" alt="" draggable="false" />
                </button>
              </template>
              {{ option.label }}
            </n-tooltip>
          </div>
        </div>
      </section>

      <section class="idpp-card">
        <div class="idpp-card__title">基础资料</div>
        <div class="idpp-field">
          <div class="idpp-field__label">频道昵称</div>
          <n-input
            v-model:value="form.displayName"
            :disabled="sharedFieldsDisabled"
            maxlength="32"
            show-count
            placeholder="请输入频道内显示的昵称"
          />
        </div>
        <div class="idpp-field">
          <div class="idpp-field__label">昵称颜色</div>
          <div class="idpp-color">
            <n-color-picker
              :value="form.color"
              :modes="['hex']"
              :show-alpha="false"
              size="small"
              class="idpp-color__picker"
              :disabled="sharedFieldsDisabled"
              @update:value="emit('color-picker-update', $event)"
            />
            <n-input
              :value="colorDraft"
              size="small"
              placeholder="#RRGGBB"
              class="idpp-color__input"
              :disabled="sharedFieldsDisabled"
              @update:value="emit('update:colorDraft', $event)"
              @blur="emit('color-commit')"
              @keyup.enter="emit('color-commit')"
            />
            <n-button tertiary size="small" :disabled="sharedFieldsDisabled" @click="emit('color-clear')">清除</n-button>
          </div>
        </div>
      </section>

      <section class="idpp-card">
        <div class="idpp-card__title">外观扩展</div>
        <div class="idpp-row">
          <div class="idpp-row__text">
            <div class="idpp-row__title">
              头像装饰
              <n-tag v-if="hasDecorations" size="small" :bordered="false" type="success">已配置</n-tag>
              <span v-else class="idpp-row__state">未配置</span>
            </div>
            <div class="idpp-hint">
              {{ sharedIdentity || form.promoteToShared
                ? '会同步到此角色的其他频道副本，并且只显示在频道消息头像上。'
                : '仅对当前频道角色生效，并且只会显示在频道消息头像上。' }}
            </div>
          </div>
          <n-button size="small" secondary :disabled="delegatedShared" @click="emit('open-decoration')">编辑装饰</n-button>
        </div>
        <div class="idpp-row">
          <div class="idpp-row__text">
            <div class="idpp-row__title">
              小剧场演出
              <n-tag v-if="form.theaterPresentation" size="small" :bordered="false" type="success">已配置</n-tag>
              <span v-else class="idpp-row__state">未配置</span>
            </div>
            <div class="idpp-hint">仅用于小剧场消息演出，不影响频道消息头像。</div>
          </div>
          <n-button size="small" secondary :disabled="delegatedShared" @click="emit('open-theater')">编辑演出</n-button>
        </div>
        <div class="idpp-row">
          <div class="idpp-row__text">
            <div class="idpp-row__title">
              聊天装扮
              <n-tag v-if="chatSkinConfigured" size="small" :bordered="false" type="success">已配置</n-tag>
              <span v-else class="idpp-row__state">未配置</span>
            </div>
            <div class="idpp-hint">
              {{ mode === 'create'
                ? '保存角色后可设置消息气泡与紧凑行的外观。'
                : '定制此角色消息的气泡 / 紧凑外观；只影响之后发送的消息，历史消息保持原样。' }}
            </div>
          </div>
          <n-button size="small" secondary :disabled="mode === 'create'" @click="emit('open-chat-skin')">编辑装扮</n-button>
        </div>
      </section>

      <n-alert v-if="delegatedShared" type="info" :show-icon="false" class="idpp-alert">
        此角色由本人跨频道共享。管理员可维护会同步到全部副本的头像差分；昵称、颜色、头像、头像装饰及基础小剧场演出只读。当前频道默认身份、文件夹、IC/OOC 映射和人物卡保持独立。
      </n-alert>

      <section v-if="showCharacterCard || !managingBot" class="idpp-card">
        <div class="idpp-card__title">频道行为</div>
        <div v-if="showCharacterCard" class="idpp-field">
          <div class="idpp-field__label">绑定人物卡</div>
          <n-select
            v-model:value="form.characterCardId"
            :options="characterCardOptions"
            placeholder="选择要绑定的人物卡"
            clearable
          />
        </div>
        <div v-if="!managingBot" class="idpp-field">
          <div class="idpp-field__label">切换到此角色时</div>
          <div class="idpp-mode">
            <n-button-group size="small">
              <n-button :type="form.icOocOnActivate === 'ic' ? 'primary' : 'default'" @click="form.icOocOnActivate = 'ic'">场内</n-button>
              <n-button :type="form.icOocOnActivate === 'ooc' ? 'primary' : 'default'" @click="form.icOocOnActivate = 'ooc'">场外</n-button>
            </n-button-group>
            <span class="idpp-hint">自动切到{{ activateModeLabel }}（需开启场内场外自动切换）</span>
          </div>
        </div>
        <div v-if="showCharacterCard" class="idpp-check">
          <n-checkbox v-model:checked="form.isDefault">设为频道默认身份</n-checkbox>
        </div>
        <div v-if="mode === 'create' && !managingBot" class="idpp-check">
          <n-checkbox v-model:checked="form.isTemporary">创建为临时 NPC 角色</n-checkbox>
        </div>
      </section>

      <section v-if="showSharedSection && (sharedIdentity || canPromoteShared)" class="idpp-card">
        <div class="idpp-card__title">跨频道角色</div>
        <n-tag v-if="sharedIdentity" type="success" size="small" :bordered="false">已启用跨频道同步</n-tag>
        <n-checkbox
          v-else-if="canPromoteShared"
          :checked="form.promoteToShared"
          @update:checked="emit('promote-shared', $event)"
        >
          提升为跨频道角色
          <n-tag size="small" type="warning" :bordered="false">实验性</n-tag>
        </n-checkbox>
      </section>
    </div>
    <footer class="idpp__footer">
      <span class="idpp__footer-hint">
        {{ mode === 'create' ? '创建后可继续配置头像差分' : '角色资料与头像差分分别保存' }}
      </span>
      <n-button type="primary" :loading="submitting" @click="emit('submit')">{{ submitText }}</n-button>
    </footer>
  </div>
</template>

<style scoped>
.idpp {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.idpp__scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1rem 1.1rem 1.25rem;
}

.idpp-card {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.9rem 1rem;
  border: 1px solid var(--idws-border);
  border-radius: var(--idws-radius, 12px);
  background: var(--idws-card);
}

.idpp-card--banner {
  border-color: color-mix(in srgb, var(--idws-accent) 30%, var(--idws-border));
  background: color-mix(in srgb, var(--idws-accent) 6%, var(--idws-card));
}

.idpp-card__title {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--idws-muted);
  letter-spacing: 0.02em;
}

.idpp-hero {
  flex-direction: row;
  align-items: center;
  gap: 1rem;
}

.idpp-hero__avatar {
  flex-shrink: 0;
  padding: 4px;
  border-radius: 22px;
  background: var(--idws-pane);
}

.idpp-hero__main {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  min-width: 0;
}

.idpp-hero__caption {
  font-size: 0.78rem;
  color: var(--idws-muted);
}

.idpp-hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.idpp-style-picker {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
}

.idpp-style-picker__item {
  width: 32px;
  height: 32px;
  padding: 2px;
  border: 1.5px solid transparent;
  border-radius: 9px;
  background: transparent;
  cursor: pointer;
  transition: border-color 0.15s ease;
}

.idpp-style-picker__item img {
  display: block;
  width: 100%;
  height: 100%;
  border-radius: 6px;
  pointer-events: none;
}

.idpp-style-picker__item:hover:not(:disabled) {
  border-color: var(--idws-border-strong);
}

.idpp-style-picker__item.is-active {
  border-color: var(--idws-accent);
}

.idpp-style-picker__item:focus-visible {
  outline: 2px solid var(--idws-accent);
  outline-offset: 1px;
}

.idpp-style-picker__item:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.idpp-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.idpp-field__label {
  font-size: 0.82rem;
  color: var(--idws-muted);
}

.idpp-color {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.idpp-color__picker {
  width: 36px;
  height: 32px;
  flex-shrink: 0;
}

.idpp-color__picker :deep(.n-color-picker-trigger) {
  padding: 0;
  border-radius: 8px;
  justify-content: center;
}

.idpp-color__picker :deep(.n-color-picker-trigger__value) {
  display: none;
}

.idpp-color__input {
  width: 120px;
}

.idpp-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.idpp-row + .idpp-row {
  padding-top: 0.75rem;
  border-top: 1px solid var(--idws-border);
}

.idpp-row__text {
  min-width: 0;
}

.idpp-row__title {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.9rem;
  font-weight: 500;
}

.idpp-row__state {
  font-size: 0.75rem;
  font-weight: 400;
  color: var(--idws-muted);
}

.idpp-hint {
  margin-top: 0.15rem;
  font-size: 0.76rem;
  line-height: 1.5;
  color: var(--idws-muted);
}

.idpp-mode {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.6rem;
}

.idpp-mode .idpp-hint {
  margin-top: 0;
}

.idpp-alert {
  border-radius: var(--idws-radius, 12px);
}

.idpp__footer {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.7rem 1.1rem;
  border-top: 1px solid var(--idws-border);
  background: var(--idws-surface);
}

.idpp__footer-hint {
  font-size: 0.76rem;
  color: var(--idws-muted);
}

@media (max-width: 640px) {
  .idpp__scroll {
    padding: 0.75rem 0.75rem 1rem;
  }

  .idpp__footer {
    padding: 0.6rem 0.75rem calc(0.6rem + env(safe-area-inset-bottom, 0px));
  }

  .idpp-hero {
    gap: 0.75rem;
  }
}
</style>
