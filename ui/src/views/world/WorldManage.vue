<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useWorldStore } from '@/stores/world';
import { useUserStore } from '@/stores/user';
import WorldMembersPanel from '@/components/world/WorldMembersPanel.vue';
import WorldInvitesPanel from '@/components/world/WorldInvitesPanel.vue';
import WorldRolesPanel from '@/components/world/WorldRolesPanel.vue';
import { NTabs, NTabPane, NButton, NModal, NForm, NFormItem, NInput, NSelect, NDivider, NAlert, useMessage } from 'naive-ui';

const worldStore = useWorldStore();
const userStore = useUserStore();
const router = useRouter();
const route = useRoute();
const activeTab = ref('members');
const message = useMessage();

const showEdit = ref(false);
const editing = ref({
  name: '',
  description: '',
  avatar: '',
  banner: '',
  visibility: 'public',
  joinPolicy: 'open',
});
const showDelete = ref(false);
const canManageMembers = computed(() => {
  const ownerId = worldStore.currentWorld?.ownerId;
  if (ownerId && ownerId === userStore.info.id) {
    return true;
  }
  return !!userStore.checkPerm('func_world_manage');
});

const isOwner = computed(() => worldStore.currentWorld?.ownerId === userStore.info.id);
const canEditWorld = computed(() => isOwner.value);

const syncForm = () => {
  if (!worldStore.currentWorld) return;
  const w = worldStore.currentWorld;
  editing.value = {
    name: w.name,
    description: w.description || '',
    avatar: w.avatar || '',
    banner: w.banner || '',
    visibility: w.visibility || 'public',
    joinPolicy: w.joinPolicy || 'open',
  };
};

const load = async () => {
  const slug = String(route.params.slug || '');
  await worldStore.fetchWorldBySlug(slug);
  await Promise.all([worldStore.fetchMembers(), worldStore.fetchInvites()]);
  syncForm();
};

onMounted(load);
watch(() => route.params.slug, load);

const handleMemberRemove = async (member: any) => {
  if (!canManageMembers.value) {
    message.warning('没有权限移除此成员');
    return;
  }
  if (!worldStore.currentWorld?.id) return;
  if (!window.confirm(`确定将 ${member.nickname || member.userId} 移出世界吗？`)) {
    return;
  }
  try {
    await worldStore.removeMember(member.userId);
    message.success('已移除成员');
  } catch (err: any) {
    message.error(err?.response?.data?.message || '移除失败');
  }
};

const handleSave = async () => {
  if (!worldStore.currentWorld?.id) return;
  try {
    await worldStore.updateWorld(editing.value as any);
    message.success('已更新世界信息');
    showEdit.value = false;
  } catch (err: any) {
    message.error(err?.response?.data?.message || '更新失败');
  }
};

const handleDelete = async () => {
  if (!worldStore.currentWorld?.id) return;
  try {
    await worldStore.deleteWorld(worldStore.currentWorld.id);
    message.success('世界已删除');
    showDelete.value = false;
    router.push('/worlds');
  } catch (err: any) {
    message.error(err?.response?.data?.message || '删除失败');
  }
};
</script>

<template>
  <div class="max-w-5xl mx-auto p-4 space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <h2 class="text-2xl font-semibold">{{ worldStore.currentWorld?.name || '世界管理' }}</h2>
        <p class="text-gray-500 text-sm">
          管理世界成员、邀请与角色
        </p>
      </div>
      <div class="space-x-2">
        <n-button tertiary @click="$router.push('/worlds')">返回大厅</n-button>
        <n-button type="primary" secondary :disabled="!canEditWorld" @click="() => { syncForm(); showEdit = true; }">编辑信息</n-button>
        <n-button type="error" tertiary :disabled="!canEditWorld" @click="() => showDelete = true">删除世界</n-button>
      </div>
    </div>

    <n-tabs type="line" v-model:value="activeTab">
      <n-tab-pane name="members" tab="成员">
        <WorldMembersPanel
          :members="worldStore.members"
          :loading="worldStore.membersLoading"
          :owner-id="worldStore.currentWorld?.ownerId"
          :disable-remove="!canManageMembers"
          @remove="handleMemberRemove"
        />
      </n-tab-pane>
      <n-tab-pane name="invites" tab="邀请链接">
        <WorldInvitesPanel
          :invites="worldStore.invites"
          :loading="worldStore.invitesLoading"
          @refresh="worldStore.fetchInvites()"
        />
      </n-tab-pane>
      <n-tab-pane name="roles" tab="角色与权限">
        <WorldRolesPanel />
      </n-tab-pane>
    </n-tabs>

    <n-modal v-model:show="showEdit" preset="dialog" title="编辑世界信息" :mask-closable="false">
      <n-form label-width="80" label-placement="top">
        <n-form-item label="名称">
          <n-input v-model:value="editing.name" placeholder="输入世界名称" />
        </n-form-item>
        <n-form-item label="简介">
          <n-input v-model:value="editing.description" type="textarea" placeholder="世界简介" />
        </n-form-item>
        <n-form-item label="图标/Avatar">
          <n-input v-model:value="editing.avatar" placeholder="图片 URL" />
        </n-form-item>
        <n-form-item label="封面/Banner">
          <n-input v-model:value="editing.banner" placeholder="图片 URL" />
        </n-form-item>
        <n-form-item label="可见性">
          <n-select v-model:value="editing.visibility" :options="[
            { label: '公开', value: 'public' },
            { label: '私密', value: 'private' },
          ]" />
        </n-form-item>
        <n-form-item label="加入策略">
          <n-select v-model:value="editing.joinPolicy" :options="[
            { label: '开放加入', value: 'open' },
            { label: '需审批', value: 'approval' },
            { label: '仅邀请', value: 'invite_only' },
          ]" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-button @click="showEdit = false">取消</n-button>
        <n-button type="primary" @click="handleSave">保存</n-button>
      </template>
    </n-modal>

    <n-modal v-model:show="showDelete" preset="dialog" type="error" title="删除世界" :mask-closable="false">
      <n-alert type="error" title="危险操作" class="mb-3">
        删除后将无法恢复，聊天记录、频道和成员关系都会被移除。
      </n-alert>
      <n-divider />
      <div class="space-x-2 text-right">
        <n-button @click="showDelete = false">取消</n-button>
        <n-button type="error" @click="handleDelete">确认删除</n-button>
      </div>
    </n-modal>
  </div>
</template>
