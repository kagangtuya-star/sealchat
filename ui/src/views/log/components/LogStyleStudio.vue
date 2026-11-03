<script setup lang="ts">
// Style Studio：样式库级的样式设计器。持有唯一的工作副本（编辑器状态，不是新的样式模型），
// 负责 打开 / 复制 / 新建 / ZIP 暂存 → 编辑 → 本地预览 → 保存为新版本 → （另一个动作）应用到当前 LOG。
// - 保存始终走既有接口：私有样式新版本、新建首版本、ZIP 导入；服务端 ValidateLogStyle 为最终校验；
// - 内置样式与他人的样式只读，编辑前先 fork 成自己的私有样式；
// - 预览只通过 preview 事件替换工作台预览，从不修改草稿的 stylePackageId / stylePackageVersion。
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useDialog, useMessage, type DropdownOption } from 'naive-ui';
import {
  AlertTriangle,
  Braces,
  Code,
  Copy,
  DeviceFloppy,
  Download,
  Eye,
  InfoCircle,
  ListDetails,
  Palette,
  Photo,
  Template,
  Variable,
  X,
} from '@vicons/tabler';
import {
  createLogStyle,
  describeLogError,
  exportLogStyleZip,
  fetchAttachmentBytes,
  fetchLogStyleAssetBytes,
  fetchLogStylePackage,
  forkLogStyle,
  importLogStyleZip,
  isLogError,
  saveLogStyleVersion,
} from '@/services/log/logApi';
import { LOG_RENDERER_VERSION, checkStyleSource, countBodySlots, type TemplateIssue, type TemplateLibrary } from '@/services/log/logTemplateCompiler';
import { templatesForElementType } from '@/services/log/logRenderer';
import { buildStyleZip, readStyleZip } from '@/services/log/logStyleZip';
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader';
import { LOG_ELEMENT_TYPES, type LogDraft, type LogStyleAsset, type LogStylePackage, type LogStyleSummary } from '@/types/log';
import {
  STYLE_KEY,
  STYLE_SEMVER,
  analyzeTemplates,
  assembleStylePackage,
  blankStylePackage,
  compareStyleVersions,
  cssAssetRefs,
  draftStyleAssetRefs,
  freshStyleId,
  inheritedAssetsDivergedFromLatest,
  isFontMime,
  nextStyleVersion,
  parseStyleJson,
  sniffStyleAssetMime,
  styleAssetPath,
  tokenCssHint,
  workingCopyFingerprint,
  workingCopyFromPackage,
  type StyleStudioPreview,
  type StyleStudioRequest,
  type StyleWorkingCopy,
  type VersionBump,
} from '../logStyleWorkingCopy';
import type { LogStyleLibrary } from '../composables/useLogStyleLibrary';
import LogStyleLibraryView from './LogStyleLibraryView.vue';
import LogStyleTemplatesEditor from './LogStyleTemplatesEditor.vue';
import LogStyleTokensEditor from './LogStyleTokensEditor.vue';
import LogStyleRulesEditor from './LogStyleRulesEditor.vue';
import LogStyleAssetsEditor from './LogStyleAssetsEditor.vue';

const props = defineProps<{
  library: LogStyleLibrary;
  /** 只读：用于统计当前 LOG 对模板 / 资源的引用。 */
  draft: LogDraft;
  /** 当前 LOG 正式绑定的样式包。 */
  boundPackage: LogStylePackage | null;
  canBind: boolean;
  /** 让当前 LOG 使用某个已保存版本（工作台负责兼容检查与草稿写入）。 */
  bind: (pkg: LogStylePackage) => Promise<boolean>;
  request: (StyleStudioRequest & { nonce: number }) | null;
  narrow: boolean;
}>();
const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'preview', payload: StyleStudioPreview | null): void;
  (e: 'peek'): void;
}>();

const toast = useMessage();
const dialog = useDialog();

type Section = 'overview' | 'templates' | 'css' | 'tokens' | 'rules' | 'assets' | 'source';
type LocalAsset = { data: ArrayBuffer; mime: string };
interface ZipStage {
  file: File;
  issues: string[];
  warnings: string[];
  extras: Record<string, Uint8Array>;
  files: string[];
  /** 打开时的指纹：未修改且无问题时直接上传原文件。 */
  fingerprint: string;
}

// ---- 工作副本 ----
const copy = ref<StyleWorkingCopy | null>(null);
const baseline = ref('');
const session = ref(0);
const localAssets = shallowRef(new Map<string, LocalAsset>());
const zip = shallowRef<ZipStage | null>(null);
const section = ref<Section>('overview');
const activeTemplate = ref('');
const bump = ref<VersionBump>('patch');
const busy = ref(false);
const serverError = ref('');
const conflict = ref<{ latest: string } | null>(null);
const lastSaved = shallowRef<LogStylePackage | null>(null);
const livePreview = ref(true);

const copyOwnership = computed(() => (copy.value ? props.library.ownership(copy.value.id) : 'unknown'));
const ownershipUnresolved = computed(() => !!copy.value && copy.value.origin === 'version' && copyOwnership.value === 'unknown' && !props.library.loaded.value);
const readonly = computed(() => !!copy.value && copy.value.origin === 'version' && (copyOwnership.value === 'foreign' || ownershipUnresolved.value));
const fingerprint = computed(() => (copy.value ? workingCopyFingerprint(copy.value) : ''));
const dirty = computed(() => !!copy.value && fingerprint.value !== baseline.value);
const parsed = computed(() => (copy.value ? parseStyleJson(copy.value) : null));
const assembled = computed(() => (copy.value && parsed.value ? assembleStylePackage(copy.value, parsed.value) : { pkg: null, issues: [] }));

/** 改动工作副本的唯一入口：只读时忽略。 */
const edit = (mutate: (c: StyleWorkingCopy) => void) => {
  if (!copy.value || readonly.value) return;
  mutate(copy.value);
};

// 前端快速检查（模板白名单、片段图、规则插槽、CSS）防抖执行；最终仍以服务端校验为准。
const check = shallowRef<{ library: TemplateLibrary; issues: TemplateIssue[] } | null>(null);
const checkPending = ref(false);
let checkTimer: ReturnType<typeof setTimeout> | null = null;
const runCheck = () => {
  if (checkTimer) clearTimeout(checkTimer);
  checkTimer = null;
  checkPending.value = false;
  const c = copy.value;
  const p = parsed.value;
  if (!c || !p) {
    check.value = null;
    return;
  }
  check.value = checkStyleSource({
    templates: c.templates,
    css: c.css,
    tokens: p.tokens || {},
    rules: p.rules || [],
    settingsSchema: p.schema || {},
    assets: c.assets,
  });
};
watch(fingerprint, () => {
  if (checkTimer) clearTimeout(checkTimer);
  checkPending.value = true;
  checkTimer = setTimeout(runCheck, 250);
});

const issueText = (i: TemplateIssue) => `${i.templateId ? `[${i.templateId}] ` : ''}${i.message}`;
const allIssues = computed(() => [...assembled.value.issues, ...(check.value?.issues || []).map(issueText)]);
const templateIssues = computed(() => {
  const out: Record<string, string[]> = {};
  for (const i of check.value?.issues || []) if (i.templateId) (out[i.templateId] ||= []).push(i.message);
  return out;
});
const cssIssues = computed(() => (check.value?.issues || []).filter((i) => !i.templateId && i.message.startsWith('CSS')).map((i) => i.message));
const checking = computed(() => !!copy.value && (!check.value || checkPending.value));
const graph = computed(() => {
  const lib = check.value?.library;
  const parts: Record<string, string[]> = {};
  const bodySlots: Record<string, number> = {};
  if (lib) {
    for (const [id, t] of lib.templates) {
      parts[id] = t.parts;
      bodySlots[id] = countBodySlots(lib, id);
    }
  }
  return { parts, bodySlots };
});
/** 当前 LOG 绑定的正是这个样式包时，才统计草稿引用。 */
const boundSame = computed(() => !!copy.value && props.boundPackage?.id === copy.value.id);
const usage = computed(() => {
  const c = copy.value;
  if (!c || !parsed.value) return {};
  const schema = parsed.value.schema || {};
  const elementTypesFor = (id: string) => LOG_ELEMENT_TYPES.filter((t) => templatesForElementType(t, c.templates, schema).includes(id));
  return analyzeTemplates(c, parsed.value, graph.value, elementTypesFor, boundSame.value ? props.draft : null);
});
const cssRefs = computed(() => cssAssetRefs(copy.value?.css || ''));
const draftAssetRefs = computed(() => (boundSame.value ? draftStyleAssetRefs(props.draft) : null));

