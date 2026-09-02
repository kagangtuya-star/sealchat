<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import type { SChannel } from '@/types';

interface SortNode {
  id: string;
  name: string;
  parentId: string;
  sortOrder: number;
  note?: string;
  membersCount?: number;
  permType?: string;
  children: SortNode[];
}

type SortRow =
  | { type: 'node'; key: string; node: SortNode; parentId: string; depth: number; index: number }
  | { type: 'tail'; key: string; parentId: string; depth: number };

const props = defineProps<{ show: boolean }>();
const emit = defineEmits<{ (e: 'update:show', value: boolean): void }>();

const chat = useChatStore();
const message = useMessage();

const visible = computed({
  get: () => props.show,
  set: (value: boolean) => emit('update:show', value),
});

const treeData = ref<SortNode[]>([]);
const originalOrders = ref<Record<string, string[]>>({});
const originalParents = ref<Record<string, string>>({});
const draggingId = ref<string | null>(null);
const dragOverKey = ref<string | null>(null);
const insideTargetId = ref<string | null>(null);
const saving = ref(false);

const initData = () => {
  const list = normalizeChannels(chat.channelTree as SChannel[] ?? [], '');
  treeData.value = list;
  originalOrders.value = {};
  originalParents.value = {};
  captureOriginalOrders(list, '');
};

const normalizeChannels = (channels: SChannel[] | undefined, parentId: string): SortNode[] => {
  if (!Array.isArray(channels)) return [];
  return channels
    .filter((item) => !item.isPrivate && item.permType !== 'private')
    .map((item) => ({
      id: item.id,
      name: item.name || '未命名频道',
      parentId,
      sortOrder: item.sortOrder ?? 0,
      note: item.note ?? '',
      membersCount: item.membersCount,
      permType: item.permType || 'public',
      children: normalizeChannels(item.children as SChannel[] | undefined, item.id),
    }));
};

const captureOriginalOrders = (nodes: SortNode[], parentId: string) => {
  originalOrders.value[parentId] = nodes.map((node) => node.id);
  nodes.forEach((node) => {
    originalParents.value[node.id] = node.parentId;
    if (node.children?.length) {
      captureOriginalOrders(node.children, node.id);
    } else {
      originalOrders.value[node.id] = [];
    }
  });
};

const rows = computed<SortRow[]>(() => {
  const flatten = (nodes: SortNode[], depth: number, parentId: string): SortRow[] => {
    const current: SortRow[] = [];
    nodes.forEach((node, index) => {
      current.push({ type: 'node', key: node.id, node, parentId, depth, index });
      if (node.children?.length) {
        current.push(...flatten(node.children, depth + 1, node.id));
      }
    });
    if (nodes.length > 0) {
      const tailKey = `tail-${parentId || 'root'}`;
      current.push({ type: 'tail', key: tailKey, parentId, depth });
    }
    return current;
  };
  return flatten(treeData.value, 0, '');
});

const isDirty = computed(() => checkDirty(treeData.value, ''));

const checkDirty = (nodes: SortNode[], parentId: string): boolean => {
  if (!nodes.length) return false;
  const currentIds = nodes.map((node) => node.id);
  const originalIds = originalOrders.value[parentId] || [];
  if (!arraysEqual(currentIds, originalIds)) {
    return true;
  }
  return nodes.some((node) => (
    node.parentId !== (originalParents.value[node.id] || '')
      || (node.children?.length ? checkDirty(node.children, node.id) : false)
  ));
};

const arraysEqual = (a: string[], b: string[]) => {
  if (a.length !== b.length) return false;
  return a.every((item, index) => item === b[index]);
};

const resetDragState = () => {
  draggingId.value = null;
  dragOverKey.value = null;
  insideTargetId.value = null;
};

watch(
  () => props.show,
  (val) => {
    if (val) {
      initData();
    } else {
      resetDragState();
    }
  },
  { immediate: true },
);

const handleDragStart = (row: SortRow) => {
  if (row.type !== 'node') return;
  draggingId.value = row.node.id;
  dragOverKey.value = null;
  insideTargetId.value = null;
};

