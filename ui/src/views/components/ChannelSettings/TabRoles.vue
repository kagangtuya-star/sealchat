<script lang="tsx" setup>
import { PermResult, type PermCheckKey, type PermTreeNode } from '@/types-perm';
import { computed, ref, watch, type PropType } from 'vue';
import { useChatStore } from '@/stores/chat';
import useRequest from 'vue-hooks-plus/es/useRequest';
import type { SChannel } from '@/types';
import { useDialog, useMessage } from 'naive-ui';
import { dialogAskConfirm, dialogError, dialogInput } from '@/utils/dialog';
import { clone, flatMap } from 'lodash-es';
import { coverErrorMessage } from '@/utils/request';

const chat = useChatStore();

const dialog = useDialog();
const message = useMessage();


const props = defineProps({
  channel: {
    type: Object as PropType<SChannel>,
  }
});


// 至少一个二级菜单才能被渲染，不过应该不是大问题
// let permTable = ref<PermTreeNode[]>([]);

const { data: permTable } = useRequest(async () => {
  const resp = await chat.channelPermTree();
  return resp.items;
}, {});


const { data: roleList } = useRequest(async () => {
  if (!props.channel?.id) return { items: [], page: 1, pageSize: 1, total: 0 };
  const resp = await chat.channelRoleList(props.channel.id);

  if (!selectedRole.value && resp.data.items.length > 0) {
    selectedRole.value = resp.data.items[0].id;
  }
  return resp.data;
}, {
  initialData: { items: [], page: 1, pageSize: 1, total: 0 },
});

const selectedRole = ref<string>();

const perm = ref<{ [K in PermCheckKey]?: boolean }>({});
const permStart = ref<{ [K in PermCheckKey]?: boolean }>({});
const defaultRoleSuffixes = ['-owner', '-member', '-bot', '-ob', '-spectator'];
const permSyncDialogVisible = ref(false);
const permSyncSourceChannelId = ref<string | null>(null);
const permSyncMode = ref<'append' | 'replace'>('append');
const permSyncChannelOptions = ref<Array<{ label: string; value: string }>>([]);
const permSyncLoading = ref(false);
const permSyncSubmitting = ref(false);

const allChannelKeys = computed(() => {
  // 从权限树中递归提取所有的 modelName
  const allKeys = flatMap(permTable.value, function traverse(node): string[] {
    const keys: string[] = [];
    if (node.modelName) {
      keys.push(node.modelName);
    }
    if (node.children) {
      keys.push(...flatMap(node.children, traverse));
    }
    return keys;
  });

  return allKeys as PermCheckKey[];
})

watch(selectedRole, async (roleId) => {
  if (!props.channel?.id) return;
  if (!roleId) {
    perm.value = {} as any;
    return;
  }

  const resp = await chat.channelRolePermsGet(props.channel?.id, roleId);
  const permLst = resp.data as PermCheckKey[];

  const m = {} as { [K in PermCheckKey]?: boolean };
  for (let i of allChannelKeys.value) {
    m[i as PermCheckKey] = false;
  }

  for (let i of permLst) {
    m[i] = true;
  }

  permStart.value = clone(m);
  perm.value = m;
});


const permModified = computed(() => {
  if (!permStart.value || !perm.value) return false;

  for (const key of allChannelKeys.value) {
    if (permStart.value[key] !== perm.value[key]) {
      return true;
    }
  }
  return false;
});

const resolveSelectedRole = () => {
  const roleId = selectedRole.value;
  if (!roleId) return null;
  return roleList.value?.items?.find(role => role.id === roleId) || null;
};

const resolveSourceRoleId = (roles: Array<{ id: string; name: string }>, targetRole: { id: string; name: string }) => {
  const suffix = defaultRoleSuffixes.find(item => targetRole.id.endsWith(item));
  if (suffix) {
    const matched = roles.find(role => role.id.endsWith(suffix));
    if (matched?.id) {
      return matched.id;
    }
  }
  const matchedByName = roles.find(role => role.name === targetRole.name);
  return matchedByName?.id || null;
};

const loadPermSyncChannelOptions = async () => {
  if (!props.channel?.worldId) {
    permSyncChannelOptions.value = [];
    return;
  }
  permSyncLoading.value = true;
  try {
    const list = await chat.channelFavoriteCandidateList(props.channel.worldId, true);
    const options = (Array.isArray(list) ? list : [])
      .filter(channel => {
        if (!channel?.id) return false;
        if (channel.id === props.channel?.id) return false;
        if (channel.isPrivate) return false;
        if (channel.permType === 'private') return false;
        return true;
      })
      .map(channel => ({
        label: channel.name || '未命名频道',
        value: channel.id,
      }));
    permSyncChannelOptions.value = options;
  } catch (error) {
    console.warn('加载同步频道列表失败', error);
    permSyncChannelOptions.value = [];
  } finally {
    permSyncLoading.value = false;
  }
};

