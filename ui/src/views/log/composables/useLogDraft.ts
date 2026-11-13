// 草稿控制器：整文档 CAS 保存，一次一个在途 PUT；后续编辑排队合并。
// 保存成功但本地已有新编辑时只推进基版本并继续保存，不用旧响应覆盖新输入。
import { computed, onBeforeUnmount, ref, shallowRef, type Ref } from 'vue';
import { isLogError, saveLogDraft, describeLogError } from '@/services/log/logApi';
import type { LogDraft } from '@/types/log';
import { cloneJson } from '../logDraftOps';

export type DraftSaveStatus = 'saved' | 'dirty' | 'saving' | 'error' | 'conflict';

const DEBOUNCE_MS = 800;
const MAX_DRAFT_BYTES = 2 * 1024 * 1024;

export const useLogDraft = (projectId: Ref<string>, initial: { draftVersion: number; draft: LogDraft }) => {
  const draft = shallowRef<LogDraft>(cloneJson(initial.draft));
  const draftVersion = ref(initial.draftVersion);
  const status = ref<DraftSaveStatus>('saved');
  const lastError = ref('');
  const conflictVersion = ref<number | null>(null);
  let savedSnapshot = JSON.stringify(initial.draft);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let inFlight: Promise<void> | null = null;
  let disposed = false;
  const paused = ref(false);
  /** 同一工程版本链上的其他 CAS 操作（排除集、发布、下架）与草稿保存串行执行。 */
  let chain: Promise<unknown> = Promise.resolve();

  const dirty = computed(() => status.value === 'dirty' || status.value === 'saving' || status.value === 'error');

  const schedule = () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      if (!paused.value) void save();
    }, DEBOUNCE_MS);
  };

  /** 以不可变方式修改草稿；外部只能通过这里写入。 */
  const update = (mutate: (next: LogDraft) => void) => {
    if (status.value === 'conflict') return false;
    const working = cloneJson(draft.value);
    mutate(working);
    // 调用方可能把 ref/reactive 对象写进草稿；序列化一次剥离代理，保证草稿始终是纯数据。
    const json = JSON.stringify(working);
    draft.value = JSON.parse(json) as LogDraft;
    if (json !== savedSnapshot) {
      if (status.value !== 'saving') status.value = 'dirty';
      schedule();
    }
    return true;
  };

  const save = (): Promise<void> => {
    if (disposed || status.value === 'conflict') return Promise.resolve();
    if (inFlight) return inFlight.then(() => (JSON.stringify(draft.value) !== savedSnapshot ? save() : undefined));
    const snapshot = JSON.stringify(draft.value);
    if (snapshot === savedSnapshot) {
      if (status.value !== 'error') status.value = 'saved';
      return Promise.resolve();
    }
    if (new Blob([snapshot]).size > MAX_DRAFT_BYTES) {
      status.value = 'error';
      lastError.value = '草稿超过 2MB，无法保存；请减少元素或长文本';
      return Promise.resolve();
    }
    const expected = draftVersion.value;
    const payload = JSON.parse(snapshot) as LogDraft;
    status.value = 'saving';
    const run = async () => {
      try {
        const result = await saveLogDraft(projectId.value, expected, payload);
        if (disposed) return;
        draftVersion.value = result.draftVersion;
        savedSnapshot = snapshot;
        lastError.value = '';
        status.value = JSON.stringify(draft.value) === savedSnapshot ? 'saved' : 'dirty';
      } catch (error) {
        if (disposed) return;
        if (isLogError(error, 'VERSION_CONFLICT')) {
          status.value = 'conflict';
          const v = Number((error as any).details?.draftVersion);
          conflictVersion.value = Number.isFinite(v) ? v : null;
          lastError.value = '工程已在别处被修改（其他设计者、管理设置或发布），本地修改未保存';
        } else {
          status.value = 'error';
          lastError.value = describeLogError(error, '保存失败');
        }
      }
    };
    inFlight = run().finally(() => {
      inFlight = null;
    });
    return inFlight.then(() => {
      if (!disposed && status.value === 'dirty') schedule();
    });
  };

  /** 立即保存待写入的修改并等待结果（发布、排除等操作前调用）。 */
  const flush = async () => {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    await save();
    if (inFlight) await inFlight;
    return status.value === 'saved';
  };

  /**
   * 串行执行会推进工程版本的其他 CAS 操作：先落盘草稿，再以当前版本执行，成功后采纳新版本。
   */
  const runVersioned = <T>(op: (expectedVersion: number) => Promise<{ value: T; draftVersion: number | null }>): Promise<T> => {
    const task = chain.then(async () => {
      const ok = await flush();
      if (!ok) throw new Error(lastError.value || (status.value === 'conflict' ? '草稿存在冲突，请先处理' : '草稿尚未保存成功，请稍后重试'));
      try {
        const { value, draftVersion: next } = await op(draftVersion.value);
        if (next !== null) draftVersion.value = next;
        return value;
      } catch (error) {
        if (isLogError(error, 'VERSION_CONFLICT')) {
          status.value = 'conflict';
          lastError.value = '工程版本已变化，请重新加载';
        }
        throw error;
      }
    });
    chain = task.catch(() => undefined);
    return task;
  };

  /** 冲突后由用户选择重新加载：丢弃本地未保存修改，采用服务端版本。 */
  const reset = (next: { draftVersion: number; draft: LogDraft }) => {
    if (timer) clearTimeout(timer);
    timer = null;
    draft.value = cloneJson(next.draft);
    draftVersion.value = next.draftVersion;
    savedSnapshot = JSON.stringify(next.draft);
    conflictVersion.value = null;
    lastError.value = '';
    status.value = 'saved';
  };

  /** 发布任务运行期间暂停自动保存，避免草稿 CAS 打断发布的版本检查。 */
  const pause = () => {
    paused.value = true;
  };
  const resume = () => {
    paused.value = false;
    if (status.value === 'dirty') schedule();
  };
  /**
   * 已确认的服务端操作把版本从 from 推进到 to 且未改变草稿内容（例如发布完成只做版本 +1）。
   * 只有本地基版本仍是 from 时才采纳；本地未保存的修改保留，随后以新版本继续保存。
   */
  const adoptVersion = (from: number, to: number) => {
    if (draftVersion.value === from && to > from && status.value !== 'conflict') {
      draftVersion.value = to;
      return true;
    }
    return false;
  };

  const localSnapshotJson = () => JSON.stringify(draft.value, null, 2);

  const onBeforeUnload = (e: BeforeUnloadEvent) => {
    if (status.value === 'dirty' || status.value === 'saving' || status.value === 'conflict') {
      e.preventDefault();
      e.returnValue = '';
    }
  };
  window.addEventListener('beforeunload', onBeforeUnload);

  onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', onBeforeUnload);
    if (timer) {
      clearTimeout(timer);
      timer = null;
      // 路由离开时尽力保存一次；不等待结果。
      if (status.value === 'dirty') void save();
    }
    disposed = true;
  });

  return {
    draft,
    draftVersion,
    status,
    lastError,
    conflictVersion,
    dirty,
    update,
    save,
    flush,
    runVersioned,
    reset,
    pause,
    resume,
    paused,
    adoptVersion,
    localSnapshotJson,
  };
};

export type LogDraftController = ReturnType<typeof useLogDraft>;
