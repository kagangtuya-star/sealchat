<script setup lang="ts">
// 批量编辑。三个模式语义不同：
// 1) 历史消息外观：写回原消息快照（preview → 确认 → apply → 轮询持久任务）；
// 2) 角色资料：在聊天的角色管理中修改，只影响角色本身；
// 3) LOG 显示规则：只改工程草稿，不写样式包或原消息。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useRouter } from 'vue-router';
import {
  describeLogError,
  fetchChannelIdentitiesForLog,
  fetchMutationJob,
  fetchMutationJobItems,
  isLogError,
  type LogIdentityVariantOption,
} from '@/services/log/logApi';
import { applyAppearanceBatch, newClientOpId, previewAppearanceBatch } from '@/services/log/logMessageCommands';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import type { LogAppearancePatch, LogAppearancePreview, LogDesignMessage, LogDisplayRule, LogDraft, LogMutationJob, LogMutationJobItem, LogSection } from '@/types/log';
import { LOG_MESSAGE_TYPES } from '@/services/log/logTemplateCompiler';
import { cloneJson, uuid } from '../logDraftOps';

const props = defineProps<{
  projectId: string;
  worldId: string;
  section: LogSection | null;
  messages: LogDesignMessage[];
  canBulk: boolean;
  draft: LogDraft;
  update: (mutate: (next: LogDraft) => void) => boolean;
  canEditDraft: boolean;
  templateIds: string[];
}>();
const emit = defineEmits<{
  (e: 'applied'): void;
}>();

const toast = useMessage();
const router = useRouter();
const mode = ref<'appearance' | 'profile' | 'rules'>('appearance');
const action = ref<'snapshot' | 'variant'>('snapshot');
const HEX = /^#[0-9a-fA-F]{6}$/;
const JOB_KEY = computed(() => `sealchat.log.bulkJob.${props.projectId}`);

// ---- 选择范围 ----
interface SpeakerOption { key: string; kind: 'identity' | 'shared'; id: string; label: string; userId: string }
const speakers = computed<SpeakerOption[]>(() => {
  const map = new Map<string, SpeakerOption>();
  for (const it of props.messages) {
    const idn = it.message.identity;
    if (!idn?.id) continue;
    const name = idn.displayName || idn.id;
    if (!map.has(`i:${idn.id}`)) map.set(`i:${idn.id}`, { key: `i:${idn.id}`, kind: 'identity', id: idn.id, label: name, userId: it.message.user?.id || '' });
    if (idn.sharedIdentityId && !map.has(`s:${idn.sharedIdentityId}`)) {
      map.set(`s:${idn.sharedIdentityId}`, { key: `s:${idn.sharedIdentityId}`, kind: 'shared', id: idn.sharedIdentityId, label: `${name}（共享角色）`, userId: it.message.user?.id || '' });
    }
  }
  return Array.from(map.values());
});
const speakerKey = ref<string | null>(null);
const speaker = computed(() => speakers.value.find((s) => s.key === speakerKey.value) || null);
const variantFilter = ref<string | null>(null); // null 表示全部差分；'' 表示无差分
const fromMs = ref<number | null>(null);
const toMs = ref<number | null>(null);

const variants = ref<LogIdentityVariantOption[]>([]);
watch(speaker, async (sp) => {
  variants.value = [];
  variantFilter.value = null;
  patchVariant.value = '';
  resetPreview();
  if (!sp || !props.section) return;
  try {
    const res = await fetchChannelIdentitiesForLog(props.section.source.channelId, sp.userId || undefined);
    variants.value = res.variants.filter((v) => sp.kind === 'shared' || v.identityId === sp.id);
  } catch {
    variants.value = [];
  }
});
const variantFilterOptions = computed(() => [
  { label: '全部差分', value: '__all__' },
  { label: '未使用差分', value: '' },
  ...variants.value.map((v) => ({ label: v.label, value: v.id })),
]);
const variantTargetOptions = computed(() => [{ label: '默认外观（取消差分）', value: '' }, ...variants.value.map((v) => ({ label: v.label, value: v.id }))]);

