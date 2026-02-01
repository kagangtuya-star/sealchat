<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useDialog, useMessage } from 'naive-ui';
import { Star, StarOff, Search } from '@vicons/tabler';
import { useRouter } from 'vue-router';
import { matchText } from '@/utils/pinyinMatch';

const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();
const router = useRouter();
const loading = ref(false);
const inviteSlug = ref('');
const joining = ref(false);
const searchKeyword = ref('');
const createVisible = ref(false);
const creating = ref(false);
const createForm = ref({
  name: '',
  description: '',
  visibility: 'public',
});

const MAX_DESCRIPTION_LENGTH = 30;
const DESCRIPTION_LINE_LENGTH = 11;

const formatWorldDescription = (description?: string) => {
  const value = (description || '暂无简介').trim() || '暂无简介';
  const limited = Array.from(value).slice(0, MAX_DESCRIPTION_LENGTH);
  const segments: string[] = [];
  for (let i = 0; i < limited.length; i += DESCRIPTION_LINE_LENGTH) {
    segments.push(limited.slice(i, i + DESCRIPTION_LINE_LENGTH).join(''));
  }
  return segments.join('\n');
};

const fetchList = async (keyword?: string) => {
  loading.value = true;
  try {
    const params: { page: number; pageSize: number; joined: boolean; keyword?: string } = {
      page: 1,
      pageSize: 50,
      joined: true,
    };
    const effectiveKeyword = keyword ?? searchKeyword.value.trim();
    if (effectiveKeyword) {
      params.keyword = effectiveKeyword;
    }
    await chat.worldList(params);
  } catch (e) {
    message.error('加载世界列表失败');
  } finally {
    loading.value = false;
  }
};

