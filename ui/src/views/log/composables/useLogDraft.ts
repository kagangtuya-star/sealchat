// 草稿控制器：整文档 CAS 保存，一次一个在途 PUT；后续编辑排队合并。
// 保存成功但本地已有新编辑时只推进基版本并继续保存，不用旧响应覆盖新输入。
// 多人协作：版本冲突或收到他人保存的通知时，先把本地修改三方合并到服务端最新版本上；
// 只有同一处被双方改成不同的值才进入冲突状态。
import { computed, onBeforeUnmount, ref, shallowRef, type Ref } from 'vue';
import { isLogError, saveLogDraft, describeLogError, LogDomainError } from '@/services/log/logApi';
import type { LogDraft } from '@/types/log';
import type { DraftPlacement } from '../logDropOps';
import { cloneJson } from '../logDraftOps';
import { mergeLogDrafts } from '../logDraftMerge';

export type DraftSaveStatus = 'saved' | 'dirty' | 'saving' | 'error' | 'conflict';

const DEBOUNCE_MS = 800;
const MAX_DRAFT_BYTES = 2 * 1024 * 1024;
/** 连续合并后仍保存冲突的上限，防止与他人的高频保存无限追赶。 */
const MAX_REBASE_STREAK = 5;

export type LatestLogDraft = { projectId: string; draftVersion: number; draft: LogDraft };

export interface LogDraftOptions {
  /** 读取服务端最新草稿；提供后，保存或版本操作冲突时先尝试合并本地修改。 */
  fetchLatest?: () => Promise<LatestLogDraft>;
  /** 冲突后合并了服务端版本（不经过 invalidate）时调用，调用方据此刷新依赖已保存版本的状态。 */
  onRebased?: () => void;
}