// ---- 预览 ----
const libraryPreviewKey = ref('');
const previewState = ref<'off' | 'live' | 'stale'>('off');
const emitPreview = (pkg: LogStylePackage) => {
  const assets: StyleStudioPreview['assets'] = {};
  for (const [id, a] of localAssets.value) assets[id] = a;
  emit('preview', { pkg, assets, session: session.value });
};
const clearPreview = () => {
  libraryPreviewKey.value = '';
  previewState.value = 'off';
  emit('preview', null);
};
const refreshPreview = (manual = false) => {
  const pkg = assembled.value.pkg;
  if (!copy.value || checking.value) return;
  if (!pkg || allIssues.value.length) {
    if (previewState.value !== 'off') previewState.value = 'stale';
    if (manual) toast.warning('存在问题，预览保持上次通过检查的结果');
    return;
  }
  emitPreview(pkg);
  previewState.value = 'live';
};
watch(check, () => {
  if (livePreview.value) refreshPreview();
});
watch(livePreview, (on) => {
  if (on) refreshPreview();
});
const stopPreview = () => {
  livePreview.value = false;
  clearPreview();
};

// ---- 打开 / 关闭（未保存保护） ----
const confirmDiscard = (): Promise<boolean> => {
  if (!dirty.value) return Promise.resolve(true);
  return new Promise((resolve) => {
    let settled = false;
    const done = (v: boolean) => {
      if (!settled) resolve(v);
      settled = true;
    };
    dialog.warning({
      title: '放弃未保存的修改？',
      content: `「${copy.value?.name || '未命名样式'}」有尚未保存为版本的修改，离开后将丢失。`,
      positiveText: '放弃修改',
      negativeText: '继续编辑',
      onPositiveClick: () => done(true),
      onNegativeClick: () => done(false),
      onClose: () => done(false),
      onMaskClick: () => done(false),
    });
  });
};

const thumbUrls = new Map<string, Promise<string | null>>();
const revokeThumbs = () => {
  for (const task of thumbUrls.values()) void task.then((url) => url && URL.revokeObjectURL(url));
  thumbUrls.clear();
};

const load = (wc: StyleWorkingCopy, opts: { assets?: Map<string, LocalAsset>; zip?: Omit<ZipStage, 'fingerprint'>; keepView?: boolean } = {}) => {
  revokeThumbs();
  clearPreview();
  copy.value = wc;
  baseline.value = workingCopyFingerprint(wc);
  session.value += 1;
  localAssets.value = opts.assets || new Map();
  zip.value = opts.zip ? { ...opts.zip, fingerprint: baseline.value } : null;
  if (!opts.keepView) {
    section.value = 'overview';
    activeTemplate.value = '';
    lastSaved.value = null;
  }
  serverError.value = '';
  conflict.value = null;
  libraryPreviewKey.value = '';
  runCheck();
};

const goLibrary = async () => {
  if (!copy.value) return;
  if (!(await confirmDiscard())) return;
  copy.value = null;
  zip.value = null;
  clearPreview();
};

const requestClose = async () => {
  if (!(await confirmDiscard())) return false;
  copy.value = null;
  clearPreview();
  emit('close');
  return true;
};
defineExpose({ requestClose });

const onBeforeUnload = (e: BeforeUnloadEvent) => {
  if (!dirty.value) return;
  e.preventDefault();
  e.returnValue = '';
};
window.addEventListener('beforeunload', onBeforeUnload);
onBeforeRouteLeave(() => confirmDiscard());
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload);
  if (checkTimer) clearTimeout(checkTimer);
  revokeThumbs();
});

const run = async <T>(label: string, fn: () => Promise<T>): Promise<T | null> => {
  busy.value = true;
  try {
    return await fn();
  } catch (error) {
    toast.error(describeLogError(error, `${label}失败`));
    return null;
  } finally {
    busy.value = false;
  }
};

/** 打开样式：自己的私有样式可编辑；内置或他人的样式只读查看。 */
const openStyle = async (s: { id: string; version: string }) => {
  if (!(await confirmDiscard())) return;
  const pkg = await run('读取样式', () => fetchLogStylePackage(s.id, s.version));
  if (pkg) load(workingCopyFromPackage(pkg, 'version'));
};

// 复制（fork）：内置样式“编辑”的唯一途径。
const forkTarget = ref<{ id: string; version: string } | null>(null);
const forkName = ref('');
// 与 forkTarget 分开：关闭动画期间对话框文字保持不变。
const forkOpen = ref(false);
const askFork = (s: { id: string; version: string; name: string }) => {
  forkTarget.value = { id: s.id, version: s.version };
  forkOpen.value = true;
  forkName.value = `${s.name}（我的副本）`.slice(0, 200);
};
const doFork = async () => {
  const target = forkTarget.value;
  const name = forkName.value.trim();
  if (!forkOpen.value || !target || !name) return;
  if (!(await confirmDiscard())) return;
  forkOpen.value = false;
  const forked = await run('复制', () => forkLogStyle(target.id, target.version, name));
  if (!forked) return;
  await props.library.reload();
  load(workingCopyFromPackage(forked, 'version'));
  toast.success(`已复制为私有样式「${forked.name}」，可以开始编辑`);
};

const newBlank = async () => {
  if (!(await confirmDiscard())) return;
  load(workingCopyFromPackage(blankStylePackage(LOG_RENDERER_VERSION), 'new'));
  section.value = 'templates';
};

// ZIP：先在本地完整解析并展示问题，可进入设计器修改，确认后再提交服务端导入。
const zipInput = ref<HTMLInputElement | null>(null);
const pickZip = () => zipInput.value?.click();
const onZipFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  if (!(await confirmDiscard())) return;
  const content = await run('读取 ZIP', () => readStyleZip(file));
  if (!content) return;
  const assets = new Map<string, LocalAsset>();
  for (const a of content.pkg.assets) {
    const bytes = content.assets[a.id];
    const mime = sniffStyleAssetMime(bytes);
    if (mime && mime !== a.mime) {
      content.warnings.push(`资源 ${a.path} 实际为 ${mime}，已更正声明的 MIME`);
      a.mime = mime;
    } else if (!mime) {
      content.issues.push(`资源 ${a.path} 不是受支持的图片或字体`);
    }
    assets.set(a.id, { data: bytes.slice().buffer, mime: mime || a.mime });
  }
  const wc = workingCopyFromPackage(content.pkg, 'zip');
  if (!wc.name.trim()) wc.name = file.name.replace(/\.zip$/i, '').slice(0, 200) || '导入的样式';
  load(wc, { assets, zip: { file, issues: content.issues, warnings: content.warnings, extras: content.extras, files: content.files } });
};

watch(() => props.request?.nonce, () => {
  const r = props.request;
  if (!r) return;
  if (r.kind === 'library') void goLibrary();
  else if (r.kind === 'new') void newBlank();
  else if (r.kind === 'fork') askFork(r);
  else if (r.kind === 'open') {
    // 自己的样式总是从最新版本继续编辑，否则保存时必然版本冲突。
    const latest = props.library.owns(r.id) ? props.library.byId(r.id)?.version : '';
    void openStyle({ id: r.id, version: latest || r.version });
  }
}, { immediate: true });
if (!props.library.loaded.value && !props.library.loading.value) void props.library.reload();

