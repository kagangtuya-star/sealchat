<script setup lang="ts">
// 当前 LOG 的样式：浏览、预览、使用 / 更新样式，调整本 LOG 的设计变量覆盖，并进入 Style Studio。
// 样式本身的设计（模板、CSS、资源、版本）在 Style Studio 中完成；使用某个版本是单独的绑定动作。
import { computed, shallowRef } from 'vue';
import { useMessage } from 'naive-ui';
import { Palette } from '@vicons/tabler';
import { describeLogError, exportLogStyleZip, fetchLogStylePackage } from '@/services/log/logApi';
import type { LogDraft, LogStylePackage, LogStyleRule, LogStyleSummary, LogTokenValue } from '@/types/log';
import { builtinStyleInfo } from '../logStyleCatalog';
import type { StyleStudioRequest } from '../logStyleWorkingCopy';
import type { LogStyleLibrary } from '../composables/useLogStyleLibrary';
import LogStyleSketch from './LogStyleSketch.vue';

const props = defineProps<{
  draft: LogDraft;
  update: (mutate: (d: LogDraft) => void) => boolean;
  stylePackage: LogStylePackage | null;
  canEdit: boolean;
  library: LogStyleLibrary;
  /** 让当前 LOG 使用某个样式版本（工作台负责兼容检查与草稿写入）。 */
  bind: (pkg: LogStylePackage) => Promise<boolean>;
}>();
const emit = defineEmits<{
  (e: 'preview', pkg: LogStylePackage | null): void;
  (e: 'open-studio', request: StyleStudioRequest): void;
}>();

const toast = useMessage();
const HEX = /^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/;

const styles = computed(() => props.library.items.value);
const ownership = computed(() => (props.stylePackage ? props.library.ownership(props.stylePackage.id) : 'unknown'));
const builtinStyles = computed(() => props.library.builtin.value);
const ownStyles = computed(() => props.library.mine.value);
const currentInfo = computed(() => (props.stylePackage ? builtinStyleInfo(props.stylePackage.id) : null));
const currentLatest = computed(() => {
  const pkg = props.stylePackage;
  if (!pkg) return null;
  const s = styles.value.find((x) => x.id === pkg.id);
  return s && s.version !== pkg.version ? s : null;
});
// 已从样式库下线的旧内置样式：仍可读取、复制与导出，但不再出现在列表里。
const currentRetired = computed(() => {
  const pkg = props.stylePackage;
  return !!pkg && props.library.loaded.value && !props.library.loading.value && !props.library.error.value && !props.library.truncated.value
    && pkg.id.startsWith('sealchat-') && !styles.value.some((s) => s.id === pkg.id);
});
const isCurrent = (s: LogStyleSummary) => props.stylePackage?.id === s.id && props.stylePackage?.version === s.version;
const useLabel = (s: LogStyleSummary) => (isCurrent(s) ? '使用中' : props.stylePackage?.id === s.id ? `更新到 ${s.version}` : '使用');

// ---- 预览与绑定 ----
const previewing = shallowRef<LogStylePackage | null>(null);

const previewStyle = async (s: LogStyleSummary) => {
  try {
    const pkg = await fetchLogStylePackage(s.id, s.version);
    previewing.value = pkg;
    emit('preview', pkg);
  } catch (error) {
    toast.error(describeLogError(error, '读取样式失败'));
  }
};
const cancelPreview = () => {
  previewing.value = null;
  emit('preview', null);
};
const applyPackage = async (pkg: LogStylePackage) => {
  if (!(await props.bind(pkg))) return;
  if (previewing.value) cancelPreview();
};
const useStyle = async (s: LogStyleSummary) => {
  try {
    await applyPackage(await fetchLogStylePackage(s.id, s.version));
  } catch (error) {
    toast.error(describeLogError(error, '读取样式失败'));
  }
};

