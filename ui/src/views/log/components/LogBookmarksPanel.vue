<script setup lang="ts">
// 私有与共享书签使用统一列表，可见范围只通过图标提示；也列出正在协作的设计者。
import { nextTick, ref } from 'vue';
import { Lock, Pencil, Plus, Trash, Users } from '@vicons/tabler';
import type { LogBookmark, LogPresence } from '@/types/log';
import { presenceColor } from '../composables/useLogCollab';

const props = defineProps<{
  supported: boolean;
  items: LogBookmark[];
  loading: boolean;
  error: string;
  busy: boolean;
  /** 新书签将标记的位置说明；为空表示当前没有可标记的位置。 */
  addTarget: string;
  /** 书签所在位置的章节说明；返回 null 表示来源片段已不存在。 */
  locationOf: (b: LogBookmark) => string | null;
  collaborators: LogPresence[];
  nameOf: (p: LogPresence) => string;
  describePresence: (p: LogPresence) => string;
}>();
const emit = defineEmits<{
  (e: 'add'): void;
  (e: 'jump', bookmark: LogBookmark): void;
  (e: 'rename', bookmark: LogBookmark, label: string): void;
  (e: 'remove', bookmark: LogBookmark): void;
  (e: 'retry'): void;
  (e: 'follow', presence: LogPresence): void;
}>();

const editingId = ref('');
const draftLabel = ref('');
const inputRef = ref<{ focus: () => void; select: () => void } | null>(null);
const startRename = async (b: LogBookmark) => {
  editingId.value = b.id;
  draftLabel.value = b.label;
  await nextTick();
  inputRef.value?.focus();
  inputRef.value?.select();
};
const commitRename = (b: LogBookmark) => {
  if (editingId.value !== b.id) return;
  editingId.value = '';
  const label = draftLabel.value.trim().slice(0, 60);
  if (label && label !== b.label) emit('rename', b, label);
};
const cancelRename = () => {
  editingId.value = '';
};
</script>