const openPermSyncDialog = async () => {
  if (!selectedRole.value) {
    message.warning('请先选择角色');
    return;
  }
  permSyncDialogVisible.value = true;
  await loadPermSyncChannelOptions();
};

const resetPermSyncDialog = () => {
  permSyncSourceChannelId.value = null;
  permSyncMode.value = 'append';
};

const handlePermSync = async () => {
  if (!props.channel?.id) {
    message.error('目标频道不存在');
    return;
  }
  if (!selectedRole.value) {
    message.warning('请先选择角色');
    return;
  }
  if (!permSyncSourceChannelId.value) {
    message.warning('请选择来源频道');
    return;
  }
  if (permSyncSourceChannelId.value === props.channel.id) {
    message.warning('不能选择当前频道');
    return;
  }
  if (!allChannelKeys.value.length) {
    message.warning('权限树未加载完成');
    return;
  }
  const targetRole = resolveSelectedRole();
  if (!targetRole) {
    message.error('未找到目标角色');
    return;
  }

  permSyncSubmitting.value = true;
  try {
    const sourceRoleResp = await chat.channelRoleList(permSyncSourceChannelId.value);
    const sourceRoles = sourceRoleResp.data?.items || [];
    const sourceRoleId = resolveSourceRoleId(sourceRoles, targetRole);
    if (!sourceRoleId) {
      message.error('来源频道未找到匹配角色');
      return;
    }

    const sourcePermResp = await chat.channelRolePermsGet(permSyncSourceChannelId.value, sourceRoleId);
    const sourcePermList = (sourcePermResp.data || []) as PermCheckKey[];
    const sourcePermSet = new Set<PermCheckKey>(sourcePermList);

    const currentPermSet = new Set<PermCheckKey>();
    for (const [key, value] of Object.entries(perm.value)) {
      if (value) {
        currentPermSet.add(key as PermCheckKey);
      }
    }

    const nextPermSet = new Set<PermCheckKey>();
    if (permSyncMode.value === 'append') {
      currentPermSet.forEach((key) => nextPermSet.add(key));
      sourcePermSet.forEach((key) => nextPermSet.add(key));
    } else {
      sourcePermSet.forEach((key) => nextPermSet.add(key));
    }

    const toAdd = Array.from(nextPermSet).filter(key => !currentPermSet.has(key));
    const toRemove = permSyncMode.value === 'replace'
      ? Array.from(currentPermSet).filter(key => !nextPermSet.has(key))
      : [];

    const removeText = permSyncMode.value === 'replace' ? `，移除 ${toRemove.length} 项` : '';
    if (!(await dialogAskConfirm(dialog, '同步权限', `将新增 ${toAdd.length} 项${removeText}，是否继续？`))) {
      return;
    }

    const nextMap = {} as { [K in PermCheckKey]?: boolean };
    for (const key of allChannelKeys.value) {
      nextMap[key] = nextPermSet.has(key);
    }

    await chat.rolePermsSet(selectedRole.value, Array.from(nextPermSet));
    perm.value = clone(nextMap);
    permStart.value = clone(nextMap);
    message.success('同步完成');
    permSyncDialogVisible.value = false;
  } catch (error) {
    console.error('同步权限失败:', error);
    message.error('同步权限失败，请确认你拥有权限');
  } finally {
    permSyncSubmitting.value = false;
  }
};

watch(
  () => permSyncDialogVisible.value,
  (visible) => {
    if (!visible) {
      resetPermSyncDialog();
    }
  },
);



const roleAdd = async () => {
  if (await dialogInput(dialog, '请输入角色名')) {
    if (!props.channel?.id) return;
    // await api.post('api/v1/channel-role-create', {
    //   name: name.value,
    //   channelId: props.channel.id
    // });  
    // roleList.refresh();
    message.success('添加成功');
  }
};

const roleDelete = async () => {
  if (!props.channel?.id || !selectedRole) return;
  if (await dialogAskConfirm(dialog)) {
    // await chat.channelRoleDelete(props.channel!.id, selectedRole);
  }
};

const roleSave = async () => {
  if (!props.channel?.id || !selectedRole.value) return;

  const permList = [] as string[];
  for (const [key, value] of Object.entries(perm.value)) {
    if (value) {
      permList.push(key);
    }
  }

  const showErr = (title: string, text: string) => {
    dialogError(dialog, title, text)
  }

  await coverErrorMessage(async () => {
    if (!selectedRole.value) return;
    await chat.rolePermsSet(selectedRole.value, permList);

    permStart.value = clone(perm.value);
    message.success('保存成功');
  }, showErr);
};
</script>

