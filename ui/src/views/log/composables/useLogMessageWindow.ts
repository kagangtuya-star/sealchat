// 章节消息窗口：按章节分页拉取（不拉全世界），游标与 sourceVersion 绑定；
// SOURCE_CHANGED 时丢弃旧游标整体重建，绝不拼接不同版本的页。
import { computed, onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue';
import { chatEvent } from '@/stores/chat';
import { fetchLogSectionMessages, isLogError, describeLogError, LogDomainError } from '@/services/log/logApi';
import type { LogDesignMessage, LogProtocolMessage, LogSection } from '@/types/log';

const PAGE_LIMIT = 100;

type ChatMessageEvent = { message?: LogProtocolMessage & { channel?: { id?: string } }; channel?: { id?: string } };

export const useLogMessageWindow = (projectId: Ref<string>, section: Ref<LogSection | null>) => {
  const items = shallowRef<LogDesignMessage[]>([]);
  const loadedSectionId = ref('');
  const nextCursor = ref<string | null>(null);
  const sourceVersion = ref<number | null>(null);
  const loading = ref(false);
  const error = ref('');
  const errorCode = ref('');
  const staleHint = ref(false);
  let generation = 0;
  let controller: AbortController | null = null;
  let reloadTimer: ReturnType<typeof setTimeout> | null = null;
  /** 本页面发出的写入：messageId → 预期的新版本下界，用于识别自己的 WS 回声。 */
  const ownWrites = new Map<string, number>();

  const complete = computed(() => !loading.value && !error.value && nextCursor.value === null && sourceVersion.value !== null);
  const indexOf = (id: string) => items.value.findIndex((it) => it.message.id === id);
  const byId = (id: string) => items.value.find((it) => it.message.id === id) || null;

  const cancel = () => {
    generation += 1;
    controller?.abort();
    controller = null;
  };

  /** 每次只请求一页；只有来源变化时才丢弃窗口并从第一页重建。 */
  const loadPage = async (append = false, retries = 0): Promise<void> => {
    cancel();
    const sec = section.value;
    const pid = projectId.value;
    if (!sec || !pid) {
      loading.value = false;
      return;
    }
    const gen = generation;
    const ctrl = new AbortController();
    controller = ctrl;
    loading.value = true;
    error.value = '';
    errorCode.value = '';
    const cursor = append ? nextCursor.value : null;
    const version = append ? sourceVersion.value : null;
    try {
      const window = await fetchLogSectionMessages(pid, sec.id, cursor, PAGE_LIMIT, ctrl.signal);
      if (gen !== generation) return;
      if (append && window.sourceVersion !== version) {
        throw new LogDomainError('SOURCE_CHANGED', '来源消息已变化，需要重新加载');
      }
      items.value = append ? items.value.concat(window.items) : window.items;
      sourceVersion.value = window.sourceVersion;
      nextCursor.value = window.nextCursor;
      loadedSectionId.value = sec.id;
      staleHint.value = false;
    } catch (err) {
      if (gen !== generation || ctrl.signal.aborted) return;
      if (isLogError(err, 'SOURCE_CHANGED')) {
        items.value = [];
        nextCursor.value = null;
        sourceVersion.value = null;
        if (retries < 3) {
          await loadPage(false, retries + 1);
          return;
        }
      }
      error.value = describeLogError(err, '加载消息失败');
      errorCode.value = (err as any)?.code || '';
    } finally {
      if (gen === generation) loading.value = false;
    }
  };

  /** 从章节开头加载一页，整体替换窗口，不预取后续页。 */
  const load = () => loadPage();

  const loadMore = async () => {
    if (loading.value || nextCursor.value === null) return;
    await loadPage(true);
  };

  const scheduleReload = (delay = 800) => {
    if (reloadTimer) clearTimeout(reloadTimer);
    reloadTimer = setTimeout(() => {
      reloadTimer = null;
      void load();
    }, delay);
  };

  /** 自己的命令成功后，以服务端返回版本就地更新（能力保持不变，下一次重载以服务端为准）。 */
  const applyServerMessage = (message: LogProtocolMessage | null | undefined) => {
    if (!message?.id) return;
    const idx = indexOf(message.id);
    if (idx < 0) return;
    const prev = items.value[idx];
    const version = typeof message.version === 'number' ? message.version : prev.version + 1;
    if (version < prev.version) return;
    const next = items.value.slice();
    // 嵌入展开由服务端按原文投影：原文变化后旧展开作废，可能成为嵌入（站内链接含 #/）时重载取新投影。
    const contentChanged = typeof message.content === 'string' && message.content !== prev.message.content;
    const presentation = contentChanged && prev.presentation?.embed ? { ...prev.presentation, embed: undefined } : prev.presentation;
    next[idx] = { ...prev, message: { ...prev.message, ...message }, version, presentation };
    items.value = next;
    ownWrites.set(message.id, version);
    if (contentChanged && (prev.presentation?.embed || message.content!.includes('#/'))) scheduleReload();
  };

  const removeLocal = (id: string) => {
    const idx = indexOf(id);
    if (idx < 0) return;
    const next = items.value.slice();
    next.splice(idx, 1);
    items.value = next;
  };

  const markOwnWrite = (id: string, expectedAtLeast: number) => ownWrites.set(id, expectedAtLeast);

  // ---- 既有消息事件同步：只处理本章节来源频道；按引用注册与注销 ----

  const eventChannel = (e: ChatMessageEvent) => e?.channel?.id || e?.message?.channel?.id || '';
  const relevant = (e: ChatMessageEvent) => !!section.value && eventChannel(e) === section.value.source.channelId;

  const onUpdated = (e: ChatMessageEvent) => {
    if (!relevant(e) || !e.message?.id) return;
    const current = byId(e.message.id);
    if (!current) return;
    const incoming = typeof e.message.version === 'number' ? e.message.version : null;
    const own = ownWrites.get(e.message.id);
    if (incoming !== null && incoming <= current.version) return; // 回声或旧事件
    if (incoming !== null && own !== undefined && incoming <= own) return;
    if (incoming === null) {
      scheduleReload();
      return;
    }
    applyServerMessage(e.message);
    ownWrites.delete(e.message.id);
  };
  const onRemoved = (e: ChatMessageEvent) => {
    if (!relevant(e) || !e.message?.id) return;
    if (byId(e.message.id)) scheduleReload(300);
  };
  const onStructural = (e: ChatMessageEvent) => {
    if (!relevant(e)) return;
    const created = e.message?.createdAt;
    const toTime = section.value ? Date.parse(section.value.source.toTime) : NaN;
    if (typeof created === 'number' && Number.isFinite(toTime) && created > toTime) return;
    staleHint.value = true;
    scheduleReload(1200);
  };
  const onConnected = () => {
    // 重连不假设补发全部历史事件：用 sourceVersion 校验后重载。
    void probe();
  };

  chatEvent.on('message-updated' as any, onUpdated as any);
  chatEvent.on('message-deleted' as any, onRemoved as any);
  chatEvent.on('message-removed' as any, onRemoved as any);
  chatEvent.on('message-reordered' as any, onStructural as any);
  chatEvent.on('message-created' as any, onStructural as any);
  chatEvent.on('message-archived' as any, onStructural as any);
  chatEvent.on('message-unarchived' as any, onStructural as any);
  chatEvent.on('connected' as any, onConnected as any);

  /** 轻量探测：比较 sourceVersion，不一致则整体重载。 */
  const probe = async () => {
    const sec = section.value;
    const pid = projectId.value;
    const gen = generation;
    if (!sec || sourceVersion.value === null || loading.value) return;
    try {
      const w = await fetchLogSectionMessages(pid, sec.id, null, 1);
      if (gen !== generation || projectId.value !== pid || section.value?.id !== sec.id) return;
      if (w.sourceVersion !== sourceVersion.value) {
        staleHint.value = true;
        void load();
      }
    } catch {
      // 探测失败不打断编辑；下一次操作会给出明确错误。
    }
  };
  const onVisible = () => {
    if (document.visibilityState === 'visible') void probe();
  };
  document.addEventListener('visibilitychange', onVisible);

  watch(() => [projectId.value, section.value?.id, JSON.stringify(section.value?.source || null)], () => {
    items.value = [];
    loadedSectionId.value = '';
    nextCursor.value = null;
    sourceVersion.value = null;
    error.value = '';
    errorCode.value = '';
    if (reloadTimer) clearTimeout(reloadTimer);
    reloadTimer = null;
    ownWrites.clear();
    void load();
  }, { immediate: true });

  onBeforeUnmount(() => {
    cancel();
    if (reloadTimer) clearTimeout(reloadTimer);
    chatEvent.off('message-updated' as any, onUpdated as any);
    chatEvent.off('message-deleted' as any, onRemoved as any);
    chatEvent.off('message-removed' as any, onRemoved as any);
    chatEvent.off('message-reordered' as any, onStructural as any);
    chatEvent.off('message-created' as any, onStructural as any);
    chatEvent.off('message-archived' as any, onStructural as any);
    chatEvent.off('message-unarchived' as any, onStructural as any);
    chatEvent.off('connected' as any, onConnected as any);
    document.removeEventListener('visibilitychange', onVisible);
  });

  return {
    items,
    loadedSectionId,
    nextCursor,
    sourceVersion,
    loading,
    error,
    errorCode,
    complete,
    staleHint,
    byId,
    indexOf,
    load,
    loadMore,
    probe,
    applyServerMessage,
    removeLocal,
    markOwnWrite,
    scheduleReload,
  };
};

export type LogMessageWindowController = ReturnType<typeof useLogMessageWindow>;
