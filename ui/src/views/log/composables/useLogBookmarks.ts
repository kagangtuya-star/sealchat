// 私有及共享书签使用同一个授权列表；不进入草稿，也不推进工程版本。
import { onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue';
import { chatEvent } from '@/stores/chat';
import { createLogBookmark, deleteLogBookmark, describeLogError, listLogBookmarks, renameLogBookmark } from '@/services/log/logApi';
import type { LogBookmark, LogBookmarkCreate } from '@/types/log';

export const useLogBookmarks = (projectId: Ref<string>, enabled: () => boolean) => {
  const items = shallowRef<LogBookmark[]>([]);
  const loading = ref(false);
  const busy = ref(false);
  const error = ref('');
  let seq = 0;
  let disposed = false;

  const reload = async () => {
    const id = ++seq;
    const pid = projectId.value;
    if (disposed || !enabled() || !pid) {
      items.value = [];
      loading.value = false;
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      const list = await listLogBookmarks(pid);
      if (id === seq) items.value = list;
    } catch (e) {
      if (id === seq) {
        items.value = [];
        error.value = describeLogError(e, '书签加载失败');
      }
    } finally {
      if (id === seq) loading.value = false;
    }
  };

  /** 写操作串行执行，结果只应用到发起时的工程。 */
  const run = async (label: string, op: (pid: string) => Promise<unknown>) => {
    const pid = projectId.value;
    if (disposed || !enabled() || !pid || busy.value) return false;
    busy.value = true;
    try {
      await op(pid);
      // 事件可能先于 HTTP 回包到达；统一重取，避免重复添加或旧列表覆盖写入。
      if (!disposed && pid === projectId.value) await reload();
      return !disposed && pid === projectId.value;
    } catch (e) {
      if (!disposed && pid === projectId.value) error.value = describeLogError(e, label);
      return false;
    } finally {
      busy.value = false;
    }
  };

  const add = (input: LogBookmarkCreate) =>
    run('添加书签失败', (pid) => createLogBookmark(pid, input));
  const rename = (bookmark: LogBookmark, label: string) =>
    run('重命名失败', (pid) => renameLogBookmark(pid, bookmark.id, label));
  const remove = (bookmark: LogBookmark) =>
    run('删除失败', (pid) => deleteLogBookmark(pid, bookmark.id));

  const onChanged = (event: { projectId?: string }) => {
    if (event?.projectId === projectId.value) void reload();
  };
  const onConnected = () => void reload();
  chatEvent.on('log-project-bookmarks-changed', onChanged);
  chatEvent.on('connected', onConnected);
  watch([projectId, () => enabled()], () => {
    items.value = [];
    void reload();
  }, { immediate: true });
  onBeforeUnmount(() => {
    disposed = true;
    seq++;
    chatEvent.off('log-project-bookmarks-changed', onChanged);
    chatEvent.off('connected', onConnected);
  });

  return { items, loading, busy, error, reload, add, rename, remove };
};