const handleTailDragEnter = (row: SortRow) => {
  if (!draggingId.value) return;
  if (row.type !== 'tail') return;
  const sourceInfo = findNodeInfo(draggingId.value);
  if (!sourceInfo || !canMoveToParent(sourceInfo, row.parentId)) {
    dragOverKey.value = null;
    insideTargetId.value = null;
    return;
  }
  dragOverKey.value = row.key;
  insideTargetId.value = null;
};

const handleInsertDragEnter = (row: SortRow) => {
  if (!draggingId.value || row.type !== 'node') return;
  const sourceInfo = findNodeInfo(draggingId.value);
  if (!sourceInfo || !canMoveToParent(sourceInfo, row.parentId)) {
    dragOverKey.value = null;
    insideTargetId.value = null;
    return;
  }
  dragOverKey.value = `before-${row.node.id}`;
  insideTargetId.value = null;
};

const handleNodeDragEnter = (row: SortRow) => {
  if (!draggingId.value || row.type !== 'node') return;
  if (row.depth !== 0) {
    dragOverKey.value = null;
    insideTargetId.value = null;
    return;
  }
  const sourceInfo = findNodeInfo(draggingId.value);
  if (!sourceInfo || !canMoveToParent(sourceInfo, row.node.id)) {
    dragOverKey.value = null;
    insideTargetId.value = null;
    return;
  }
  dragOverKey.value = `inside-${row.node.id}`;
  insideTargetId.value = row.node.id;
};

const handleDrop = (row: SortRow) => {
  if (!draggingId.value) return;
  if (row.type === 'node') {
    if (dragOverKey.value === `before-${row.node.id}`) {
      moveNodeBefore(row.parentId, row.index, false);
    } else if (row.depth === 0 && dragOverKey.value === `inside-${row.node.id}`) {
      moveNodeBefore(row.node.id, Number.POSITIVE_INFINITY, true);
    }
  } else if (row.type === 'tail') {
    moveNodeBefore(row.parentId, Number.POSITIVE_INFINITY, true);
  }
};

const moveNodeBefore = (parentId: string, targetIndex: number, isTail = false) => {
  const dragged = draggingId.value;
  if (!dragged) return;
  const sourceInfo = findNodeInfo(dragged);
  if (!sourceInfo) return;
  if (!canMoveToParent(sourceInfo, parentId)) {
    message.warning('该频道不能移动到目标层级');
    return;
  }
  const sourceList = getListByParent(sourceInfo.parentId);
  const targetList = getListByParent(parentId);
  if (!sourceList || !targetList) return;
  const from = sourceInfo.index;
  let insertIndex = isTail ? targetList.length : targetIndex;
  const sameList = sourceList === targetList;
  const [item] = sourceList.splice(from, 1);
  if (sameList && isTail) {
    insertIndex = sourceList.length;
  } else if (sameList && from < insertIndex) {
    insertIndex -= 1;
  }
  insertIndex = Math.max(0, Math.min(insertIndex, targetList.length));
  item.parentId = parentId;
  targetList.splice(insertIndex, 0, item);
  dragOverKey.value = null;
  insideTargetId.value = null;
};

const canMoveToParent = (
  sourceInfo: { node: SortNode; parentId: string; index: number },
  parentId: string,
) => {
  if (!parentId) return true;
  if (sourceInfo.node.id === parentId) return false;
  const parentInfo = findNodeInfo(parentId);
  if (!parentInfo || parentInfo.parentId) return false;
  if (sourceInfo.parentId === parentId) return true;
  return !sourceInfo.node.children?.length;
};

const findNodeInfo = (
  nodeId: string,
  nodes: SortNode[] = treeData.value,
  parentId = '',
): { node: SortNode; parentId: string; index: number } | null => {
  for (let index = 0; index < nodes.length; index += 1) {
    const node = nodes[index];
    if (node.id === nodeId) {
      return { node, parentId, index };
    }
    if (node.children?.length) {
      const child = findNodeInfo(nodeId, node.children, node.id);
      if (child) return child;
    }
  }
  return null;
};

const getListByParent = (parentId: string): SortNode[] | null => {
  if (!parentId) return treeData.value;
  const parentInfo = findNodeInfo(parentId);
  return parentInfo?.node.children || null;
};

const closeModal = () => {
  visible.value = false;
};

interface SortUpdate {
  id: string;
  name: string;
  note?: string;
  permType?: string;
  sortOrder: number;
}