// ---- 设计变量（本 LOG 的覆盖值，不修改样式包） ----
const tokenEntries = computed(() => Object.entries(props.stylePackage?.tokens || {}));
const tokenValue = (k: string, base: LogTokenValue) => (k in props.draft.themeTokens ? props.draft.themeTokens[k] : base);
const setToken = (k: string, v: LogTokenValue | null) => props.update((d) => {
  if (v === null || v === undefined || (typeof v === 'string' && /[;{}\\<>@()]/.test(v))) return;
  d.themeTokens = { ...d.themeTokens, [k]: v };
});
const resetToken = (k: string) => props.update((d) => {
  const next = { ...d.themeTokens };
  delete next[k];
  d.themeTokens = next;
});

const ruleText = (r: LogStyleRule) => [r.messageType && `类型=${r.messageType}`, r.identityKey && `角色=${r.identityKey}`, r.variantKey && `差分=${r.variantKey}`].filter(Boolean).join('，') || '任意消息';

// ---- Style Studio 入口 ----
const editCurrent = () => {
  const pkg = props.stylePackage;
  if (!pkg) return;
  if (ownership.value === 'foreign') emit('open-studio', { kind: 'fork', id: pkg.id, version: pkg.version, name: pkg.name });
  else emit('open-studio', { kind: 'open', id: pkg.id, version: pkg.version });
};