<template>
  <div class="log-bookmarks">
    <section v-if="collaborators.length">
      <div class="log-bookmarks__title">正在协作（{{ collaborators.length }}）</div>
      <ul class="log-bookmarks__list">
        <li v-for="p in collaborators" :key="p.sessionId" class="log-bookmarks__peer">
          <i class="log-bookmarks__dot" :style="{ background: presenceColor(p.userId) }" aria-hidden="true" />
          <div class="log-bookmarks__main">
            <strong>{{ nameOf(p) }}</strong>
            <span>{{ describePresence(p) }}</span>
          </div>
          <n-button size="tiny" quaternary :disabled="!p.sectionId" @click="emit('follow', p)">跳转</n-button>
        </li>
      </ul>
    </section>

    <section>
      <div class="log-bookmarks__head">
        <div class="log-bookmarks__title">书签（{{ items.length }}）</div>
        <n-button size="tiny" quaternary :disabled="!supported || !addTarget || busy" @click="emit('add')">
          <template #icon><n-icon :component="Plus" /></template>
          添加
        </n-button>
      </div>
      <p class="log-bookmarks__hint">
        <template v-if="!supported">当前服务器不支持书签，需同步部署后端。</template>
        <template v-else>{{ addTarget ? `将标记：${addTarget}` : '选中消息或滚动到目标位置后添加。' }}</template>
      </p>
      <n-alert v-if="error" type="warning" :show-icon="false" class="log-bookmarks__error">
        {{ error }}
        <n-button size="tiny" style="margin-left: 8px" @click="emit('retry')">重试</n-button>
      </n-alert>
      <div v-if="loading && !items.length" class="log-bookmarks__empty">加载中…</div>
      <div v-else-if="supported && !items.length && !error" class="log-bookmarks__empty">还没有书签。选中消息后点击“添加”，或在消息右键菜单中添加。</div>
      <ul v-else class="log-bookmarks__list">
        <li v-for="b in items" :key="b.id" class="log-bookmarks__item" :class="{ 'is-missing': props.locationOf(b) === null }">
          <template v-if="editingId === b.id">
            <n-input
              :ref="(el: any) => (inputRef = el)"
              v-model:value="draftLabel"
              size="small"
              maxlength="60"
              aria-label="书签名称"
              @keydown.enter.prevent="commitRename(b)"
              @keydown.esc.prevent.stop="cancelRename"
              @blur="commitRename(b)"
            />
          </template>
          <template v-else>
            <n-tooltip trigger="hover">
              <template #trigger>
                <span class="log-bookmarks__scope" tabindex="0" role="img" :aria-label="b.visibility === 'shared' ? '共享：有此 LOG 工作台访问权限的用户可见' : '仅自己可见'">
                  <n-icon :component="b.visibility === 'shared' ? Users : Lock" size="14" />
                </span>
              </template>
              {{ b.visibility === 'shared' ? '共享：有此 LOG 工作台访问权限的用户可见' : '仅自己可见' }}
            </n-tooltip>
            <button type="button" class="log-bookmarks__jump" :disabled="props.locationOf(b) === null" :title="props.locationOf(b) === null ? '书签所在的来源片段已不存在' : '跳转到书签位置'" @click="emit('jump', b)">
              <strong>{{ b.label }}</strong>
              <span>{{ props.locationOf(b) ?? '来源片段已不存在' }}</span>
            </button>
            <button v-if="b.canManage !== false" type="button" class="log-bookmarks__icon" title="重命名" aria-label="重命名书签" :disabled="busy" @click="startRename(b)">
              <n-icon :component="Pencil" size="14" />
            </button>
            <n-popconfirm v-if="b.canManage !== false" @positive-click="emit('remove', b)">
              <template #trigger>
                <button type="button" class="log-bookmarks__icon" title="删除" aria-label="删除书签" :disabled="busy">
                  <n-icon :component="Trash" size="14" />
                </button>
              </template>
              删除书签「{{ b.label }}」？
            </n-popconfirm>
          </template>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.log-bookmarks {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.log-bookmarks__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.log-bookmarks__title {
  font-weight: 600;
  font-size: 13px;
}

.log-bookmarks__hint,
.log-bookmarks__empty {
  margin: 6px 0 8px;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.log-bookmarks__error {
  margin-bottom: 8px;
}

.log-bookmarks__list {
  list-style: none;
  margin: 6px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.log-bookmarks__item,
.log-bookmarks__peer {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  padding: 2px 4px;
  border-radius: 6px;
}

.log-bookmarks__item:hover,
.log-bookmarks__peer:hover {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
}

.log-bookmarks__jump,
.log-bookmarks__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 4px 2px;
  text-align: left;
  font: inherit;
  color: inherit;
}

.log-bookmarks__jump {
  border: 0;
  background: transparent;
  cursor: pointer;
}

.log-bookmarks__jump:focus-visible,
.log-bookmarks__icon:focus-visible {
  outline: 2px solid var(--primary-color, #2f6fde);
  outline-offset: 1px;
  border-radius: 4px;
}

.log-bookmarks__jump strong,
.log-bookmarks__main strong,
.log-bookmarks__jump span,
.log-bookmarks__main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-bookmarks__jump strong,
.log-bookmarks__main strong {
  font-size: 13px;
  font-weight: 500;
}

.log-bookmarks__jump span,
.log-bookmarks__main span {
  font-size: 11px;
  color: var(--sc-text-secondary, #666);
}

.log-bookmarks__item.is-missing .log-bookmarks__jump {
  cursor: not-allowed;
  opacity: 0.6;
}

.log-bookmarks__icon {
  flex: none;
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--sc-text-secondary, #666);
  cursor: pointer;
}

.log-bookmarks__icon:hover:not(:disabled) {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.08));
  color: inherit;
}

.log-bookmarks__icon:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.log-bookmarks__scope {
  flex: none;
  display: grid;
  place-items: center;
  width: 22px;
  color: var(--sc-text-secondary, #666);
}

.log-bookmarks__dot {
  flex: none;
  width: 8px;
  height: 8px;
  margin: 0 4px;
  border-radius: 50%;
}
</style>
