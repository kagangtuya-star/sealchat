// 样式库列表：内置样式 + 自己的私有样式（服务端 ListLogStyles 只返回这两类）。
// 由工作台持有一份，样式面板与 Style Studio 共用，保存 / 新建 / 导入后统一刷新。
import { computed, ref, shallowRef } from 'vue';
import { describeLogError, fetchLogStyles } from '@/services/log/logApi';
import type { LogStyleSummary } from '@/types/log';
import { builtinStyleOrder } from '../logStyleCatalog';

const MAX_PAGES = 20;

export type LogStyleOwnership = 'owned' | 'foreign' | 'unknown';

/** 列表项存在时归属明确；只有完整成功读完列表后，“不存在”才代表不是自己的样式。 */
export const resolveLogStyleOwnership = (
  items: readonly LogStyleSummary[],
  state: { loaded: boolean; loading: boolean; error: string; truncated: boolean },
  id: string,
): LogStyleOwnership => {
  const found = items.find((s) => s.id === id);
  if (found) return found.builtin ? 'foreign' : 'owned';
  if (id.startsWith('sealchat-')) return 'foreign';
  if (!state.loaded || !!state.error || state.truncated) return 'unknown';
  return 'foreign';
};

export const useLogStyleLibrary = () => {
  const items = shallowRef<LogStyleSummary[]>([]);
  const loading = ref(false);
  const loaded = ref(false);
  const error = ref('');
  /** 列表达到分页上限仍未读完时为 true（极少见），此时“不在列表中”不能当作“不属于我”。 */
  const truncated = ref(false);
  let seq = 0;

  /** 读完所有分页再替换列表；并发刷新时只采用最后一次的结果。 */
  const reload = async () => {
    const id = ++seq;
    loading.value = true;
    error.value = '';
    try {
      const all: LogStyleSummary[] = [];
      let cursor: string | null = null;
      let pages = 0;
      do {
        const r = await fetchLogStyles(cursor, 50);
        all.push(...r.items);
        cursor = r.nextCursor || null;
        pages += 1;
      } while (cursor && pages < MAX_PAGES);
      if (id !== seq) return;
      items.value = all;
      truncated.value = !!cursor;
      loaded.value = true;
    } catch (e) {
      if (id === seq) error.value = describeLogError(e, '读取样式列表失败');
    } finally {
      if (id === seq) loading.value = false;
    }
  };

  const byId = (id: string) => items.value.find((s) => s.id === id) || null;
  const builtin = computed(() => items.value.filter((s) => s.builtin).slice().sort((a, b) => builtinStyleOrder(a.id) - builtinStyleOrder(b.id)));
  const mine = computed(() => items.value.filter((s) => !s.builtin));
  const ownership = (id: string) => resolveLogStyleOwnership(items.value, {
    loaded: loaded.value,
    loading: loading.value,
    error: error.value,
    truncated: truncated.value,
  }, id);
  /** 明确确认属于当前用户时才返回 true；unknown 由调用方单独处理。 */
  const owns = (id: string) => ownership(id) === 'owned';

  return { items, loading, loaded, error, truncated, reload, byId, builtin, mine, ownership, owns };
};

export type LogStyleLibrary = ReturnType<typeof useLogStyleLibrary>;