const fetchExploreList = async (keyword?: string) => {
  loading.value = true;
  try {
    const params: { page?: number; pageSize?: number; keyword?: string; visibility?: string; joined?: boolean } = {
      page: 1,
      pageSize: 50,
      visibility: 'public',
      joined: false,
    };
    const effectiveKeyword = keyword ?? searchKeyword.value.trim();
    if (effectiveKeyword) {
      params.keyword = effectiveKeyword;
    }
    await chat.worldListExplore(params);
  } catch (e) {
    message.error('加载公开世界失败');
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  const keyword = searchKeyword.value.trim();
  if (chat.worldLobbyMode === 'mine') {
    fetchList(keyword);
  } else {
    fetchExploreList(keyword);
  }
};

watch(searchKeyword, (val) => {
  if (val === '') {
    if (chat.worldLobbyMode === 'mine') {
      fetchList();
    } else {
      fetchExploreList();
    }
  }
});

onMounted(async () => {
  await chat.fetchFavoriteWorlds().catch(() => {});
  await fetchList();
});

const enterWorld = async (worldId: string) => {
  try {
    await chat.switchWorld(worldId, { force: true });
    await router.push({ name: 'home' });
  } catch (err: any) {
    message.error(err?.response?.data?.message || '进入世界失败');
  }
};

const consumeInvite = async () => {
  const slug = inviteSlug.value.trim();
  if (!slug) return;
  joining.value = true;
  try {
    const resp = await chat.consumeWorldInvite(slug);
    const worldId = resp.world?.id;
    const worldName = resp.world?.name || '目标世界';
    if (resp.already_joined && worldId) {
      message.info(`您已经加入了「${worldName}」`);
      await chat.switchWorld(worldId, { force: true });
      await router.push({ name: 'home' });
      return;
    }
    if (worldId) {
      await chat.switchWorld(worldId, { force: true });
      message.success('已加入世界');
      await router.push({ name: 'home' });
    }
  } catch (e: any) {
    const msg = e?.response?.data?.message || '加入失败';
    message.error(msg);
  } finally {
    joining.value = false;
  }
};

const lobbyMode = computed(() => chat.worldLobbyMode);

const filteredMineWorlds = computed(() => {
  const keyword = searchKeyword.value.trim();
  const items = chat.worldListCache?.items || [];
  if (!keyword) return items;
  return items.filter((item: any) => {
    const name = item.world?.name || '';
    const desc = item.world?.description || '';
    return matchText(keyword, name) || matchText(keyword, desc);
  });
});

const exploreWorlds = computed(() => {
  const cache = chat.exploreWorldCache;
  const items = cache?.items || [];
  const keyword = searchKeyword.value.trim();
  if (!keyword) return items;
  return items.filter((item: any) => {
    const name = item.world?.name || '';
    const desc = item.world?.description || '';
    return matchText(keyword, name) || matchText(keyword, desc);
  });
});

const toggleFavorite = async (worldId: string) => {
  try {
    await chat.toggleWorldFavorite(worldId);
    if (chat.worldLobbyMode === 'mine') {
      await fetchList(searchKeyword.value.trim());
    } else {
      await fetchExploreList(searchKeyword.value.trim());
    }
  } catch (err: any) {
    message.error(err?.response?.data?.message || '更新收藏失败');
  }
};

const getWorldRoleTag = (role: string) => {
  switch (role) {
    case 'owner':
      return { label: '拥有者', type: 'warning' as const };
    case 'admin':
      return { label: '管理员', type: 'info' as const };
    case 'spectator':
      return { label: '旁观者', type: 'default' as const };
    case 'member':
      return { label: '成员', type: 'success' as const };
    default:
      return { label: '已加入', type: 'success' as const };
  }
};

const confirmLeaveWorld = (item: any) => {
  if (!item?.world?.id) return;
  if (item.memberRole === 'owner') {
    message.warning('世界创建者无法退出该世界');
    return;
  }
  dialog.warning({
    title: '确认退出世界',
    content: `确定要退出「${item.world.name}」吗？退出后需要重新邀请才能再次进入。`,
    positiveText: '确认退出',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: async () => {
      try {
        await chat.leaveWorld(item.world.id);
        message.success('已退出世界');
        await fetchList(searchKeyword.value.trim());
      } catch (error: any) {
        message.error(error?.response?.data?.message || '退出失败');
      }
    },
  });
};

const resetCreateForm = () => {
  createForm.value = {
    name: '',
    description: '',
    visibility: 'public',
  };
};

const handleCreateWorld = async () => {
  if (!createForm.value.name.trim()) {
    message.error('请输入世界名称');
    return;
  }
  creating.value = true;
  try {
    await chat.createWorld({
      name: createForm.value.name,
      description: createForm.value.description,
      visibility: createForm.value.visibility,
    });
    message.success('创建世界成功');
    createVisible.value = false;
    resetCreateForm();
    await fetchList();
  } catch (err: any) {
    message.error(err?.response?.data?.message || err?.message || '创建世界失败');
  } finally {
    creating.value = false;
  }
};

const switchLobbyMode = async () => {
  if (chat.worldLobbyMode === 'mine') {
    chat.worldLobbyMode = 'explore';
    await fetchExploreList();
  } else {
    chat.worldLobbyMode = 'mine';
    await fetchList();
  }
};
</script>

<template>
  <div class="p-4 space-y-3">
    <div class="flex justify-between items-center">
      <h2 class="text-lg font-bold">世界大厅</h2>
      <n-space size="small">
        <n-button
          size="small"
          @click="() => (chat.worldLobbyMode === 'mine' ? fetchList() : fetchExploreList())"
          :loading="loading"
        >
          刷新
        </n-button>
        <n-button size="small" type="primary" @click="createVisible = true" v-if="lobbyMode === 'mine'">创建世界</n-button>
        <n-button
          size="small"
          :type="lobbyMode === 'mine' ? 'tertiary' : 'primary'"
          @click="switchLobbyMode"
        >
          {{ lobbyMode === 'mine' ? '探索世界' : '我的世界' }}
        </n-button>
      </n-space>
    </div>
    <div class="flex gap-2 items-center">
      <n-input
        v-model:value="searchKeyword"
        size="small"
        clearable
        placeholder="搜索世界或频道"
        @keyup.enter="handleSearch"
        @clear="() => (chat.worldLobbyMode === 'mine' ? fetchList() : fetchExploreList())"
      >
        <template #prefix>
          <n-icon size="14">
            <Search />
          </n-icon>
        </template>
      </n-input>
      <n-button size="small" type="primary" @click="handleSearch" :loading="loading">搜索</n-button>
    </div>
    <div class="flex gap-2 items-center">
      <n-input v-model:value="inviteSlug" size="small" placeholder="输入邀请码" />
      <n-button size="small" type="primary" :loading="joining" @click="consumeInvite">通过邀请码加入</n-button>
    </div>
    <template v-if="lobbyMode === 'mine'">
      <n-grid :cols="1" :x-gap="12" :y-gap="12">
        <n-gi>
          <n-card title="世界列表" class="sc-card-scroll">
            <div class="card-body-scroll space-y-2">
              <n-empty v-if="!filteredMineWorlds.length" description="暂无世界" />
              <div v-for="item in filteredMineWorlds" :key="item.world.id" class="world-row">
                <div class="flex items-start gap-2">
                  <n-button quaternary circle size="tiny" @click="toggleFavorite(item.world.id)">
                    <n-icon size="16" :color="chat.favoriteWorldIds.includes(item.world.id) ? '#f59e0b' : '#94a3b8'">
                      <component :is="chat.favoriteWorldIds.includes(item.world.id) ? Star : StarOff" />
                    </n-icon>
                  </n-button>
                  <div class="flex-1 min-w-0">
                    <div class="font-bold text-sm flex items-center gap-1">
                      {{ item.world.name }}
                      <n-tag v-if="chat.favoriteWorldIds.includes(item.world.id)" size="tiny" type="warning">收藏</n-tag>
                    </div>
                    <div class="text-xs text-gray-500 world-desc">{{ formatWorldDescription(item.world.description) }}</div>
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <n-tag
                    v-if="item.isMember"
                    size="small"
                    :type="getWorldRoleTag(item.memberRole).type"
                  >
                    {{ getWorldRoleTag(item.memberRole).label }}
                  </n-tag>
                  <n-button
                    v-if="item.isMember && item.memberRole !== 'owner'"
                    size="tiny"
                    quaternary
                    type="error"
                    @click="confirmLeaveWorld(item)"
                  >
                    退出
                  </n-button>
                  <n-button size="tiny" type="primary" @click="enterWorld(item.world.id)">进入</n-button>
                </div>
              </div>
            </div>
          </n-card>
        </n-gi>
      </n-grid>
    </template>

    <template v-else>
      <n-card title="探索世界" class="sc-card-scroll">
        <div class="card-body-scroll space-y-2">
          <n-empty v-if="!exploreWorlds.length" description="暂无公开世界" />
          <div v-for="item in exploreWorlds" :key="item.world.id" class="world-row">
            <div class="flex items-start gap-2">
              <n-button quaternary circle size="tiny" @click="toggleFavorite(item.world.id)">
                <n-icon size="16" :color="chat.favoriteWorldIds.includes(item.world.id) ? '#f59e0b' : '#94a3b8'">
                  <component :is="chat.favoriteWorldIds.includes(item.world.id) ? Star : StarOff" />
                </n-icon>
              </n-button>
              <div class="flex-1 min-w-0">
                <div class="font-bold text-sm flex items-center gap-1">
                  {{ item.world.name }}
                  <n-tag v-if="chat.favoriteWorldIds.includes(item.world.id)" size="tiny" type="warning">收藏</n-tag>
                </div>
                <div class="text-xs text-gray-500 world-desc">{{ formatWorldDescription(item.world.description) }}</div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <n-tag
                v-if="item.isMember"
                size="small"
                :type="getWorldRoleTag(item.memberRole).type"
              >
                {{ getWorldRoleTag(item.memberRole).label }}
              </n-tag>
              <n-button
                v-if="item.isMember && item.memberRole !== 'owner'"
                size="tiny"
                quaternary
                type="error"
                @click="confirmLeaveWorld(item)"
              >
                退出
              </n-button>
              <n-button size="tiny" type="primary" @click="enterWorld(item.world.id)">进入</n-button>
            </div>
          </div>
        </div>
      </n-card>
    </template>
    <n-modal v-model:show="createVisible" preset="dialog" title="创建世界" style="max-width: 420px">
      <n-form label-width="72">
        <n-form-item label="名称">
          <n-input v-model:value="createForm.name" placeholder="输入世界名称" />
        </n-form-item>
        <n-form-item label="简介">
          <n-input
            v-model:value="createForm.description"
            type="textarea"
            placeholder="简单介绍这个世界"
            maxlength="30"
            show-count
          />
        </n-form-item>
        <n-form-item label="可见性">
          <n-select
            v-model:value="createForm.visibility"
            :options="[
              { label: '公开', value: 'public' },
              { label: '私有', value: 'private' },
              { label: '隐藏链接', value: 'unlisted' },
            ]"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button quaternary @click="() => { createVisible = false; resetCreateForm(); }">取消</n-button>
          <n-button type="primary" :loading="creating" @click="handleCreateWorld">创建</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.sc-card-scroll {
  max-height: 420px;
}
.card-body-scroll {
  max-height: 360px;
  overflow: auto;
  padding-right: 4px;
}
.world-desc {
  white-space: pre-line;
}
.world-row {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: start;
  gap: 8px;
  padding: 8px;
  border-radius: 8px;
  transition: background-color 0.2s ease;
}
.world-row:hover {
  background-color: rgba(148, 163, 184, 0.12);
}
</style>