// ---- 当前 LOG 工程显示规则（独立的角色 ID / 用户 ID，全部条件为 AND） ----
const displayRules = computed(() => props.draft.displayRules || []);
const ruleEdit = ref<LogDisplayRule | null>(null);
const ruleTypes = LOG_MESSAGE_TYPES.map((type) => ({ label: type, value: type }));
const ruleIdentities = computed(() => speakers.value.filter((s) => s.kind === 'identity').map((s) => ({ label: s.label, value: s.id })));
const ruleUsers = computed(() => {
  const users = new Map<string, string>();
  for (const { message: m } of props.messages) {
    if (m.user?.id && !users.has(m.user.id)) users.set(m.user.id, m.user.nick || m.user.name || m.user.username || m.member?.nick || m.member?.name || m.sender_member_name || '未命名用户');
  }
  return Array.from(users, ([value, label]) => ({ value, label }));
});
const ruleVariants = ref<LogIdentityVariantOption[]>([]);
let ruleVariantSeq = 0;
watch([() => ruleEdit.value?.match.identityId, () => props.section?.source.channelId], async ([identityId, channelId]) => {
  const seq = ++ruleVariantSeq;
  ruleVariants.value = [];
  if (!identityId || !channelId) return;
  const owner = speakers.value.find((s) => s.kind === 'identity' && s.id === identityId);
  try {
    const result = await fetchChannelIdentitiesForLog(channelId, owner?.userId || undefined);
    if (seq === ruleVariantSeq) ruleVariants.value = result.variants.filter((v) => v.identityId === identityId);
  } catch {
    // 历史消息里的差分仍可选；无权限读取资料时不影响规则编辑。
  }
});
const ruleVariantOptions = computed(() => {
  const found = new Map(ruleVariants.value.map((v) => [v.id, v.label]));
  for (const { message: m } of props.messages) {
    if ((!ruleEdit.value?.match.identityId || m.identity?.id === ruleEdit.value.match.identityId) && m.identity?.variantId && !found.has(m.identity.variantId)) {
      found.set(m.identity.variantId, `${m.identity.displayName || '角色'}（历史差分 ${found.size + 1}）`);
    }
  }
  return Array.from(found, ([value, label]) => ({ value, label }));
});
const withCurrent = (options: Array<{ value: string; label: string }>, value: string | undefined, missing: string) =>
  value && !options.some((o) => o.value === value) ? [...options, { value, label: missing }] : options;
const templateOptions = computed(() => withCurrent(props.templateIds.map((value) => ({ value, label: value })), ruleEdit.value?.templateId, '模板已不存在或不可用于消息'));
const ruleLabel = (rule: LogDisplayRule) => {
  const m = rule.match;
  const name = (options: Array<{ value: string; label: string }>, id: string) => options.find((o) => o.value === id)?.label;
  return [m.messageType && `类型：${m.messageType}`, m.identityId && `角色：${name(ruleIdentities.value, m.identityId) || '未加载角色'}`,
    m.userId && `用户：${name(ruleUsers.value, m.userId) || '未加载用户'}`, m.variantId && `差分：${name(ruleVariantOptions.value, m.variantId) || '指定历史差分'}`].filter(Boolean).join(' · ');
};
const ruleCanSave = computed(() => !!ruleEdit.value && Object.values(ruleEdit.value.match).some(Boolean) && props.templateIds.includes(ruleEdit.value.templateId));
const editRule = (rule?: LogDisplayRule) => {
  if (!props.canEditDraft) return;
  ruleEdit.value = rule ? cloneJson(rule) : { id: uuid(), match: {}, templateId: props.templateIds[0] || '' };
};
const setRuleMatch = (key: keyof LogDisplayRule['match'], value: string | null) => {
  if (!ruleEdit.value || !props.canEditDraft) return;
  if (value) ruleEdit.value.match[key] = value;
  else delete ruleEdit.value.match[key];
  if (key === 'identityId') delete ruleEdit.value.match.variantId;
};
const saveRule = () => {
  if (!props.canEditDraft || !ruleCanSave.value || !ruleEdit.value) return;
  const rule = cloneJson(ruleEdit.value);
  if (props.update((next) => {
    const rules = next.displayRules ||= [];
    const index = rules.findIndex((r) => r.id === rule.id);
    if (index < 0) rules.push(rule); else rules[index] = rule;
  })) ruleEdit.value = null;
};
const deleteRule = (id: string) => {
  if (!props.canEditDraft) return;
  if (props.update((next) => { next.displayRules = (next.displayRules || []).filter((r) => r.id !== id); }) && ruleEdit.value?.id === id) ruleEdit.value = null;
};
const moveRule = (id: string, direction: -1 | 1) => {
  if (!props.canEditDraft) return;
  props.update((next) => {
    const rules = next.displayRules || [];
    const index = rules.findIndex((r) => r.id === id);
    const target = index + direction;
    if (index >= 0 && target >= 0 && target < rules.length) [rules[index], rules[target]] = [rules[target], rules[index]];
  });
};