<template>
  <div class="mb-4 flex space-x-2 flex-col">
    <div class="pl-2 mb-4">
      <!-- {{ roleList }} -->
      <div class="flex justify-between items-center mb-2">
        <div>当前编辑角色:</div>
        <div class="space-x-2">
          <n-button size="small" class="perm-sync-btn" :disabled="!selectedRole" @click="openPermSyncDialog">同步权限</n-button>
          <n-button size="small" :disabled="!permModified" type="success" @click="roleSave">保存</n-button>
          <!-- 想了一下暂时没有必要 先摸了
          <n-button size="small" type="primary" @click="roleAdd">添加</n-button>
          <n-button size="small" type="error" @click="roleDelete" :disabled="!selectedRole">删除</n-button> -->
        </div>
      </div>
      <n-select class="w-48" placeholder="选择角色" :options="roleList?.items?.map(role => ({
        label: role.name,
        value: role.id
      })) || []" v-model:value="selectedRole" />
    </div>

    <div class=" overflow-x-hidden overflow-y-auto" style="height: 58vh;;">
      <span class="text-gray-500 text-sm pl-2 mb-2">请注意，并不是所有权限都实装了，慢慢更新中</span>
      <n-table :bordered="true" :single-line="false">
        <thead>
          <tr>
            <th style="position: sticky; top:0">模块</th>
            <th rowspan="2" style="white-space: nowrap;">页面</th>
            <th style="">功能</th>
          </tr>
        </thead>
        <tbody>

          <template v-for="(i, iIndex) in permTable">
            <template v-for="(j, jIndex) in i.children">
              <tr>
                <td :rowspan="i.children?.length || 1" v-if="jIndex === 0">
                  <n-checkbox v-if="i.modelName" v-model:checked="perm[i.modelName]">{{ i.name }}</n-checkbox>
                  <span v-else>{{ i.name }}</span>
                </td>
                <td class="whitespace-nowrap">
                  <n-checkbox v-if="j.modelName" v-model:checked="perm[j.modelName]">{{ j.name }}</n-checkbox>
                  <span v-else>{{ j.name }}</span>
                </td>
                <td class="t3">
                  <template v-for="k in j.children">
                    <n-checkbox v-if="k.modelName" v-model:checked="perm[k.modelName]">{{ k.name }}</n-checkbox>
                  </template>
                </td>
              </tr>
            </template>
          </template>
        </tbody>
      </n-table>
    </div>

    <n-modal
      v-model:show="permSyncDialogVisible"
      preset="dialog"
      title="同步权限"
      style="max-width: 520px"
    >
      <div class="perm-sync-modal">
        <div class="perm-sync-field">
          <div class="perm-sync-label">来源频道</div>
          <n-select
            v-model:value="permSyncSourceChannelId"
            :options="permSyncChannelOptions"
            placeholder="选择要同步的频道"
            size="small"
            filterable
            clearable
            :loading="permSyncLoading"
          />
          <div v-if="!permSyncLoading && permSyncChannelOptions.length === 0" class="perm-sync-hint">
            暂无可同步频道
          </div>
        </div>
        <div class="perm-sync-field">
          <div class="perm-sync-label">同步模式</div>
          <n-radio-group v-model:value="permSyncMode" size="small">
            <n-radio value="append">追加</n-radio>
            <n-radio value="replace">覆盖</n-radio>
          </n-radio-group>
          <div class="perm-sync-hint">
            覆盖模式会移除当前角色中不在来源角色的权限
          </div>
        </div>
        <div class="perm-sync-footer">
          <n-button size="small" :disabled="permSyncSubmitting" @click="permSyncDialogVisible = false">
            取消
          </n-button>
          <n-button size="small" type="primary" :loading="permSyncSubmitting" @click="handlePermSync">
            开始同步
          </n-button>
        </div>
      </div>
    </n-modal>

  </div>
</template>

<style lang="scss">
.perm-sync-btn {
  --n-color: var(--n-card-color, var(--n-color, #f8fafc));
  --n-color-hover: var(--n-color-hover, var(--n-color, #eef2f7));
  --n-color-pressed: var(--n-color-pressed, var(--n-color, #e2e8f0));
  --n-text-color: var(--n-text-color-2, var(--n-text-color, #1f2937));
  --n-border: 1px solid var(--n-border-color, rgba(148, 163, 184, 0.4));
}

:root[data-display-palette='night'] .perm-sync-btn {
  --n-color: var(--n-card-color, rgba(30, 41, 59, 0.65));
  --n-color-hover: var(--n-color-hover, rgba(51, 65, 85, 0.75));
  --n-color-pressed: var(--n-color-pressed, rgba(51, 65, 85, 0.9));
  --n-text-color: var(--n-text-color-2, #e2e8f0);
  --n-border: 1px solid var(--n-border-color, rgba(148, 163, 184, 0.3));
}

.perm-sync-modal {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.perm-sync-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.perm-sync-label,
.perm-sync-hint {
  font-size: 12px;
  color: var(--n-text-color-3, rgba(100, 116, 139, 0.9));
}

.perm-sync-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 4px;
}
</style>