export const useLogDraft = (projectId: Ref<string>, initial: { draftVersion: number; draft: LogDraft }, options: LogDraftOptions = {}) => {
  const draft = shallowRef<LogDraft>(cloneJson(initial.draft));
  const draftVersion = ref(initial.draftVersion);
  const status = ref<DraftSaveStatus>('saved');
  const lastError = ref('');
  const conflictVersion = ref<number | null>(null);
  let savedSnapshot = JSON.stringify(initial.draft);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let inFlight: Promise<void> | null = null;
  let elementWrite: Promise<void> | null = null;
  let disposed = false;
  const paused = ref(false);
  /** 同一工程版本链上的其他 CAS 操作（排除集、发布、下架）与草稿保存串行执行。 */
  let chain: Promise<unknown> = Promise.resolve();
  let rebaseStreak = 0;

  /** 状态会被其他异步流程改写；await 之后用函数读取，避免沿用 await 之前的类型收窄。 */
  const inConflict = () => status.value === 'conflict';

  const dirty = computed(() => status.value === 'dirty' || status.value === 'saving' || status.value === 'error');

  const schedule = () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      if (!paused.value && !elementWrite) void save();
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

  /**
   * 把本地修改合并到服务端最新版本：基版本为最近一次保存成功的快照。
   * 合并成功后采纳最新版本号，剩余的本地差异继续按普通保存流程写入；无法合并时进入冲突状态。
   */
  const adoptLatest = (latest: LatestLogDraft) => {
    const result = mergeLogDrafts(JSON.parse(savedSnapshot) as LogDraft, draft.value, latest.draft);
    if (!result.ok) {
      status.value = 'conflict';
      conflictVersion.value = latest.draftVersion;
      const shown = result.conflicts.slice(0, 3).join('、');
      lastError.value = `你和其他设计者同时修改了${shown}${result.conflicts.length > 3 ? '等' : ''}，本地修改未保存`;
      return false;
    }
    savedSnapshot = JSON.stringify(latest.draft);
    draftVersion.value = latest.draftVersion;
    const json = JSON.stringify(result.draft);
    draft.value = JSON.parse(json) as LogDraft;
    lastError.value = '';
    status.value = json === savedSnapshot ? 'saved' : 'dirty';
    return true;
  };

  /** 冲突后读取最新版本并合并；已与最新版本同步也视为成功。 */
  const rebaseFromServer = async () => {
    const fetchLatest = options.fetchLatest;
    if (!fetchLatest || rebaseStreak >= MAX_REBASE_STREAK) return false;
    rebaseStreak += 1;
    const pid = projectId.value;
    try {
      const latest = await fetchLatest();
      if (disposed || status.value === 'conflict' || projectId.value !== pid || latest.projectId !== pid) return false;
      if (latest.draftVersion <= draftVersion.value) return true;
      return adoptLatest(latest);
    } catch {
      return false;
    }
  };

  const save = (): Promise<void> => {
    if (disposed || status.value === 'conflict') return Promise.resolve();
    if (elementWrite) return elementWrite.then(() => save());
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
        if (disposed || status.value === 'conflict') return;
        draftVersion.value = result.draftVersion;
        savedSnapshot = snapshot;
        rebaseStreak = 0;
        lastError.value = '';
        status.value = JSON.stringify(draft.value) === savedSnapshot ? 'saved' : 'dirty';
      } catch (error) {
        if (disposed || status.value === 'conflict') return;
        if (isLogError(error, 'VERSION_CONFLICT')) {
          if (await rebaseFromServer()) {
            if (status.value === 'saving') status.value = JSON.stringify(draft.value) === savedSnapshot ? 'saved' : 'dirty';
            options.onRebased?.();
            return;
          }
          if (disposed || inConflict()) return;
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
        if (next !== null && next > draftVersion.value) draftVersion.value = next;
        return value;
      } catch (error) {
        if (isLogError(error, 'VERSION_CONFLICT') && status.value !== 'conflict') {
          // 期间可能有本地自动保存在途：等它落定，避免两次合并互相覆盖。
          if (inFlight) await inFlight;
          if (!inConflict() && await rebaseFromServer()) {
            if (status.value === 'dirty' && !paused.value && !timer) schedule();
            options.onRebased?.();
            throw new LogDomainError('VERSION_CONFLICT', '工程刚被其他设计者更新，已同步最新版本，请重试');
          }
          if (!inConflict()) {
            status.value = 'conflict';
            lastError.value = '工程版本已变化，请重新加载';
          }
        }
        throw error;
      }
    });
    chain = task.catch(() => undefined);
    return task;
  };

  /** 采纳排序事务已保存的锚点；期间的新正文/样式编辑及新的锚点编辑仍保留为待保存。 */
  const placeElements = (placements: DraftPlacement[], op: (expectedVersion: number) => Promise<number>, afterCommit?: () => void) => runVersioned(async (expected) => {
    let finishWrite!: () => void;
    elementWrite = new Promise<void>((resolve) => { finishWrite = resolve; });
    try {
      const baseline = JSON.parse(savedSnapshot) as LogDraft;
      const nextVersion = await op(expected);
      if (status.value === 'conflict') throw new Error(lastError.value);
      const next = cloneJson(draft.value);
      const savedElements = new Map(baseline.elements.map((e) => [e.id, e]));
      const currentElements = new Map(next.elements.map((e) => [e.id, e]));
      for (const placement of placements) {
        const old = savedElements.get(placement.elementId);
        const current = currentElements.get(placement.elementId);
        if (!old) continue;
        if (current && current.order === old.order && JSON.stringify(current.anchor) === JSON.stringify(old.anchor)) {
          current.anchor = cloneJson(placement.anchor);
          current.order = placement.order;
        }
        old.anchor = cloneJson(placement.anchor);
        old.order = placement.order;
      }
      savedSnapshot = JSON.stringify(baseline);
      draftVersion.value = nextVersion;
      draft.value = next;
      afterCommit?.();
      status.value = JSON.stringify(next) === savedSnapshot ? 'saved' : 'dirty';
      return { value: undefined, draftVersion: nextVersion };
    } finally {
      elementWrite = null;
      finishWrite();
      if (!paused.value && status.value === 'dirty') schedule();
    }
  });

  /** 冲突后由用户选择重新加载：丢弃本地未保存修改，采用服务端版本。 */
  const reset = (next: { draftVersion: number; draft: LogDraft }) => {
    if (timer) clearTimeout(timer);
    timer = null;
    draft.value = cloneJson(next.draft);
    draftVersion.value = next.draftVersion;
    savedSnapshot = JSON.stringify(next.draft);
    rebaseStreak = 0;
    conflictVersion.value = null;
    lastError.value = '';
    status.value = 'saved';
  };

  let newestNotice = 0;
  /**
   * 收到版本推进通知：本地无修改时直接采用最新版本；有修改时合并到最新版本上。
   * 在途写入可能就是这条通知的来源（HTTP 保存的回声会先于响应到达），先等它落定再比较版本。
   * 返回 true 表示已采纳新版本。
   */
  const invalidate = async (version: number, fetch: () => Promise<LatestLogDraft>) => {
    if (disposed || !Number.isSafeInteger(version) || version <= draftVersion.value) return false;
    newestNotice = Math.max(version, newestNotice);
    while (inFlight || elementWrite) {
      await (elementWrite || inFlight);
      if (disposed) return false;
    }
    if (version < newestNotice || version <= draftVersion.value || status.value === 'conflict') return false;
    const pid = projectId.value;
    const conflict = (message: string) => {
      status.value = 'conflict';
      conflictVersion.value = newestNotice;
      lastError.value = message;
      return false;
    };
    try {
      const next = await fetch();
      if (disposed || projectId.value !== pid || version < newestNotice || draftVersion.value >= newestNotice) return false;
      if (next.projectId !== pid || next.draftVersion < newestNotice) return conflict('工程草稿已变化，未能取得对应版本，请重新加载');
      // 读取期间开始的新写入会由自己的 CAS 检出冲突并合并。
      if (inFlight || elementWrite || inConflict()) return false;
      if (status.value === 'saved') {
        reset(next);
        return true;
      }
      if (!adoptLatest(next)) return false;
      if (status.value === 'dirty' && !paused.value) schedule();
      return true;
    } catch {
      if (disposed || projectId.value !== pid || draftVersion.value >= version || version < newestNotice) return false;
      // 本地有修改时保留：下一次保存的 CAS 冲突会再次尝试合并。
      if (status.value !== 'saved') return false;
      return conflict('工程草稿已在别处变化，读取最新版本失败，请重新加载');
    }
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
    placeElements,
    reset,
    invalidate,
    pause,
    resume,
    paused,
    adoptVersion,
    localSnapshotJson,
  };
};

export type LogDraftController = ReturnType<typeof useLogDraft>;