// ---- 资源 ----
const assetBusy = ref(false);
const ATTACHMENT_ID = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$/;
const randomAssetId = () => `a-${Array.from(crypto.getRandomValues(new Uint8Array(8)), (b) => b.toString(16).padStart(2, '0')).join('')}`;

/** 上传资源：保存到既有样式包时先上传为自己的附件（服务端按附件读取字节）；ZIP 暂存只保留本地字节。 */
const addAssets = async (files: File[], replaceId = '') => {
  const c = copy.value;
  if (!c || readonly.value) return;
  const owner = session.value;
  assetBusy.value = true;
  try {
    for (const file of files) {
      if (session.value !== owner) return; // 上传期间切换了工作副本：放弃剩余文件
      let bytes = new Uint8Array(await file.arrayBuffer());
      let mime = sniffStyleAssetMime(bytes);
      if (!mime) {
        toast.error(`${file.name}：只支持 PNG / JPEG / WebP 图片和 TTF / OTF 字体`);
        continue;
      }
      if (bytes.byteLength > 20 * 1024 * 1024) {
        toast.error(`${file.name}：单个资源不能超过 20MB`);
        continue;
      }
      const old = replaceId ? c.assets.find((a) => a.id === replaceId) : undefined;
      if (old && isFontMime(old.mime) !== isFontMime(mime)) {
        toast.error('替换文件的类型必须与原资源一致（图片换图片，字体换字体）');
        continue;
      }
      let id = randomAssetId();
      if (c.origin !== 'zip') {
        const { attachmentId } = await uploadImageAttachment(new File([bytes], file.name, { type: mime }), { channelId: null, skipCompression: true });
        id = attachmentId.replace(/^id:/, '');
        // 服务端开启图片压缩时会把 PNG / JPEG 转存为 WebP；以实际存储的字节为准，
        // 否则声明的 MIME 与服务端校验不一致，保存会被拒绝。
        if (ATTACHMENT_ID.test(id)) {
          const stored = new Uint8Array((await fetchAttachmentBytes(id)).data);
          const storedMime = sniffStyleAssetMime(stored);
          if (!storedMime || isFontMime(storedMime) !== isFontMime(mime)) {
            toast.error(`${file.name}：上传后的文件无法识别`);
            continue;
          }
          bytes = stored;
          mime = storedMime;
        }
      }
      if (session.value !== owner) return;
      if (!ATTACHMENT_ID.test(id)) {
        toast.error('上传结果无效');
        continue;
      }
      const next = new Map(localAssets.value);
      next.set(id, { data: bytes.buffer, mime });
      localAssets.value = next;
      edit((w) => {
        const entry: LogStyleAsset = { id, path: old?.path || styleAssetPath(file.name, mime, w.assets.map((a) => a.path)), mime, sha256: '' };
        w.assets = old ? w.assets.map((a) => (a.id === old.id ? entry : a)) : [...w.assets, entry];
      });
      toast.success(old ? `已替换 ${old.path}` : `已添加 ${file.name}`);
    }
  } catch (error) {
    toast.error(error instanceof Error && error.message ? error.message : '上传失败');
  } finally {
    assetBusy.value = false;
  }
};
const removeAsset = (id: string) => edit((w) => { w.assets = w.assets.filter((a) => a.id !== id); });

const assetBytes = async (a: LogStyleAsset): Promise<LocalAsset | null> => {
  const local = localAssets.value.get(a.id);
  if (local) return local;
  const base = copy.value?.base;
  if (!base || !base.assets.some((x) => x.id === a.id)) return null;
  const r = await fetchLogStyleAssetBytes(base.id, base.version, a.id);
  return { data: r.data, mime: r.mime };
};
const thumb = (a: LogStyleAsset) => {
  let task = thumbUrls.get(a.id);
  if (!task) {
    task = assetBytes(a)
      .then((b) => (b && /^image\/(png|jpeg|webp)$/.test(a.mime) ? URL.createObjectURL(new Blob([b.data], { type: a.mime })) : null))
      .catch(() => null);
    thumbUrls.set(a.id, task);
  }
  return task;
};
const sizeOf = (id: string) => localAssets.value.get(id)?.data.byteLength ?? null;

// ---- 保存 ----
const zipUnchanged = computed(() => !!zip.value && zip.value.fingerprint === fingerprint.value && !zip.value.issues.length);
const nextVersion = computed(() => {
  const c = copy.value;
  if (!c) return '';
  if (c.origin === 'version' && c.base) return nextStyleVersion(c.base.version, bump.value);
  if (c.origin === 'new') return '1.0.0';
  return c.version;
});
/** ZIP 保存目标：按包 ID 判断是新样式还是自己已有样式的新版本。 */
const zipTarget = computed(() => {
  const c = copy.value;
  if (!c || c.origin !== 'zip') return null;
  if (!STYLE_KEY.test(c.id)) return { kind: 'invalid' as const, message: '包 ID 须以字母开头，只含字母、数字、下划线或连字符' };
  if (!STYLE_SEMVER.test(c.version)) return { kind: 'invalid' as const, message: '版本须为三段数字，例如 1.0.0' };
  const existing = props.library.byId(c.id);
  if (existing?.builtin) return { kind: 'invalid' as const, message: `与内置样式「${existing.name}」的 ID 相同，不能导入为它的版本，请改用新 ID` };
  if (existing) {
    if (compareStyleVersions(c.version, existing.version) <= 0) return { kind: 'invalid' as const, message: `将成为「${existing.name}」的新版本，版本须高于当前最新的 ${existing.version}` };
    return { kind: 'update' as const, message: `保存后成为你的样式「${existing.name}」的新版本 ${c.version}（当前最新 ${existing.version}）` };
  }
  if (c.id.startsWith('sealchat-')) return { kind: 'invalid' as const, message: 'sealchat- 前缀保留给内置样式，请改用新 ID' };
  return { kind: 'create' as const, message: `保存后作为新的私有样式 ${c.id}@${c.version}；若该 ID 已被他人使用，服务端会拒绝` };
});
const canSave = computed(() => !!copy.value && !readonly.value && !busy.value && !checking.value && !!assembled.value.pkg
  && !allIssues.value.length && (copy.value.origin !== 'zip' || zipTarget.value?.kind !== 'invalid')
  && (dirty.value || copy.value.origin !== 'version'));

const downloadBlob = (blob: Blob, name: string) => {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
};

/** 保存成功后：工作副本改以新版本为基准，本地资源字节保留供预览使用。 */
const rebase = (saved: LogStylePackage) => {
  load(workingCopyFromPackage(saved, 'version'), { assets: localAssets.value, keepView: true });
  lastSaved.value = saved;
  if (livePreview.value) refreshPreview();
};

const describeSaveError = (error: unknown) => {
  if (isLogError(error, 'PERMISSION_DENIED')) return '没有权限保存到这个样式（只能保存自己的私有样式；ZIP 的包 ID 可能属于他人或内置样式，请改用新 ID）';
  if (isLogError(error, 'INVALID_TEMPLATE')) return '服务端校验未通过：样式包、模板、CSS 或资源无效。请检查问题列表，或在高级源码中核对。';
  return describeLogError(error, '保存失败');
};

