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

  </div>
</template>