interface ParentMove {
  id: string;
  parentId: string;
}

const generateSequentialOrders = (count: number) => {
  const base = count * 100;
  return Array.from({ length: count }, (_, index) => base - index * 100);
};

const collectUpdates = (nodes: SortNode[], parentId: string, bucket: SortUpdate[]) => {
  if (!nodes.length) return;
  const original = originalOrders.value[parentId] || [];
  const current = nodes.map((node) => node.id);
  const changed = !arraysEqual(current, original);
  if (changed) {
    const orders = generateSequentialOrders(nodes.length);
    nodes.forEach((node, index) => {
      const nextOrder = orders[index];
      if (node.sortOrder !== nextOrder) {
        bucket.push({
          id: node.id,
          sortOrder: nextOrder,
          name: node.name,
          note: node.note ?? '',
          permType: node.permType,
        });
      }
    });
  }
  nodes.forEach((node) => {
    if (node.children?.length) {
      collectUpdates(node.children, node.id, bucket);
    }
  });
};

const collectParentMoves = (nodes: SortNode[], bucket: ParentMove[]) => {
  nodes.forEach((node) => {
    if (node.parentId !== (originalParents.value[node.id] || '')) {
      bucket.push({ id: node.id, parentId: node.parentId });
    }
    if (node.children?.length) {
      collectParentMoves(node.children, bucket);
    }
  });
};

const saveReorder = async () => {
  if (saving.value) return false;
  if (!isDirty.value) {
    message.info('顺序未发生变化');
    closeModal();
    return false;
  }
  const updates: SortUpdate[] = [];
  const parentMoves: ParentMove[] = [];
  collectUpdates(treeData.value, '', updates);
  collectParentMoves(treeData.value, parentMoves);
  if (!updates.length && !parentMoves.length) {
    message.info('顺序未发生变化');
    closeModal();
    return false;
  }
  saving.value = true;
  try {
    for (const move of parentMoves.sort((a, b) => {
      const depthDiff = Number(Boolean(originalParents.value[b.id]))
        - Number(Boolean(originalParents.value[a.id]));
      if (depthDiff) return depthDiff;
      return Number(!b.parentId) - Number(!a.parentId);
    })) {
      await chat.channelMove(move.id, move.parentId);
    }
    for (const update of updates) {
      await chat.channelInfoEdit(update.id, {
        sortOrder: update.sortOrder,
        name: update.name,
        note: update.note ?? '',
        permType: update.permType,
      });
    }
    if (chat.currentWorldId) {
      await chat.channelList(chat.currentWorldId, true);
    }
    message.success('频道排序已更新');
    closeModal();
  } catch (error: any) {
    message.error(error?.message || '保存排序失败');
  } finally {
    saving.value = false;
  }
  return false;
};

const refreshFromServer = async () => {
  if (!chat.currentWorldId) return;
  await chat.channelList(chat.currentWorldId, true);
  initData();
};
</script>