const save = async (apply = false) => {
  const c = copy.value;
  const pkg = assembled.value.pkg;
  if (!c || !pkg || !canSave.value) return;
  busy.value = true;
  serverError.value = '';
  conflict.value = null;
  try {
    let saved: LogStylePackage;
    if (c.origin === 'version') {
      const base = c.base!;
      saved = await saveLogStyleVersion(base.id, base.version, { ...pkg, id: base.id, version: nextStyleVersion(base.version, bump.value) });
    } else if (c.origin === 'new') {
      saved = await createLogStyle(pkg);
    } else {
      const stage = zip.value!;
      let file = stage.file;
      if (!zipUnchanged.value) {
        const bytes: Record<string, Uint8Array> = {};
        for (const [id, a] of localAssets.value) bytes[id] = new Uint8Array(a.data);
        const data = await buildStyleZip(pkg, bytes, stage.extras);
        file = new File([data], `${pkg.id}-${pkg.version}.zip`, { type: 'application/zip' });
      }
      const r = await importLogStyleZip(file);
      saved = await fetchLogStylePackage(r.stylePackageId, r.version);
    }
    await props.library.reload();
    rebase(saved);
    const bound = props.boundPackage?.id === saved.id;
    toast.success(`已保存「${saved.name}」${saved.version}${apply ? '' : bound ? '；当前 LOG 仍使用旧版本，需要时点“应用到当前 LOG”' : ''}`);
    if (apply) await props.bind(saved);
  } catch (error) {
    if (isLogError(error, 'VERSION_CONFLICT') && c.origin === 'version') {
      await props.library.reload();
      conflict.value = { latest: props.library.byId(c.id)?.version || '' };
    } else if (isLogError(error, 'VERSION_CONFLICT')) {
      serverError.value = '该包 ID 已有相同或更高的版本，请提高版本号或改用新 ID';
    } else {
      serverError.value = describeSaveError(error);
    }
  } finally {
    busy.value = false;
  }
};

/** 冲突处理：显式以最新版本为基准保存（覆盖他处的改动），绝不静默进行。 */
const overwriteOnLatest = () => {
  const c = copy.value;
  const latest = conflict.value?.latest;
  if (!c || !latest) return;
  dialog.warning({
    title: `以 ${latest} 为基准保存？`,
    content: `你的修改基于 ${c.base?.version}。继续将以你的工作副本内容保存为 ${latest} 之后的新版本，他处在 ${latest} 中的改动会被你的内容取代（旧版本仍保留可用）。`,
    positiveText: '覆盖保存',
    negativeText: '取消',
    onPositiveClick: async () => {
      const pkg = await run('读取最新版本', () => fetchLogStylePackage(c.id, latest));
      if (!pkg || copy.value !== c) return;
      const divergedAssets = inheritedAssetsDivergedFromLatest(c, pkg);
      if (divergedAssets.length) {
        serverError.value = `最新版本已移除或替换当前工作副本继承的资源：${divergedAssets.map((a) => a.path).join('、')}。不能直接改用最新版本作为保存基准；请另存为新样式，或先移除这些资源。`;
        return;
      }
      c.base = pkg;
      conflict.value = null;
      await save();
    },
  });
};
const loadLatest = async () => {
  const c = copy.value;
  const latest = conflict.value?.latest;
  if (!c || !latest) return;
  baseline.value = fingerprint.value; // 用户明确选择放弃本地修改
  await openStyle({ id: c.id, version: latest });
};

/** 另存为新样式：既有包先 fork（服务端复制已保存的资源），再把工作副本保存为副本的新版本。 */
const saveAsNew = async () => {
  const c = copy.value;
  const pkg = assembled.value.pkg;
  if (!c || !pkg || readonly.value || allIssues.value.length) return;
  if (c.origin === 'new') return save();
  if (c.origin === 'zip') {
    edit((w) => {
      w.id = freshStyleId();
      w.version = '1.0.0';
    });
    return save();
  }
  const base = c.base!;
  busy.value = true;
  serverError.value = '';
  try {
    const forked = await forkLogStyle(base.id, base.version, `${pkg.name}（副本）`.slice(0, 200));
    const saved = await saveLogStyleVersion(forked.id, forked.version, { ...pkg, id: forked.id, name: forked.name, version: nextStyleVersion(forked.version) });
    await props.library.reload();
    rebase(saved);
    conflict.value = null;
    toast.success(`已另存为新样式「${saved.name}」`);
  } catch (error) {
    serverError.value = describeSaveError(error);
  } finally {
    busy.value = false;
  }
};

const applySaved = async () => {
  const base = copy.value?.base;
  if (base && !dirty.value) await props.bind(base);
};

const exportSaved = async (s: { id: string; version: string }) => {
  const blob = await run('导出', () => exportLogStyleZip(s.id, s.version));
  if (blob) downloadBlob(blob, `${s.id}-${s.version}.zip`);
};
/** 工作副本 ZIP：本地打包，未经服务端校验，用于备份或在别处继续编辑。 */
const downloadWorking = async () => {
  const c = copy.value;
  const pkg = assembled.value.pkg;
  if (!c || !pkg) {
    toast.warning('请先修正 JSON 源码中的错误');
    return;
  }
  await run('打包', async () => {
    const bytes: Record<string, Uint8Array> = {};
    for (const a of pkg.assets) {
      const b = await assetBytes(a);
      if (!b) throw new Error(`读取资源 ${a.path} 失败`);
      bytes[a.id] = new Uint8Array(b.data);
    }
    const data = await buildStyleZip({ ...pkg, id: pkg.id || 'my-style' }, bytes, zip.value?.extras);
    downloadBlob(new Blob([data], { type: 'application/zip' }), `${pkg.id || 'my-style'}-working.zip`);
  });
};

// ---- 样式库视图事件 ----
const onLibraryOpen = (s: LogStyleSummary) => void openStyle(s);
const onLibraryPreview = async (s: LogStyleSummary | null) => {
  if (!s) {
    clearPreview();
    return;
  }
  const pkg = await run('读取样式', () => fetchLogStylePackage(s.id, s.version));
  if (!pkg) return;
  session.value += 1;
  emit('preview', { pkg, assets: {}, session: session.value });
  libraryPreviewKey.value = `${s.id}@${s.version}`;
};
const onLibraryApply = async (s: LogStyleSummary) => {
  const pkg = await run('读取样式', () => fetchLogStylePackage(s.id, s.version));
  if (pkg && (await props.bind(pkg))) clearPreview();
};
const onLibraryNew = (kind: 'blank' | 'copy-bound') => {
  if (kind === 'blank') void newBlank();
  else if (props.boundPackage) askFork(props.boundPackage);
};

// ---- 视图 ----
const sections = computed(() => {
  const c = copy.value;
  const p = parsed.value;
  const tplIssues = Object.values(templateIssues.value).reduce((n, l) => n + l.length, 0);
  return [
    { key: 'overview' as const, label: '概览', icon: InfoCircle, count: '', warn: !!zip.value?.issues.length || !!serverError.value },
    { key: 'templates' as const, label: '模板', icon: Template, count: c ? String(Object.keys(c.templates).length) : '', warn: tplIssues > 0 },
    { key: 'css' as const, label: '外观 CSS', icon: Palette, count: '', warn: cssIssues.value.length > 0 },
    { key: 'tokens' as const, label: '设计变量', icon: Variable, count: p?.tokens ? String(Object.keys(p.tokens).length) : '!', warn: !!p?.errors.tokens },
    { key: 'rules' as const, label: '规则', icon: ListDetails, count: p?.rules ? String(p.rules.length) : '!', warn: !!p?.errors.rules },
    { key: 'assets' as const, label: '资源', icon: Photo, count: c ? String(c.assets.length) : '', warn: false },
    { key: 'source' as const, label: '高级源码', icon: Code, count: '', warn: !!p?.errors.schema },
  ];
});
const sectionLabel = computed(() => sections.value.find((s) => s.key === section.value)?.label || '');
const gotoTemplate = (id: string) => {
  activeTemplate.value = id;
  section.value = 'templates';
};

const originLabel = computed(() => {
  const c = copy.value;
  if (!c) return '';
  if (c.origin === 'new') return '新样式 · 尚未保存到样式库';
  if (c.origin === 'zip') return 'ZIP 暂存 · 尚未导入样式库';
  return readonly.value ? `只读 · ${c.id} @ ${c.base?.version}` : `基于 ${c.base?.version}`;
});
const boundLabel = computed(() => {
  const c = copy.value;
  const b = props.boundPackage;
  if (!c || !b || b.id !== c.id) return '当前 LOG 未使用此样式';
  return `当前 LOG 使用 ${b.version}`;
});
const boundIsBase = computed(() => !!copy.value?.base && props.boundPackage?.id === copy.value.id && props.boundPackage.version === copy.value.base.version);