// ---- patch ----
const edit = ref({ name: false, color: false, avatar: false });
const patchName = ref('');
const patchColor = ref('');
const patchAvatar = ref('');
const patchVariant = ref('');
const avatarUploading = ref(false);
const avatarInput = ref<HTMLInputElement | null>(null);

const buildPatch = (): LogAppearancePatch | null => {
  if (action.value === 'variant') return { variant_id: patchVariant.value };
  const p: LogAppearancePatch = {};
  if (edit.value.name) p.name = patchName.value.trim();
  if (edit.value.color) p.color = patchColor.value;
  if (edit.value.avatar) p.avatar_attachment_id = patchAvatar.value;
  return Object.keys(p).length ? p : null;
};
const patchError = computed(() => {
  if (action.value === 'snapshot') {
    if (!edit.value.name && !edit.value.color && !edit.value.avatar) return '至少勾选一项要修改的外观';
    if (edit.value.color && patchColor.value && !HEX.test(patchColor.value)) return '颜色须为 #RRGGBB';
  }
  return '';
});

const onAvatarFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file || !props.section) return;
  avatarUploading.value = true;
  try {
    const { attachmentId } = await uploadImageAttachment(file, { channelId: props.section.source.channelId });
    patchAvatar.value = attachmentId.replace(/^id:/, '');
    edit.value.avatar = true;
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '上传失败');
  } finally {
    avatarUploading.value = false;
  }
};

// ---- 预览 ----
const preview = ref<LogAppearancePreview | null>(null);
const previewBusy = ref(false);
const previewError = ref('');
const now = ref(Date.now());
const ticker = setInterval(() => {
  now.value = Date.now();
}, 5000);
const previewExpired = computed(() => !!preview.value && Date.parse(preview.value.expires_at) <= now.value);
function resetPreview() {
  preview.value = null;
  previewError.value = '';
}
watch([action, edit, patchName, patchColor, patchAvatar, patchVariant, variantFilter, fromMs, toMs], resetPreview, { deep: true });
watch(() => props.section?.id, () => {
  const s = props.section;
  fromMs.value = s?.source.fromTime ? Date.parse(s.source.fromTime) : null;
  toMs.value = s ? Date.parse(s.source.toTime) : null;
  speakerKey.value = null;
  resetPreview();
}, { immediate: true });

const runPreview = async () => {
  const sp = speaker.value;
  const sec = props.section;
  const patch = buildPatch();
  if (!sp || !sec || !patch || fromMs.value === null || toMs.value === null) return;
  previewBusy.value = true;
  previewError.value = '';
  try {
    preview.value = await previewAppearanceBatch({
      channelId: sec.source.channelId,
      selection: {
        identity_id: sp.kind === 'identity' ? sp.id : null,
        shared_identity_id: sp.kind === 'shared' ? sp.id : null,
        variant_id: variantFilter.value === '__all__' || variantFilter.value === null ? null : variantFilter.value,
        from_time: new Date(fromMs.value).toISOString(),
        to_time: new Date(toMs.value).toISOString(),
      },
      patch,
    });
  } catch (error) {
    previewError.value = describeLogError(error, '预览失败');
  } finally {
    previewBusy.value = false;
  }
};