const exportCurrent = async () => {
  const pkg = props.stylePackage;
  if (!pkg) return;
  try {
    const blob = await exportLogStyleZip(pkg.id, pkg.version);
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${pkg.id}-${pkg.version}.zip`;
    a.rel = 'noopener';
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (error) {
    toast.error(describeLogError(error, '导出失败'));
  }
};
</script>

<template>
  <div class="log-style">
    <section>
      <div class="log-style__title">当前样式</div>
      <div v-if="stylePackage" class="log-style__current">
        <div class="log-style__current-head">
          <LogStyleSketch :kind="currentInfo?.sketch || 'generic'" />
          <div class="log-style__current-text">
            <div><strong>{{ stylePackage.name }}</strong><span v-if="currentInfo" class="log-style__cat">{{ currentInfo.category }}</span></div>
            <span class="log-style__meta">{{ stylePackage.id }} @ {{ stylePackage.version }} · {{ stylePackage.license || '未声明许可' }}</span>
            <p v-if="currentInfo" class="log-style__summary">{{ currentInfo.summary }}</p>
          </div>
        </div>
        <div v-if="currentLatest" class="log-style__notice">
          样式库中已有 {{ currentLatest.version }}；当前工程仍固定使用 {{ stylePackage.version }}。
          <n-button size="tiny" text type="primary" :disabled="!canEdit" @click="useStyle(currentLatest)">更新</n-button>
          <n-button size="tiny" text @click="previewStyle(currentLatest)">先预览</n-button>
        </div>
        <div v-else-if="currentRetired" class="log-style__notice">
          这是已从样式库下线的旧版内置样式，可以继续使用、复制或导出，也可以从下方挑选新的排版。
        </div>
        <n-space size="small">
          <n-button size="tiny" type="primary" secondary @click="editCurrent">{{ ownership === 'foreign' ? '复制后编辑' : '在样式设计器中编辑' }}</n-button>
          <n-button size="tiny" @click="exportCurrent">导出 ZIP</n-button>
        </n-space>
      </div>
      <n-spin v-else size="small" />
    </section>

    <n-alert v-if="previewing" type="info" :show-icon="false">
      正在预览：{{ previewing.name }} {{ previewing.version }}（未应用）
      <n-space size="small" style="margin-top: 6px">
        <n-button size="tiny" type="primary" :disabled="!canEdit" @click="applyPackage(previewing)">应用到本工程</n-button>
        <n-button size="tiny" @click="cancelPreview">取消预览</n-button>
      </n-space>
    </n-alert>

    <button type="button" class="log-style__studio" @click="emit('open-studio', { kind: 'library' })">
      <n-icon :component="Palette" size="20" />
      <span>
        <strong>样式设计器</strong>
        <span>管理样式库：新建、复制、导入 ZIP，编辑模板、CSS、变量、规则与资源，保存为新版本</span>
      </span>
    </button>

    <section v-if="tokenEntries.length">
      <div class="log-style__title">设计变量</div>
      <div v-for="[k, base] in tokenEntries" :key="k" class="log-style__token">
        <label :for="`tok-${k}`">{{ k }}</label>
        <n-switch v-if="typeof base === 'boolean'" :value="tokenValue(k, base) === true" :disabled="!canEdit" @update:value="(v: boolean) => setToken(k, v)" />
        <n-input-number v-else-if="typeof base === 'number'" :input-props="{ id: `tok-${k}` }" size="small" :value="tokenValue(k, base) as number" :disabled="!canEdit" @update:value="(v: number | null) => setToken(k, v)" />
        <n-color-picker v-else-if="HEX.test(String(base))" size="small" :modes="['hex']" :value="String(tokenValue(k, base))" :disabled="!canEdit" @update:value="(v: string) => setToken(k, v)" />
        <n-input v-else :input-props="{ id: `tok-${k}` }" size="small" :value="String(tokenValue(k, base))" maxlength="200" :disabled="!canEdit" @update:value="(v: string) => setToken(k, v)" />
        <n-button size="tiny" quaternary :disabled="!canEdit || !(k in draft.themeTokens)" @click="resetToken(k)">还原</n-button>
      </div>
    </section>

    <section>
      <div class="log-style__title">消息模板规则（按顺序首个匹配）</div>
      <ol class="log-style__rules">
        <li v-for="(r, i) in stylePackage?.rules || []" :key="i">{{ ruleText(r) }} → <code>{{ r.templateId }}</code></li>
      </ol>
      <p class="log-style__hint">“LOG 显示规则”只影响排版，不写原消息；要修改样式自带的规则请在样式设计器中编辑。</p>
    </section>

    <section>
      <div class="log-style__title">样式库 · 内置排版</div>
      <p class="log-style__hint log-style__hint--lead">每套内置样式是一种不同的版面结构；配色、字体、间距等细节在上方“设计变量”里调整。</p>
      <n-alert v-if="library.error.value" type="error" :show-icon="false">{{ library.error.value }}</n-alert>
      <div class="log-style__lib">
        <article
          v-for="s in builtinStyles"
          :key="s.id"
          class="log-style__card"
          :class="{ 'is-current': stylePackage?.id === s.id, 'is-previewing': previewing?.id === s.id && previewing?.version === s.version }"
        >
          <LogStyleSketch :kind="builtinStyleInfo(s.id)?.sketch || 'generic'" />
          <div class="log-style__card-body">
            <div class="log-style__card-head">
              <strong>{{ s.name }}</strong>
              <span class="log-style__cat">{{ builtinStyleInfo(s.id)?.category || '内置' }}</span>
            </div>
            <p v-if="builtinStyleInfo(s.id)" class="log-style__summary">{{ builtinStyleInfo(s.id)!.summary }}</p>
            <div v-if="builtinStyleInfo(s.id)" class="log-style__scenes">适合：{{ builtinStyleInfo(s.id)!.scenes.join(' · ') }}</div>
            <div class="log-style__card-foot">
              <span class="log-style__meta">v{{ s.version }}</span>
              <n-space size="small">
                <n-button size="tiny" @click="previewStyle(s)">预览</n-button>
                <n-button size="tiny" :type="isCurrent(s) ? 'default' : 'primary'" secondary :disabled="!canEdit || isCurrent(s)" @click="useStyle(s)">{{ useLabel(s) }}</n-button>
              </n-space>
            </div>
          </div>
        </article>
      </div>
      <template v-if="ownStyles.length">
        <div class="log-style__subtitle">我的样式</div>
        <div v-for="s in ownStyles" :key="s.id" class="log-style__row" :class="{ 'is-current': stylePackage?.id === s.id }">
          <div class="log-style__row-name">
            {{ s.name }}
            <span class="log-style__meta">{{ s.version }}</span>
          </div>
          <n-space size="small">
            <n-button size="tiny" quaternary @click="emit('open-studio', { kind: 'open', id: s.id, version: s.version })">编辑</n-button>
            <n-button size="tiny" @click="previewStyle(s)">预览</n-button>
            <n-button size="tiny" :disabled="!canEdit || isCurrent(s)" @click="useStyle(s)">{{ useLabel(s) }}</n-button>
          </n-space>
        </div>
      </template>
      <n-spin v-if="library.loading.value && !library.loaded.value" size="small" />
    </section>
  </div>
</template>

<style scoped>
.log-style {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 8px 0 16px;
}

.log-style > section + section {
  padding-top: 14px;
  border-top: 1px solid var(--sc-border-mute, #e5e5e5);
}

.log-style__title {
  font-weight: 600;
  font-size: 12px;
  letter-spacing: 0.02em;
  color: var(--sc-text-secondary, #666);
  margin-bottom: 8px;
}

.log-style__subtitle {
  font-weight: 600;
  font-size: 12px;
  margin: 8px 0 4px;
}

.log-style__current {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--sc-border-strong, #e5e5e5);
  border-radius: 8px;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
}

.log-style__meta {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  margin-left: 4px;
}

.log-style__hint {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.log-style__token {
  display: grid;
  grid-template-columns: 110px 1fr auto;
  gap: 6px;
  align-items: center;
  margin-bottom: 4px;
  font-size: 12px;
}

.log-style__token label {
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-style__rules {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
}

.log-style__current-head {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.log-style__current-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.log-style__current-text .log-style__meta {
  margin-left: 0;
  overflow-wrap: anywhere;
}

.log-style__notice {
  font-size: 12px;
  line-height: 1.5;
  color: var(--sc-text-secondary, #666);
}

.log-style__cat {
  display: inline-block;
  margin-left: 6px;
  padding: 0 6px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 18px;
  color: var(--sc-text-secondary, #666);
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
  vertical-align: 1px;
}

.log-style__summary {
  margin: 2px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--sc-text-primary, inherit);
}

.log-style__hint--lead {
  margin: -4px 0 8px;
}

.log-style__lib {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.log-style__card {
  display: flex;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--sc-border-mute, #e5e5e5);
  border-radius: 8px;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.log-style__card:hover {
  border-color: var(--sc-border-strong, #cfcfcf);
}

.log-style__card.is-previewing {
  border-color: #2f6fde;
  border-style: dashed;
}

.log-style__card.is-current {
  border-color: #2f6fde;
  background: rgba(47, 111, 222, 0.06);
}

.log-style__card-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.log-style__card-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px 0;
  font-size: 13px;
}

.log-style__scenes {
  font-size: 11px;
  line-height: 1.5;
  color: var(--sc-text-secondary, #666);
}

.log-style__card-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  margin-top: 4px;
}

.log-style__card-foot .log-style__meta {
  margin-left: 0;
}

.log-style__row.is-current {
  font-weight: 600;
}

.log-style__row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
  padding: 4px 0;
  border-bottom: 1px solid var(--sc-border-mute, #eee);
  font-size: 13px;
}

.log-style__studio {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 12px;
  border: 1px dashed color-mix(in srgb, var(--primary-color, #2f6fde) 45%, transparent);
  border-radius: 8px;
  background: color-mix(in srgb, var(--primary-color, #2f6fde) 6%, transparent);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.log-style__studio:hover {
  border-style: solid;
}

.log-style__studio:focus-visible {
  outline: 2px solid var(--primary-color, #2f6fde);
  outline-offset: 2px;
}

.log-style__studio > .n-icon {
  flex: none;
  color: var(--primary-color, #2f6fde);
}

.log-style__studio > span {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.log-style__studio strong {
  font-size: 13px;
}

.log-style__studio > span > span {
  font-size: 12px;
  line-height: 1.5;
  color: var(--sc-text-secondary, #666);
}
</style>