const saveMenu = computed<DropdownOption[]>(() => [
  { key: 'apply', label: '保存并应用到当前 LOG', disabled: !props.canBind || !canSave.value },
  { key: 'new', label: '另存为新样式', disabled: readonly.value || !assembled.value.pkg || !!allIssues.value.length || busy.value || copy.value?.origin === 'new' },
  { key: 'download', label: '下载工作副本 ZIP（未经服务端校验）' },
]);
const onSaveMenu = (key: string) => {
  if (key === 'apply') void save(true);
  else if (key === 'new') void saveAsNew();
  else if (key === 'download') void downloadWorking();
};
const bumpOptions = computed(() => {
  const v = copy.value?.base?.version || '1.0.0';
  return [
    { label: `修订 ${nextStyleVersion(v, 'patch')}`, value: 'patch' },
    { label: `次版本 ${nextStyleVersion(v, 'minor')}`, value: 'minor' },
    { label: `主版本 ${nextStyleVersion(v, 'major')}`, value: 'major' },
  ];
});

// ---- 外观 CSS 与高级源码 ----
const cssEditor = ref<{ textareaElRef?: HTMLTextAreaElement | null } | null>(null);
const css = computed({
  get: () => copy.value?.css || '',
  set: (v: string) => edit((c) => { c.css = v; }),
});
const insertCss = (snippet: string) => {
  if (readonly.value) return;
  const el = cssEditor.value?.textareaElRef || null;
  const src = css.value;
  const start = el ? el.selectionStart : src.length;
  const end = el ? el.selectionEnd : src.length;
  css.value = src.slice(0, start) + snippet + src.slice(end);
  requestAnimationFrame(() => {
    el?.focus();
    el?.setSelectionRange(start + snippet.length, start + snippet.length);
  });
};

type SourceTab = 'template' | 'css' | 'tokens' | 'rules' | 'schema' | 'manifest';
const sourceTab = ref<SourceTab>('template');
const sourceTemplate = computed({
  get: () => (copy.value && activeTemplate.value in copy.value.templates ? activeTemplate.value : Object.keys(copy.value?.templates || {})[0] || ''),
  set: (v: string) => { activeTemplate.value = v; },
});
const sourceTemplateHtml = computed({
  get: () => (copy.value && sourceTemplate.value ? copy.value.templates[sourceTemplate.value] || '' : ''),
  set: (v: string) => {
    const id = sourceTemplate.value;
    if (id) edit((c) => { c.templates = { ...c.templates, [id]: v }; });
  },
});
const textField = (key: 'tokensText' | 'rulesText' | 'schemaText') => computed({
  get: () => copy.value?.[key] || '',
  set: (v: string) => edit((c) => { c[key] = v; }),
});
const tokensText = textField('tokensText');
const rulesText = textField('rulesText');
const schemaText = textField('schemaText');
const manifestPreview = computed(() => {
  const c = copy.value;
  if (!c) return '';
  return JSON.stringify({ schemaVersion: 1, id: c.id, version: nextVersion.value, name: c.name, license: c.license, minRendererVersion: c.minRendererVersion, assets: c.assets.map(({ id, path, mime }) => ({ id, path, mime })) }, null, 2);
});
const name = computed({ get: () => copy.value?.name || '', set: (v: string) => edit((c) => { c.name = v; }) });
const license = computed({ get: () => copy.value?.license || '', set: (v: string) => edit((c) => { c.license = v; }) });
const zipId = computed({ get: () => copy.value?.id || '', set: (v: string) => edit((c) => { c.id = v.trim(); }) });
const zipVersion = computed({ get: () => copy.value?.version || '', set: (v: string) => edit((c) => { c.version = v.trim(); }) });
const licenseOptions = ['MIT', 'Apache-2.0', 'CC0-1.0', 'CC-BY-4.0'];
const stats = computed(() => {
  const c = copy.value;
  const p = parsed.value;
  return [
    { key: 'templates' as const, label: '模板', value: c ? Object.keys(c.templates).length : 0 },
    { key: 'rules' as const, label: '消息规则', value: p?.rules?.length ?? '—' },
    { key: 'tokens' as const, label: '设计变量', value: p?.tokens ? Object.keys(p.tokens).length : '—' },
    { key: 'assets' as const, label: '资源', value: c?.assets.length || 0 },
  ];
});
</script>

