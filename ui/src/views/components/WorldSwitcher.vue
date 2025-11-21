<script setup lang="ts">
import { computed, watch, ref, onMounted } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useMessage } from 'naive-ui';

const chat = useChatStore();
const message = useMessage();

const drawerVisible = defineModel<boolean>('visible', { default: false });
const worldForm = ref({ name: '', description: '', visibility: 'public' as string });
const showCreate = ref(false);
const worldJumpId = ref('');

const worldOptions = computed(() => chat.joinedWorldOptions);

watch(drawerVisible, async (visible) => {
  if (visible) {
    await chat.worldList({ page: 1, pageSize: 20, joined: true });
  }
});

onMounted(() => {
  chat.ensureWorldReady();
});

const handleSwitch = async (worldId: string) => {
  try {
    await chat.switchWorld(worldId, { force: true });
    drawerVisible.value = false;
  } catch (e) {
    message.error('切换世界失败');
  }
};

const handleWorldCreate = async () => {
  if (!worldForm.value.name.trim()) {
    message.error('请输入世界名称');
    return;
  }
  try {
    await chat.createWorld({ ...worldForm.value });
    showCreate.value = false;
    drawerVisible.value = false;
    worldForm.value = { name: '', description: '', visibility: 'public' };
    message.success('创建世界成功');
  } catch (e: any) {
    message.error(e?.response?.data?.message || '创建失败');
  }
};

const handleWorldJump = async () => {
  const id = worldJumpId.value.trim();
  if (!id) return;
  try {
    await chat.switchWorld(id, { force: true });
    worldJumpId.value = '';
    drawerVisible.value = false;
  } catch (e) {
    message.error('进入世界失败');
  }
};
</script>

<template>
  <n-drawer v-model:show="drawerVisible" width="60%">
    <n-drawer-content title="世界大厅" closable>
      <div class="space-y-3">
        <div class="flex gap-2">
          <n-input v-model:value="worldJumpId" placeholder="输入世界ID" size="small" />
          <n-button size="small" type="primary" @click="handleWorldJump">进入</n-button>
          <n-button size="small" quaternary @click="chat.worldList({ page: 1, pageSize: 20, joined: true })">刷新</n-button>
          <n-button size="small" type="primary" quaternary @click="showCreate = true">新建</n-button>
        </div>
        <n-list bordered>
          <template v-if="chat.worldListCache?.items?.length">
            <n-list-item v-for="item in chat.worldListCache?.items" :key="item.world.id">
              <div class="flex items-center justify-between">
                <div>
                  <div class="font-bold text-sm">{{ item.world.name }}</div>
                  <div class="text-xs text-gray-500">{{ item.world.description }}</div>
                </div>
                <div class="flex items-center gap-2">
                  <n-tag v-if="item.isMember" size="small" type="success">已加入</n-tag>
                  <n-button size="tiny" type="primary" @click="handleSwitch(item.world.id)">进入</n-button>
                </div>
              </div>
            </n-list-item>
          </template>
          <n-empty v-else description="暂无世界" />
        </n-list>
      </div>
    </n-drawer-content>
  </n-drawer>

  <n-modal v-model:show="showCreate" preset="dialog" title="新建世界" style="max-width:420px">
    <n-form label-width="72">
      <n-form-item label="名称">
        <n-input v-model:value="worldForm.name" placeholder="输入世界名称" />
      </n-form-item>
      <n-form-item label="简介">
        <n-input
          v-model:value="worldForm.description"
          type="textarea"
          placeholder="简单描述"
          maxlength="30"
          show-count
        />
      </n-form-item>
      <n-form-item label="可见性">
        <n-select
          v-model:value="worldForm.visibility"
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
        <n-button quaternary @click="showCreate = false">取消</n-button>
        <n-button type="primary" @click="handleWorldCreate">创建</n-button>
      </n-space>
    </template>
  </n-modal>
</template>