const sampleText = (v: Record<string, unknown>) => {
  const pick = (k: string, ...alts: string[]) => [k, ...alts].map((x) => v[x]).find((x) => x !== undefined && x !== null);
  const name = pick('name', 'sender_identity_name');
  const color = pick('color', 'sender_identity_color');
  const avatar = pick('avatarAttachmentId', 'sender_identity_avatar_id');
  const variant = pick('variantId', 'sender_identity_variant_id');
  return [
    name !== undefined ? `名：${String(name) || '（空）'}` : '',
    color !== undefined ? `色：${String(color) || '（空）'}` : '',
    avatar !== undefined ? `头像：${avatar ? '已设置' : '（空）'}` : '',
    variant !== undefined ? `差分：${String(variant) || '（无）'}` : '',
  ].filter(Boolean).join(' · ');
};

// ---- 执行与持久任务 ----
const job = ref<LogMutationJob | null>(null);
const jobError = ref('');
const applying = ref(false);
let pollTimer: ReturnType<typeof setTimeout> | null = null;
let disposed = false;

const saveJob = (jobId: string | null) => {
  try {
    if (jobId) sessionStorage.setItem(JOB_KEY.value, jobId);
    else sessionStorage.removeItem(JOB_KEY.value);
  } catch {
    // 存储不可用时只能在当前页面跟踪
  }
};

const poll = async (jobId: string) => {
  if (disposed) return;
  try {
    const j = await fetchMutationJob(jobId);
    if (disposed) return;
    job.value = j;
    jobError.value = '';
    if (j.status === 'done' || j.status === 'failed') {
      saveJob(null);
      emit('applied');
      void loadItems(true);
      return;
    }
  } catch (error) {
    if (isLogError(error, 'NOT_FOUND', 'PERMISSION_DENIED')) {
      saveJob(null);
      jobError.value = describeLogError(error, '任务不可用');
      return;
    }
    jobError.value = `${describeLogError(error, '读取任务失败')}，稍后重试`;
  }
  pollTimer = setTimeout(() => void poll(jobId), 1500);
};

const apply = async () => {
  const p = preview.value;
  if (!p || previewExpired.value) return;
  applying.value = true;
  try {
    const r = await applyAppearanceBatch(p.preview_id, newClientOpId('bulk'));
    preview.value = null;
    job.value = { jobId: r.job_id, status: 'queued', matched: p.matched_count, succeeded: 0, conflicted: 0, denied: 0, failed: 0 };
    failedItems.value = [];
    itemsCursor.value = null;
    saveJob(r.job_id);
    void poll(r.job_id);
  } catch (error) {
    if (isLogError(error, 'PREVIEW_EXPIRED')) {
      preview.value = null;
      previewError.value = '预览已过期，请重新预览';
    } else {
      toast.error(describeLogError(error, '提交失败'));
    }
  } finally {
    applying.value = false;
  }
};

const failedItems = ref<LogMutationJobItem[]>([]);
const itemsCursor = ref<string | null>(null);
const itemsLoading = ref(false);
const loadItems = async (reset = false) => {
  const j = job.value;
  if (!j) return;
  itemsLoading.value = true;
  try {
    const r = await fetchMutationJobItems(j.jobId, reset ? null : itemsCursor.value, 100);
    const bad = r.items.filter((it) => it.status !== 'success');
    failedItems.value = reset ? bad : [...failedItems.value, ...bad];
    itemsCursor.value = r.nextCursor || null;
  } catch (error) {
    jobError.value = describeLogError(error, '读取结果失败');
  } finally {
    itemsLoading.value = false;
  }
};

const itemStatusText = (s: LogMutationJobItem['status']) => ({ success: '成功', conflict: '版本冲突', denied: '无权限', failed: '失败' }[s]);
const progress = computed(() => {
  const j = job.value;
  if (!j || !j.matched) return 0;
  return Math.min(100, Math.round(((j.succeeded + j.conflicted + j.denied + j.failed) / j.matched) * 100));
});

