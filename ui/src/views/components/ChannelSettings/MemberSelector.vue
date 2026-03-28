<script lang="tsx" setup>
import type { PropType } from 'vue';
import type { UserInfo } from '@/types';
import { Check, Search } from '@vicons/tabler';
import { defineProps, ref, watch } from 'vue';

const props = defineProps({
  memberList: {
    type: Array as PropType<UserInfo[]>,
    default: () => [],
  },
  startSelectedList: {
    type: Array as PropType<UserInfo[]>,
    default: () => [],
  },
  remote: {
    type: Boolean,
    default: false,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  keyword: {
    type: String,
    default: '',
  },
  page: {
    type: Number,
    default: 1,
  },
  pageSize: {
    type: Number,
    default: 20,
  },
  total: {
    type: Number,
    default: 0,
  },
});

const selectedList = ref<string[]>([]);
const searchKeyword = ref(props.keyword || '');

const syncSelectedFromProps = () => {
  const nextSelected: string[] = [];
  for (const item of props.startSelectedList || []) {
    if (item.id) {
      nextSelected.push(item.id);
    }
  }
  selectedList.value = Array.from(new Set(nextSelected));
};

syncSelectedFromProps();

watch(() => props.startSelectedList, syncSelectedFromProps, { deep: true });
watch(() => props.keyword, (value) => {
  searchKeyword.value = value || '';
});

const toggleSelection = (userId?: string) => {
  if (!userId) return;
  const index = selectedList.value.indexOf(userId);
  if (index === -1) {
    selectedList.value.push(userId);
  } else {
    selectedList.value.splice(index, 1);
  }
};

const isSelected = (userId?: string) => {
  if (!userId) return false;
  return selectedList.value.includes(userId);
};

const emit = defineEmits(['confirm', 'search', 'page-change']);

const handleSearch = () => {
  emit('search', searchKeyword.value.trim());
};

const handlePageChange = (page: number) => {
  emit('page-change', page);
};

const handleConfirm = () => {
  emit('confirm', selectedList.value, props.startSelectedList?.map(i => i.id) ?? []);
};
</script>

<template>
  <div v-if="props.remote" class="mb-3 flex gap-2">
    <n-input
      v-model:value="searchKeyword"
      clearable
      placeholder="搜索用户ID、昵称或用户名"
      @keydown.enter.prevent="handleSearch"
    />
    <n-button secondary @click="handleSearch">
      <template #icon>
        <n-icon>
          <Search />
        </n-icon>
      </template>
      搜索
    </n-button>
  </div>

  <n-spin :show="props.loading">
    <div class="member-selector-list flex flex-wrap justify-center relative min-h-12">
      <template v-if="props.memberList && props.memberList.length > 0">
        <div
          v-for="j in props.memberList"
          :key="j.id || j.username"
          class="relative group pr-1 select-none"
          @click="toggleSelection(j.id)"
        >
          <UserLabelV
            :name="j.nick ?? j.username"
            :src="j.avatar"
            :decoration="j.avatarDecoration"
          />

          <div class="absolute inset-0 bg-black bg-opacity-50 flex items-center justify-center " v-if="isSelected(j.id)">
            <n-icon size="24" color="#ffffff">
              <Check />
            </n-icon>
          </div>
        </div>
      </template>
      <div v-else class="w-full py-6 text-center text-sm text-gray-500">
        暂无可选成员
      </div>
    </div>
  </n-spin>

  <div v-if="props.remote && props.total > props.pageSize" class="mt-3 flex justify-center">
    <n-pagination
      :page="props.page"
      :page-size="props.pageSize"
      :item-count="props.total"
      simple
      @update:page="handlePageChange"
    />
  </div>

  <div class="flex justify-end mt-2">
    <n-button class="mt-4 w-full" type="primary" @click="handleConfirm">
      确定
    </n-button>
  </div>
</template>

<style scoped>
.member-selector-list {
  max-height: min(52vh, 420px);
  overflow-y: auto;
  align-content: flex-start;
  padding-right: 4px;
}
</style>