<template>
  <section class="lss" :class="{ 'is-narrow': narrow }" aria-label="样式设计器">
    <header class="lss__head">
      <nav class="lss__crumbs" aria-label="样式设计器位置">
        <button type="button" class="lss__crumb" :class="{ 'is-current': !copy }" :disabled="!copy" @click="goLibrary">样式库</button>
        <template v-if="copy">
          <span aria-hidden="true">/</span>
          <span class="lss__crumb is-name" :title="copy.name">{{ copy.name || '未命名样式' }}</span>
          <span aria-hidden="true">/</span>
          <span class="lss__crumb is-current">{{ sectionLabel }}</span>
        </template>
      </nav>
      <div class="lss__head-actions">
        <n-button v-if="narrow" size="small" secondary title="查看 LOG 预览" @click="emit('peek')">
          <template #icon><n-icon :component="Eye" /></template>预览
        </n-button>
        <button type="button" class="lss__icon-btn" title="关闭样式设计器" aria-label="关闭样式设计器" @click="requestClose">
          <n-icon :component="X" size="18" />
        </button>
      </div>
    </header>

    <div class="lss__scroll">
      <LogStyleLibraryView
        v-if="!copy"
        :library="library"
        :bound-package="boundPackage"
        :previewing-key="libraryPreviewKey"
        :can-bind="canBind"
        :busy="busy"
        @open="onLibraryOpen"
        @fork="askFork"
        @preview="onLibraryPreview"
        @apply="onLibraryApply"
        @export="exportSaved"
        @new="onLibraryNew"
        @import="pickZip"
      />

      <template v-else>
        <div class="lss__status" role="status" aria-live="polite">
          <span class="lss__chip">{{ originLabel }}</span>
          <span v-if="!readonly" class="lss__chip" :class="dirty ? 'is-warn' : 'is-ok'">{{ dirty ? '有未保存修改' : copy.origin === 'version' ? '无未保存修改' : '尚未修改' }}</span>
          <span class="lss__chip" :class="checking ? '' : allIssues.length ? 'is-error' : 'is-ok'" title="前端快速检查；保存时以服务端校验为准">
            {{ checking ? '检查中…' : allIssues.length ? `${allIssues.length} 个问题` : '前端检查通过' }}
          </span>
          <span class="lss__chip" :class="{ 'is-accent': previewState === 'live', 'is-warn': previewState === 'stale' }">
            {{ previewState === 'live' ? (dirty ? '本地预览中（未保存）' : '本地预览中') : previewState === 'stale' ? '预览停在上次通过检查的结果' : '未预览' }}
          </span>
          <span class="lss__chip" :class="{ 'is-accent': boundIsBase }">{{ boundLabel }}</span>
          <span v-if="lastSaved" class="lss__chip is-ok">本次已保存 {{ lastSaved.version }}</span>
        </div>

        <div class="lss__alerts">
          <n-alert v-if="ownershipUnresolved" type="info" :show-icon="false">
            {{ library.error.value ? '暂时无法确认这个样式是否属于你，请刷新样式库后再编辑；不会自动复制。' : '正在确认这个样式是否属于你，确认完成后即可编辑；不会自动复制。' }}
          </n-alert>
          <n-alert v-else-if="readonly" type="info" :show-icon="false">
            {{ library.byId(copy.id)?.builtin || copy.id.startsWith('sealchat-') ? '内置样式不能直接修改。' : '这不是你的样式，只能查看。' }}复制成你的私有样式后即可编辑，原样式保持不变。
            <n-button size="tiny" type="primary" style="margin-left: 6px" :disabled="busy" @click="askFork({ id: copy.id, version: copy.base!.version, name: copy.name })">
              <template #icon><n-icon :component="Copy" /></template>复制后编辑
            </n-button>
          </n-alert>
          <n-alert v-if="conflict" type="error" title="版本冲突：样式已在别处保存了新版本">
            你的修改基于 {{ copy.base?.version }}，样式库中的最新版本是 {{ conflict.latest || '更高版本' }}。为避免覆盖他处的修改，本次没有保存。
            <n-space size="small" style="margin-top: 8px">
              <n-button size="tiny" :disabled="busy || !conflict.latest" @click="loadLatest">载入最新版本（放弃我的修改）</n-button>
              <n-button size="tiny" type="warning" :disabled="busy || !conflict.latest" @click="overwriteOnLatest">以最新版本为基准保存我的修改…</n-button>
              <n-button size="tiny" :disabled="busy" @click="saveAsNew">另存为新样式</n-button>
              <n-button size="tiny" quaternary @click="downloadWorking">下载工作副本</n-button>
            </n-space>
          </n-alert>
          <n-alert v-if="serverError" type="error" :show-icon="false" closable @close="serverError = ''">{{ serverError }}</n-alert>
        </div>

        <div class="lss__body">
          <nav class="lss__nav" role="tablist" aria-label="样式内容">
            <button
              v-for="s in sections"
              :key="s.key"
              type="button"
              role="tab"
              class="lss__nav-btn"
              :class="{ 'is-on': section === s.key }"
              :aria-selected="section === s.key"
              @click="section = s.key"
            >
              <n-icon :component="s.icon" size="16" />
              <span>{{ s.label }}</span>
              <span v-if="s.count" class="lss__nav-count">{{ s.count }}</span>
              <i v-if="s.warn" class="lss__nav-dot" aria-label="有问题" />
            </button>
          </nav>

          <div class="lss__pane" role="tabpanel" :aria-label="sectionLabel">
            <!-- 概览 -->
            <div v-if="section === 'overview'" class="lss__overview">
              <n-form size="small" label-placement="left" label-width="72" class="lss__form" @submit.prevent>
                <n-form-item label="名称"><n-input v-model:value="name" maxlength="200" :readonly="readonly" /></n-form-item>
                <n-form-item label="许可">
                  <n-input v-model:value="license" maxlength="200" :readonly="readonly" placeholder="如 MIT；他人复制或导出需要开放许可" />
                  <span v-if="!readonly" class="lss__licenses">
                    <button v-for="l in licenseOptions" :key="l" type="button" class="lss__snippet" @click="license = l">{{ l }}</button>
                  </span>
                </n-form-item>
                <n-form-item label="包 ID">
                  <template v-if="copy.origin === 'zip'">
                    <n-input v-model:value="zipId" maxlength="64" />
                    <n-button size="small" style="margin-left: 6px" @click="zipId = freshStyleId()">改用新 ID</n-button>
                  </template>
                  <span v-else class="lss__value">{{ copy.origin === 'new' ? '保存时由服务器分配' : copy.id }}</span>
                </n-form-item>
                <n-form-item label="版本">
                  <n-input v-if="copy.origin === 'zip'" v-model:value="zipVersion" maxlength="40" style="max-width: 160px" />
                  <span v-else-if="copy.origin === 'new'" class="lss__value">保存后为 1.0.0</span>
                  <span v-else class="lss__value">
                    当前基准 {{ copy.base?.version }}<template v-if="library.byId(copy.id) && library.byId(copy.id)!.version !== copy.base?.version">（样式库最新 {{ library.byId(copy.id)!.version }}）</template>
                    <template v-if="!readonly"> · 保存为
                      <n-select v-model:value="bump" size="tiny" :options="bumpOptions" style="width: 150px; display: inline-block; vertical-align: middle" />
                    </template>
                  </span>
                </n-form-item>
                <n-form-item label="渲染器"><span class="lss__value">需要 {{ copy.minRendererVersion }}，当前 {{ LOG_RENDERER_VERSION }}</span></n-form-item>
              </n-form>

              <n-alert v-if="zipTarget" :type="zipTarget.kind === 'invalid' ? 'warning' : 'info'" :show-icon="false">{{ zipTarget.message }}</n-alert>
              <section v-if="zip" class="lss__card">
                <h4>ZIP 检查：{{ zip.file.name }}</h4>
                <p class="lss__hint">共 {{ zip.files.length }} 个文件；资源 {{ copy.assets.length }} 个<template v-if="Object.keys(zip.extras).length">，预览图 {{ Object.keys(zip.extras).length }} 张（原样保留）</template>。</p>
                <template v-if="zip.issues.length">
                  <p class="lss__hint is-warn">原始 ZIP 直接导入会被服务器拒绝。下列问题已在工作副本中尽量修正，保存时会按修正后的内容重新打包：</p>
                  <ul class="lss__list is-warn"><li v-for="m in zip.issues" :key="m">{{ m }}</li></ul>
                </template>
                <ul v-if="zip.warnings.length" class="lss__list"><li v-for="m in zip.warnings" :key="m">{{ m }}</li></ul>
                <p v-if="!zip.issues.length" class="lss__hint">{{ zipUnchanged ? '未修改：保存时直接提交原 ZIP。' : '已修改：保存时按工作副本重新打包为相同格式的 ZIP。' }}</p>
              </section>

              <div class="lss__stats">
                <button v-for="s in stats" :key="s.key" type="button" class="lss__stat" @click="section = s.key">
                  <strong>{{ s.value }}</strong><span>{{ s.label }}</span>
                </button>
              </div>

              <section class="lss__card">
                <h4>当前 LOG</h4>
                <p class="lss__hint">保存样式只会产生样式库里的新版本，不会改变任何 LOG；需要时再把版本应用到当前 LOG。其他 LOG 也可以选用这个版本。</p>
                <p class="lss__hint">{{ boundLabel }}<template v-if="boundSame && boundPackage && copy.base && boundPackage.version !== copy.base.version">；当前编辑基于 {{ copy.base.version }}</template></p>
                <n-space size="small">
                  <n-button v-if="copy.base" size="small" :disabled="!canBind || dirty || boundIsBase || busy" :title="dirty ? '请先保存修改' : ''" @click="applySaved">
                    {{ boundIsBase ? '当前 LOG 已使用此版本' : `应用 ${copy.base.version} 到当前 LOG` }}
                  </n-button>
                  <n-button v-if="copy.base" size="small" quaternary :disabled="busy" @click="exportSaved(copy.base)">
                    <template #icon><n-icon :component="Download" /></template>导出已保存版本 ZIP
                  </n-button>
                  <n-button size="small" quaternary :disabled="busy" @click="downloadWorking">下载工作副本 ZIP</n-button>
                </n-space>
              </section>
            </div>

            <LogStyleTemplatesEditor
              v-else-if="section === 'templates'"
              v-model:active="activeTemplate"
              :copy="copy"
              :edit="edit"
              :readonly="readonly"
              :usage="usage"
              :issues="templateIssues"
              @goto="(s) => (section = s)"
            />

            <div v-else-if="section === 'css'" class="lss__css">
              <div class="lss__css-main">
                <n-input
                  ref="cssEditor"
                  v-model:value="css"
                  type="textarea"
                  class="lss__code"
                  :readonly="readonly"
                  :autosize="{ minRows: 18, maxRows: 40 }"
                  spellcheck="false"
                  aria-label="样式 CSS"
                />
                <ul v-if="cssIssues.length" class="lss__list is-warn"><li v-for="(m, i) in cssIssues" :key="i">{{ m }}</li></ul>
                <p class="lss__hint">不允许 @import、外部 url()、反斜杠与尖括号（含子选择器 &gt;）；url() 只能引用本样式的资源路径且不加引号。</p>
              </div>
              <aside class="lss__css-side" aria-label="插入引用">
                <h5>设计变量</h5>
                <p v-if="!parsed?.tokens || !Object.keys(parsed.tokens).length" class="lss__hint">没有可用变量</p>
                <button v-for="(v, k) in parsed?.tokens || {}" :key="k" type="button" class="lss__snippet" :disabled="readonly" @click="insertCss(tokenCssHint(String(k), v))">{{ tokenCssHint(String(k), v) }}</button>
                <h5>资源</h5>
                <p v-if="!copy.assets.length" class="lss__hint">没有资源 <button type="button" class="lss__link" @click="section = 'assets'">去上传</button></p>
                <button v-for="a in copy.assets" :key="a.id" type="button" class="lss__snippet" :disabled="readonly" @click="insertCss(`url(${a.path})`)">url({{ a.path }})</button>
                <h5>渲染器变量</h5>
                <button v-for="v in ['var(--log-font-family)', 'var(--log-font-size)', 'var(--log-line-height)', 'var(--log-page-bg)']" :key="v" type="button" class="lss__snippet" :disabled="readonly" @click="insertCss(v)">{{ v }}</button>
              </aside>
            </div>

            <LogStyleTokensEditor
              v-else-if="section === 'tokens'"
              :edit="edit"
              :readonly="readonly"
              :tokens="parsed?.tokens || null"
              :error="parsed?.errors.tokens || ''"
              @goto="sourceTab = 'tokens'; section = 'source'"
            />

            <LogStyleRulesEditor
              v-else-if="section === 'rules'"
              :edit="edit"
              :readonly="readonly"
              :rules="parsed?.rules || null"
              :error="parsed?.errors.rules || ''"
              :template-ids="Object.keys(copy.templates).sort()"
              :usage="usage"
              @goto="sourceTab = 'rules'; section = 'source'"
              @template="gotoTemplate"
            />

            <LogStyleAssetsEditor
              v-else-if="section === 'assets'"
              :key="session"
              :assets="copy.assets"
              :readonly="readonly"
              :busy="assetBusy"
              :thumb="thumb"
              :size-of="sizeOf"
              :css-refs="cssRefs"
              :draft-refs="draftAssetRefs"
              @upload="(files) => addAssets(files)"
              @replace="(id, file) => addAssets([file], id)"
              @remove="removeAsset"
            />

            <div v-else class="lss__source">
              <n-radio-group v-model:value="sourceTab" size="small" class="lss__source-tabs">
                <n-radio-button value="template">模板 HTML</n-radio-button>
                <n-radio-button value="css">CSS</n-radio-button>
                <n-radio-button value="tokens">tokens.json</n-radio-button>
                <n-radio-button value="rules">rules</n-radio-button>
                <n-radio-button value="schema">settings.schema.json</n-radio-button>
                <n-radio-button value="manifest">manifest</n-radio-button>
              </n-radio-group>
              <p class="lss__hint">高级源码与上方各页编辑的是同一份工作副本；JSON 有语法错误时，对应的可视化编辑会暂停，修正后自动恢复。</p>
              <template v-if="sourceTab === 'template'">
                <n-select v-model:value="sourceTemplate" size="small" filterable style="max-width: 260px" :options="Object.keys(copy.templates).sort().map((id) => ({ label: id, value: id }))" />
                <n-input v-model:value="sourceTemplateHtml" type="textarea" class="lss__code" :readonly="readonly" :autosize="{ minRows: 14, maxRows: 40 }" spellcheck="false" aria-label="模板 HTML 源码" />
                <ul v-if="templateIssues[sourceTemplate]?.length" class="lss__list is-warn"><li v-for="(m, i) in templateIssues[sourceTemplate]" :key="i">{{ m }}</li></ul>
              </template>
              <template v-else-if="sourceTab === 'css'">
                <n-input v-model:value="css" type="textarea" class="lss__code" :readonly="readonly" :autosize="{ minRows: 14, maxRows: 40 }" spellcheck="false" aria-label="CSS 源码" />
              </template>
              <template v-else-if="sourceTab === 'tokens'">
                <n-input v-model:value="tokensText" type="textarea" class="lss__code" :readonly="readonly" :status="parsed?.errors.tokens ? 'error' : undefined" :autosize="{ minRows: 10, maxRows: 40 }" spellcheck="false" aria-label="tokens.json" />
                <p v-if="parsed?.errors.tokens" class="lss__hint is-warn">{{ parsed.errors.tokens }}</p>
              </template>
              <template v-else-if="sourceTab === 'rules'">
                <n-input v-model:value="rulesText" type="textarea" class="lss__code" :readonly="readonly" :status="parsed?.errors.rules ? 'error' : undefined" :autosize="{ minRows: 10, maxRows: 40 }" spellcheck="false" aria-label="rules JSON" />
                <p v-if="parsed?.errors.rules" class="lss__hint is-warn">{{ parsed.errors.rules }}</p>
              </template>
              <template v-else-if="sourceTab === 'schema'">
                <n-input v-model:value="schemaText" type="textarea" class="lss__code" :readonly="readonly" :status="parsed?.errors.schema ? 'error' : undefined" :autosize="{ minRows: 10, maxRows: 40 }" spellcheck="false" aria-label="settings.schema.json" />
                <p v-if="parsed?.errors.schema" class="lss__hint is-warn">{{ parsed.errors.schema }}</p>
                <p class="lss__hint">键为模板 ID；每项须是 additionalProperties:false 的 object，参数放在 properties.props 下，在模板中以 props.* 引用。</p>
              </template>
              <template v-else>
                <n-input :value="manifestPreview" type="textarea" class="lss__code" readonly :autosize="{ minRows: 8, maxRows: 30 }" aria-label="manifest 预览" />
                <p class="lss__hint">manifest 由概览中的名称、许可、版本与资源列表生成；资源的 SHA-256 由服务端计算。</p>
              </template>
            </div>
          </div>
        </div>
      </template>
    </div>

    <footer v-if="copy" class="lss__foot">
      <n-popover trigger="click" placement="top-start" :disabled="!allIssues.length" style="max-width: 420px">
        <template #trigger>
          <button type="button" class="lss__issues-btn" :class="{ 'is-error': allIssues.length }" :disabled="!allIssues.length">
            <n-icon :component="allIssues.length ? AlertTriangle : Braces" size="15" />
            {{ allIssues.length ? `${allIssues.length} 个问题` : '无问题' }}
          </button>
        </template>
        <ul class="lss__list is-warn lss__issue-list"><li v-for="(m, i) in allIssues" :key="i">{{ m }}</li></ul>
      </n-popover>
      <label class="lss__live">
        <n-switch v-model:value="livePreview" size="small" />
        <span>实时预览</span>
      </label>
      <n-button v-if="!livePreview" size="small" quaternary :disabled="checking" @click="refreshPreview(true)">预览一次</n-button>
      <n-button v-if="previewState !== 'off'" size="small" quaternary @click="stopPreview">停止预览</n-button>
      <span class="lss__spacer" />
      <template v-if="readonly">
        <n-button size="small" type="primary" :disabled="busy" @click="askFork({ id: copy.id, version: copy.base!.version, name: copy.name })">
          <template #icon><n-icon :component="Copy" /></template>复制后编辑
        </n-button>
      </template>
      <template v-else>
        <n-button v-if="copy.base && !dirty && !boundIsBase" size="small" :disabled="!canBind || busy" @click="applySaved">应用到当前 LOG</n-button>
        <n-button-group size="small">
          <n-button type="primary" :loading="busy" :disabled="!canSave" @click="save()">
            <template #icon><n-icon :component="DeviceFloppy" /></template>
            {{ copy.origin === 'version' ? `保存为 ${nextVersion}` : copy.origin === 'zip' ? '导入到样式库' : '保存到样式库' }}
          </n-button>
          <n-dropdown trigger="click" placement="top-end" :options="saveMenu" @select="onSaveMenu">
            <n-button type="primary" secondary aria-label="更多保存方式">▾</n-button>
          </n-dropdown>
        </n-button-group>
      </template>
    </footer>

    <input ref="zipInput" type="file" accept=".zip,application/zip" hidden @change="onZipFile">

    <n-modal :show="forkOpen" preset="card" title="复制为我的样式" style="width: min(440px, calc(100vw - 32px))" :mask-closable="!busy" @update:show="(v: boolean) => { if (!v) forkOpen = false; }">
      <div class="lss__fork">
        <p class="lss__hint">将复制 {{ forkTarget?.id }} @ {{ forkTarget?.version }}（含资源）为你的私有样式 1.0.0，原样式保持不变。</p>
        <n-input v-model:value="forkName" maxlength="200" placeholder="新样式名称" @keydown.enter.prevent="doFork" />
        <n-space justify="end" size="small">
          <n-button size="small" @click="forkOpen = false">取消</n-button>
          <n-button size="small" type="primary" :loading="busy" :disabled="!forkName.trim()" @click="doFork">复制并编辑</n-button>
        </n-space>
      </div>
    </n-modal>
  </section>