<template>
  <n-modal v-model:show="visible" preset="dialog" title="频道排序" :positive-text="'保存'" :negative-text="'取消'"
    :positive-button-props="{ disabled: !isDirty || !treeData.length, loading: saving }" @positive-click="saveReorder"
    @negative-click="closeModal">
    <div class="space-y-3">
      <div class="channel-sort-toolbar">
        <div class="channel-sort-help">
          <div class="channel-sort-help-text">拖动调整顺序，拖到上级频道上可移动为其子频道。</div>
        </div>
        <div class="channel-sort-actions">
          <n-button size="tiny" tertiary @click="refreshFromServer" :loading="saving">
            重新获取
          </n-button>
          <n-button size="tiny" tertiary @click="initData" :disabled="saving">
            恢复初始顺序
          </n-button>
        </div>
      </div>
      <div v-if="!treeData.length" class="py-6">
        <n-empty description="暂无可排序的频道" />
      </div>
      <div v-else class="channel-sort-list" @dragover.prevent>
        <div v-for="row in rows" :key="row.key">
          <template v-if="row.type === 'node'">
            <div v-if="draggingId" class="channel-sort-insert-line"
              :class="{ active: dragOverKey === `before-${row.node.id}` }"
              @dragenter.prevent="handleInsertDragEnter(row)" @dragover.prevent @drop.prevent="handleDrop(row)"
              :style="{ marginLeft: `${row.depth * 20 + 12}px` }" aria-hidden="true"></div>
            <div class="channel-sort-item" :class="{
              dragging: draggingId === row.node.id,
              'drop-inside': insideTargetId === row.node.id,
              subchannel: row.depth > 0,
            }" draggable="true"
              @dragstart="handleDragStart(row)" @dragend="resetDragState"
              @dragenter.prevent="handleNodeDragEnter(row)" @dragover.prevent @drop.prevent="handleDrop(row)"
              :style="{ paddingLeft: `${row.depth * 20 + 12}px` }">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="channel-sort-grip" aria-hidden="true">⠿</span>
                  <div class="font-medium">{{ row.node.name }}</div>
                  <n-tag size="small" v-if="row.node.permType === 'non-public'" type="warning" round>
                    非公开
                  </n-tag>
                </div>
                <div class="flex items-center gap-2">
                  <div class="channel-sort-members">
                    {{ row.node.membersCount ? `${row.node.membersCount}人` : '' }}
                  </div>
                  <span v-if="insideTargetId === row.node.id" class="channel-sort-inside-hint">↳ 移入</span>
                </div>
              </div>
            </div>
          </template>
          <template v-else>
            <div v-if="draggingId" class="channel-sort-insert-line"
              :class="{ active: dragOverKey === row.key }"
              @dragenter.prevent="handleTailDragEnter(row)" @dragover.prevent @drop.prevent="handleDrop(row)"
              :style="{ marginLeft: `${row.depth * 20 + 12}px` }" aria-hidden="true"></div>
          </template>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
.channel-sort-list {
  max-height: 60vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.15rem 0;
}

.channel-sort-toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.channel-sort-help-title {
  color: var(--n-text-color);
  font-size: 0.9rem;
  font-weight: 600;
}

.channel-sort-help-text {
  margin-top: 0.2rem;
  color: var(--n-text-color-3);
  font-size: 0.8rem;
}

.channel-sort-actions {
  display: flex;
  flex-shrink: 0;
  gap: 0.25rem;
}

.channel-sort-item {
  position: relative;
  min-height: 2.35rem;
  border: 1px solid transparent;
  border-radius: 0.5rem;
  background: transparent;
  cursor: grab;
  transition: background-color 0.16s ease, border-color 0.16s ease, box-shadow 0.16s ease, opacity 0.16s ease;
}

.channel-sort-item:hover {
  background: var(--n-color-hover);
}

.channel-sort-item.subchannel {
  border-left-color: transparent;
  border-top-left-radius: 0.2rem;
  border-bottom-left-radius: 0.2rem;
}

.channel-sort-item.subchannel::before {
  position: absolute;
  top: 0.25rem;
  bottom: 0.25rem;
  left: 1.25rem;
  width: 2px;
  border-radius: 999px;
  background: var(--n-border-color);
  content: '';
}

.channel-sort-item.dragging {
  opacity: 0.5;
  cursor: grabbing;
}

.channel-sort-item.drop-inside {
  border-color: color-mix(in srgb, var(--n-primary-color) 55%, transparent);
  background: color-mix(in srgb, var(--n-primary-color) 12%, transparent);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--n-primary-color) 24%, transparent);
}

.channel-sort-grip {
  display: inline-flex;
  width: 1rem;
  margin-right: 0.35rem;
  color: var(--n-text-color-3);
  font-size: 1rem;
  line-height: 1;
  opacity: 0.7;
  user-select: none;
}

.channel-sort-inside-hint {
  margin-left: 0.5rem;
  color: var(--n-primary-color);
  font-size: 0.75rem;
  white-space: nowrap;
}

.channel-sort-members {
  color: var(--n-text-color-3);
  font-size: 0.75rem;
}

.channel-sort-insert-line {
  height: 0.3rem;
  margin-top: -0.05rem;
  margin-right: 0.4rem;
  border-top: 2px solid transparent;
  border-radius: 999px;
  transition: border-color 0.16s ease, background-color 0.16s ease;
}

.channel-sort-insert-line:hover,
.channel-sort-insert-line.active {
  border-top-color: var(--n-primary-color);
  background: color-mix(in srgb, var(--n-primary-color) 15%, transparent);
}
</style>