onMounted(() => {
  let saved: string | null = null;
  try {
    saved = sessionStorage.getItem(JOB_KEY.value);
  } catch {
    saved = null;
  }
  if (saved) {
    job.value = { jobId: saved, status: 'running', matched: 0, succeeded: 0, conflicted: 0, denied: 0, failed: 0 };
    void poll(saved);
  }
});
onBeforeUnmount(() => {
  disposed = true;
  ruleVariantSeq++;
  if (pollTimer) clearTimeout(pollTimer);
  clearInterval(ticker);
});

const openChannel = () => {
  if (props.section) void router.push({ name: 'world-channel', params: { worldId: props.worldId, channelId: props.section.source.channelId } });
};
</script>

<template>
  <div class="log-bulk">
    <n-radio-group v-model:value="mode" size="small" class="log-bulk__modes">
      <n-radio-button value="appearance">历史消息外观</n-radio-button>
      <n-radio-button value="profile">角色资料</n-radio-button>
      <n-radio-button value="rules">LOG 显示规则</n-radio-button>
    </n-radio-group>

    <template v-if="mode === 'appearance'">
      <n-alert type="warning" :show-icon="false">会写回聊天原消息的外观快照（显示名、颜色、头像或差分），聊天页同步变化；不修改角色资料。</n-alert>
      <n-alert v-if="!canBulk" type="info" :show-icon="false">你可以预览；只有你有权编辑的消息会被修改，其余计入“无权限”。</n-alert>

      <n-form size="small" label-placement="top">
        <n-form-item label="角色">
          <n-select v-model:value="speakerKey" :options="speakers.map((s) => ({ label: s.label, value: s.key }))" placeholder="从本章已加载消息中选择角色" filterable />
        </n-form-item>
        <n-form-item label="限定差分">
          <n-select :value="variantFilter ?? '__all__'" :options="variantFilterOptions" :disabled="!speaker" @update:value="(v: string) => (variantFilter = v === '__all__' ? null : v)" />
        </n-form-item>
        <n-form-item label="时间范围">
          <n-space vertical size="small" style="width: 100%">
            <n-date-picker v-model:value="fromMs" type="datetime" clearable placeholder="开始" />
            <n-date-picker v-model:value="toMs" type="datetime" clearable placeholder="结束" />
          </n-space>
        </n-form-item>
      </n-form>

      <n-radio-group v-model:value="action" size="small">
        <n-radio-button value="snapshot">修改外观</n-radio-button>
        <n-radio-button value="variant">切换差分</n-radio-button>
      </n-radio-group>
      <n-form v-if="action === 'snapshot'" size="small" label-placement="left" label-width="80" class="log-bulk__patch">
        <n-form-item>
          <template #label><n-checkbox v-model:checked="edit.name">显示名</n-checkbox></template>
          <n-input v-model:value="patchName" :disabled="!edit.name" maxlength="64" placeholder="留空表示清除覆盖" />
        </n-form-item>
        <n-form-item>
          <template #label><n-checkbox v-model:checked="edit.color">颜色</n-checkbox></template>
          <n-color-picker v-model:value="patchColor" :modes="['hex']" :show-alpha="false" :disabled="!edit.color" />
        </n-form-item>
        <n-form-item>
          <template #label><n-checkbox v-model:checked="edit.avatar">头像</n-checkbox></template>
          <n-space size="small">
            <n-button size="tiny" :loading="avatarUploading" :disabled="!section" @click="avatarInput?.click()">上传</n-button>
            <n-button size="tiny" quaternary @click="patchAvatar = ''; edit.avatar = true">清除</n-button>
            <span class="log-bulk__hint">{{ edit.avatar ? (patchAvatar ? '新头像已上传' : '将清除') : '' }}</span>
          </n-space>
          <input ref="avatarInput" type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden @change="onAvatarFile">
        </n-form-item>
      </n-form>
      <n-form v-else size="small" label-placement="top" class="log-bulk__patch">
        <n-form-item label="切换到差分">
          <n-select v-model:value="patchVariant" :options="variantTargetOptions" :disabled="!speaker" />
        </n-form-item>
      </n-form>
      <p v-if="patchError" class="log-bulk__hint is-error">{{ patchError }}</p>

      <n-button size="small" :loading="previewBusy" :disabled="!speaker || !!patchError || fromMs === null || toMs === null || !!(job && job.status !== 'done' && job.status !== 'failed')" @click="runPreview">预览影响</n-button>
      <n-alert v-if="previewError" type="error" :show-icon="false">{{ previewError }}</n-alert>

      <div v-if="preview" class="log-bulk__preview">
        <div>匹配 <b>{{ preview.matched_count }}</b> 条 · 可修改 <b>{{ preview.allowed_count }}</b> · 无权限 <b>{{ preview.denied_count }}</b></div>
        <div class="log-bulk__hint">预览有效至 {{ new Date(preview.expires_at).toLocaleTimeString() }}{{ previewExpired ? '（已过期，请重新预览）' : '' }}</div>
        <div v-for="s in preview.samples.slice(0, 8)" :key="s.message_id" class="log-bulk__sample">
          <div>前：{{ sampleText(s.before) }}</div>
          <div>后：{{ sampleText(s.after) }}</div>
        </div>
        <n-popconfirm @positive-click="apply">
          <template #trigger>
            <n-button type="primary" size="small" :loading="applying" :disabled="previewExpired || !preview.allowed_count">确认修改 {{ preview.allowed_count }} 条原消息</n-button>
          </template>
          将修改 {{ preview.allowed_count }} 条聊天原消息的外观，确定？
        </n-popconfirm>
      </div>

      <div v-if="job" class="log-bulk__job" role="status" aria-live="polite">
        <div class="log-bulk__title">任务 {{ job.jobId.slice(0, 8) }}：{{ { queued: '排队中', running: '执行中', done: '已完成', failed: '失败' }[job.status] }}</div>
        <n-progress type="line" :percentage="progress" :status="job.status === 'failed' ? 'error' : job.status === 'done' ? 'success' : 'default'" />
        <div class="log-bulk__hint">成功 {{ job.succeeded }} · 冲突 {{ job.conflicted }} · 无权限 {{ job.denied }} · 失败 {{ job.failed }}（共 {{ job.matched }}）</div>
        <p class="log-bulk__hint">离开页面后任务仍在服务器上继续；返回本页会按任务号恢复进度。</p>
        <n-alert v-if="jobError" type="warning" :show-icon="false">{{ jobError }}</n-alert>
        <template v-if="job.status === 'done' || job.status === 'failed'">
          <div v-for="it in failedItems" :key="it.messageId" class="log-bulk__item">
            <code>{{ it.messageId.slice(0, 10) }}</code> {{ itemStatusText(it.status) }}{{ it.code ? `（${it.code}）` : '' }}
          </div>
          <n-button v-if="itemsCursor" size="tiny" :loading="itemsLoading" @click="loadItems()">加载更多结果</n-button>
          <n-button size="tiny" quaternary @click="job = null; failedItems = []">关闭结果</n-button>
        </template>
      </div>
    </template>

    <template v-else-if="mode === 'profile'">
      <p class="log-bulk__hint">角色资料（名称、颜色、头像、差分）在聊天频道的角色管理中修改，遵循共享角色与代管权限。修改资料会影响之后的新消息，不会改写已发送消息的历史快照；要改历史显示请使用“历史消息外观”。</p>
      <n-button size="small" :disabled="!section" @click="openChannel">打开来源频道管理角色</n-button>
    </template>

    <template v-else>
      <p class="log-bulk__hint">仅保存到当前 LOG 工程。全部已填条件须同时满足，第一条有效匹配优先；单条消息的精确指定优先于这里的规则。</p>
      <p v-if="!canEditDraft" class="log-bulk__hint">当前没有编辑草稿权限，显示规则只读。</p>
      <div v-for="(rule, index) in displayRules" :key="rule.id" class="log-bulk__preview">
        <div>{{ index + 1 }}. {{ ruleLabel(rule) }}</div>
        <div :class="{ 'is-error': !templateIds.includes(rule.templateId) }" class="log-bulk__hint">→ {{ rule.templateId }}{{ templateIds.includes(rule.templateId) ? '' : '（模板已不存在或不可用于消息，将继续自动匹配）' }}</div>
        <n-space size="small">
          <n-button size="tiny" :disabled="!canEditDraft || index === 0" @click="moveRule(rule.id, -1)">上移</n-button>
          <n-button size="tiny" :disabled="!canEditDraft || index === displayRules.length - 1" @click="moveRule(rule.id, 1)">下移</n-button>
          <n-button size="tiny" :disabled="!canEditDraft" @click="editRule(rule)">编辑</n-button>
          <n-button size="tiny" :disabled="!canEditDraft" @click="deleteRule(rule.id)">删除</n-button>
        </n-space>
      </div>
      <p v-if="!displayRules.length" class="log-bulk__hint">暂无工程规则，使用样式包默认规则。</p>
      <n-button size="small" :disabled="!canEditDraft || displayRules.length >= 1000" @click="editRule()">新建规则</n-button>
      <n-form v-if="ruleEdit" size="small" label-placement="top" class="log-bulk__preview" :disabled="!canEditDraft">
        <n-form-item label="消息类型（留空不限）">
          <n-select :value="ruleEdit.match.messageType || null" :options="ruleTypes" clearable placeholder="不限类型" @update:value="(v: string | null) => setRuleMatch('messageType', v)" />
        </n-form-item>
        <n-form-item label="角色（留空不限）">
          <n-select :value="ruleEdit.match.identityId || null" :options="withCurrent(ruleIdentities, ruleEdit.match.identityId, '未加载角色')" filterable clearable placeholder="从本片段已加载消息中选择" @update:value="(v: string | null) => setRuleMatch('identityId', v)" />
        </n-form-item>
        <n-form-item label="发送用户（使用任何角色或无角色时均匹配）">
          <n-select :value="ruleEdit.match.userId || null" :options="withCurrent(ruleUsers, ruleEdit.match.userId, '未加载用户')" filterable clearable placeholder="不限用户" @update:value="(v: string | null) => setRuleMatch('userId', v)" />
        </n-form-item>
        <n-form-item label="角色差分（留空不限）">
          <n-select :value="ruleEdit.match.variantId || null" :options="withCurrent(ruleVariantOptions, ruleEdit.match.variantId, '指定历史差分')" filterable clearable placeholder="选择角色后可补全差分" @update:value="(v: string | null) => setRuleMatch('variantId', v)" />
        </n-form-item>
        <n-form-item label="对话框模板">
          <n-select v-model:value="ruleEdit.templateId" :options="templateOptions" filterable placeholder="当前样式包的消息模板" />
        </n-form-item>
        <p v-if="!Object.values(ruleEdit.match).some(Boolean)" class="log-bulk__hint is-error">至少选择一个匹配条件。</p>
        <p v-if="!templateIds.includes(ruleEdit.templateId)" class="log-bulk__hint is-error">请重新选择当前样式包中的合法消息模板。</p>
        <n-space size="small">
          <n-button size="small" type="primary" :disabled="!canEditDraft || !ruleCanSave" @click="saveRule">保存规则</n-button>
          <n-button size="small" @click="ruleEdit = null">取消</n-button>
        </n-space>
      </n-form>
    </template>
  </div>
</template>

<style scoped>
.log-bulk {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 0 16px;
}

.log-bulk__modes {
  display: flex;
}

.log-bulk__modes :deep(.n-radio-button) {
  flex: 1;
  text-align: center;
}

.log-bulk__title {
  font-weight: 600;
  font-size: 13px;
}

.log-bulk__hint {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  margin: 0;
}

.log-bulk__hint.is-error {
  color: #d03050;
}

.log-bulk__patch {
  margin-top: 6px;
}

.log-bulk__preview,
.log-bulk__job {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--sc-border-strong, #e5e5e5);
  border-radius: 8px;
  font-size: 13px;
}

.log-bulk__sample {
  font-size: 12px;
  padding: 4px 0;
  border-top: 1px dashed var(--sc-border-mute, #e5e5e5);
}

.log-bulk__item {
  font-size: 12px;
}
</style>