</template>

<style scoped>
.lss {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--sc-bg-elevated, #fff);
  color: var(--sc-text-primary, #222);
}

.lss__head {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px 8px 16px;
  border-bottom: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.08));
}

.lss__crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 13px;
  color: var(--sc-text-secondary, #666);
}

.lss__crumb {
  padding: 2px 4px;
  border: 0;
  border-radius: 4px;
  background: none;
  color: inherit;
  font: inherit;
  white-space: nowrap;
}

button.lss__crumb:not(:disabled) {
  color: var(--primary-color, #2f6fde);
  cursor: pointer;
}

.lss__crumb.is-name {
  overflow: hidden;
  max-width: 260px;
  text-overflow: ellipsis;
}

.lss__crumb.is-current {
  color: var(--sc-text-primary, #222);
  font-weight: 600;
}

.lss__head-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.lss__icon-btn {
  display: inline-grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 8px;
  background: none;
  color: var(--sc-text-secondary, #666);
  cursor: pointer;
}

.lss__icon-btn:hover {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
}

.lss__scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.lss__status {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 10px 16px 0;
}

.lss__chip {
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 12px;
  line-height: 20px;
  color: var(--sc-text-secondary, #555);
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
}

.lss__chip.is-ok {
  color: #2f7a46;
  background: rgba(47, 122, 70, 0.1);
}

.lss__chip.is-warn {
  color: #9a5b00;
  background: rgba(217, 154, 30, 0.16);
}

.lss__chip.is-error {
  color: #b42318;
  background: rgba(180, 35, 24, 0.1);
}

.lss__chip.is-accent {
  color: var(--primary-color, #2f6fde);
  background: color-mix(in srgb, var(--primary-color, #2f6fde) 12%, transparent);
}

.lss__alerts {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 16px 0;
}

.lss__alerts:empty {
  display: none;
}

.lss__body {
  display: flex;
  gap: 0;
  min-height: 0;
  padding: 12px 0 16px;
}

.lss__nav {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 132px;
  padding: 0 8px 0 12px;
}

.lss__nav-btn {
  position: relative;
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 7px 8px;
  border: 0;
  border-radius: 8px;
  background: none;
  color: var(--sc-text-secondary, #555);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.lss__nav-btn:hover {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
}

.lss__nav-btn.is-on {
  color: var(--primary-color, #2f6fde);
  font-weight: 600;
  background: color-mix(in srgb, var(--primary-color, #2f6fde) 11%, transparent);
}

.lss__nav-btn:focus-visible,
.lss__stat:focus-visible,
.lss__snippet:focus-visible,
.lss__issues-btn:focus-visible {
  outline: 2px solid var(--primary-color, #2f6fde);
  outline-offset: -2px;
}

.lss__nav-count {
  margin-left: auto;
  font-size: 11px;
  font-weight: 400;
  color: var(--sc-text-secondary, #888);
}

.lss__nav-dot {
  position: absolute;
  top: 6px;
  left: 4px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #d99a1e;
}

.lss__pane {
  flex: 1;
  min-width: 0;
  padding: 0 16px 0 8px;
}

.lss__overview {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 760px;
}

.lss__form :deep(.n-form-item-blank) {
  flex-wrap: wrap;
}

.lss__value {
  font-size: 13px;
  overflow-wrap: anywhere;
}

.lss__licenses {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  width: 100%;
  margin-top: 4px;
}

.lss__card {
  padding: 10px 12px;
  border: 1px solid var(--sc-border-mute, #e5e5e5);
  border-radius: 10px;
}

.lss__card h4 {
  margin: 0 0 6px;
  font-size: 13px;
}

.lss__card .lss__hint + .lss__hint {
  margin-top: 4px;
}

.lss__card .n-space {
  margin-top: 8px;
}

.lss__stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.lss__stat {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: 10px 12px;
  border: 1px solid var(--sc-border-mute, #e5e5e5);
  border-radius: 10px;
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
}

.lss__stat strong {
  font-size: 18px;
}

.lss__stat span {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.lss__css {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 210px;
  gap: 12px;
}

.lss__css-main {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.lss__css-side {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-width: 0;
}

.lss__css-side h5 {
  margin: 8px 0 2px;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.lss__css-side h5:first-child {
  margin-top: 0;
}

.lss__snippet {
  max-width: 100%;
  overflow: hidden;
  padding: 2px 7px;
  border: 0;
  border-radius: 6px;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
  color: inherit;
  font: 11px/1.6 "SFMono-Regular", Consolas, monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.lss__snippet:disabled {
  cursor: default;
  opacity: 0.6;
}

.lss__source {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.lss__source-tabs {
  flex-wrap: wrap;
}

.lss__code :deep(textarea) {
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
}

.lss__list {
  margin: 4px 0;
  padding-left: 18px;
  font-size: 12px;
}

.lss__list.is-warn,
.lss__hint.is-warn {
  color: #9a5b00;
}

.lss__issue-list {
  max-height: 50vh;
  overflow: auto;
}

.lss__hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--sc-text-secondary, #666);
}

.lss__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--primary-color, #2f6fde);
  font: inherit;
  cursor: pointer;
}

.lss__foot {
  flex: none;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-top: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.08));
  background: var(--sc-bg-elevated, #fff);
}

.lss__issues-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border: 0;
  border-radius: 6px;
  background: none;
  color: var(--sc-text-secondary, #666);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.lss__issues-btn.is-error {
  color: #9a5b00;
  background: rgba(217, 154, 30, 0.14);
}

.lss__issues-btn:disabled {
  cursor: default;
}

.lss__live {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  cursor: pointer;
}

.lss__spacer {
  flex: 1;
}

.lss__fork {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* 窄屏：导航改为横向标签，CSS 辅助栏落到下方 */
.lss.is-narrow .lss__body {
  flex-direction: column;
  gap: 10px;
}

.lss.is-narrow .lss__nav {
  flex-direction: row;
  width: auto;
  padding: 0 12px;
  overflow-x: auto;
  scrollbar-width: none;
}

.lss.is-narrow .lss__nav-btn {
  flex: none;
}

.lss.is-narrow .lss__pane {
  padding: 0 14px;
}

.lss.is-narrow .lss__stats {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.lss.is-narrow .lss__css {
  grid-template-columns: minmax(0, 1fr);
}

.lss.is-narrow .lss__css-side {
  flex-direction: row;
  flex-wrap: wrap;
}

.lss.is-narrow .lss__css-side h5 {
  flex-basis: 100%;
}

@media (max-width: 1180px) {
  .lss:not(.is-narrow) .lss__css {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
