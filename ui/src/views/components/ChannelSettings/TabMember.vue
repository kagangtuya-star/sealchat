<script lang="tsx" setup>
import { ChannelType, type ChannelRoleModel, type SChannel, type UserInfo, type UserRoleModel } from '@/types';
import { clone, times, uniqBy } from 'lodash-es';
import { useDialog, useMessage } from 'naive-ui';
import { computed, onMounted, onUnmounted, ref, watch, type PropType } from 'vue';
import UserLabelV from '@/components/UserLabelV.vue'
import BtnPlus from './BtnPlus.vue';
import useRequest from 'vue-hooks-plus/es/useRequest';
import { useChatStore, chatEvent } from '@/stores/chat';
import { dialogAskConfirm } from '@/utils/dialog';
import MemberSelector from './MemberSelector.vue';
import { useUserStore } from '@/stores/user';

const message = useMessage();
const dialog = useDialog();

const chat = useChatStore();
const userStore = useUserStore();
const currentUserId = computed(() => userStore.info.id);

interface WorldMemberItem extends UserInfo {
  role?: string;
}


const props = defineProps({
  channel: {
    type: Object as PropType<SChannel>,
  }
})

const model = ref<SChannel>({
  id: '',
  type: 0, // 0 text
})

const dataLoad = async () => {
  if (!props.channel?.id) return undefined;
  const resp = await chat.channelRoleList(props.channel.id);
	if (resp.data && resp.data.items) {
	const owners: ChannelRoleModel[] = [];
	const members: ChannelRoleModel[] = [];
	const others: ChannelRoleModel[] = [];
	for (const item of resp.data.items) {
		if (item.id.endsWith('-owner')) {
			owners.push(item);
			continue;
		}
		if (item.id.endsWith('-member')) {
			members.push(item);
			continue;
		}
		others.push(item);
	}

	resp.data.items = [...owners, ...members, ...others].filter(item => !item.id.endsWith('-visitor'));
	}
  return resp.data;
}

const dataLoadMember = async () => {
  if (!props.channel?.id) return undefined;
  const resp = await chat.channelMemberList(props.channel.id);
  return resp.data;
}

// 我正在加的用户的列表
const { data: roleList, run: doReload } = useRequest(dataLoad, {})
const { data: memberList, run: doMemberReload } = useRequest(dataLoadMember, {})

if (props.channel) {
  model.value = clone(props.channel);
}

const filterMembersByChannelId = (roleId: string) => {
  if (!memberList.value || !memberList.value.items) {
    return [];
  }
  return memberList.value.items.filter(member =>
    member.roleId == roleId
  );
};

const removeUserRole = async (userId: string | undefined, roleId: string) => {
  if (!userId) {
    message.error('用户ID不存在');
    return;
  }

  if (!canRemoveMember(roleId, userId)) {
    message.error('无法移除自己的群主身份');
    return;
  }

  if (await dialogAskConfirm(dialog, '确认移除', '您确定要移除此项角色关联吗？')) {
    try {
      await chat.userRoleUnlink(roleId, [userId]);
      await doMemberReload();

      message.success('成员已成功移除');
    } catch (error) {
      console.error('移除成员失败:', error);
      message.error('移除成员失败，请确认你拥有权限');
    }
  }
}


const worldMembers = ref<WorldMemberItem[]>([]);

const loadWorldMembers = async () => {
  if (!props.channel?.worldId) {
    worldMembers.value = [];
    return;
  }
  try {
    const resp = await chat.worldMemberList(props.channel.worldId, { page: 1, pageSize: 500 });
    const items = resp?.items || [];
    worldMembers.value = items.map(item => ({
      id: item.userId,
      username: item.username,
      nick: item.nickname,
      avatar: item.avatar,
      role: item.role,
    })) as WorldMemberItem[];
  } catch (error) {
    console.error('加载世界成员失败:', error);
    message.error('加载世界成员失败');
  }
};

watch(
  () => props.channel?.worldId,
  () => {
    loadWorldMembers();
  },
  { immediate: true },
);


const botList = ref<UserInfo[]>([]);

const fetchBotList = async (force = false) => {
  try {
    const response = await chat.botList(force);
    const lst = [];
    for (let i of response?.items || []) {
      lst.push(i);
    }
    botList.value = lst;
  } catch (error) {
    console.error('获取机器人列表失败:', error);
    message.error('获取机器人列表失败，请重试');
  }
};

fetchBotList();

const handleBotListUpdated = async () => {
  await fetchBotList(true);
};

chatEvent.on('bot-list-updated', handleBotListUpdated as any);
onUnmounted(() => {
  chatEvent.off('bot-list-updated', handleBotListUpdated as any);
});


const selectedMembersSet = async (role: ChannelRoleModel, lst: string[], oldLst: string[]) => {
  // 计算需要移除和添加的成员
  const toRemove = oldLst.filter(id => !lst.includes(id));
  const toAdd = lst.filter(id => !oldLst.includes(id));

  if (role.id.endsWith('-owner')) {
    const selfId = currentUserId.value;
    if (selfId && toRemove.includes(selfId)) {
      message.error('无法移除自己的群主身份');
      return;
    }
  }

  console.log('需要移除的成员:', toRemove);
  console.log('需要添加的成员:', toAdd);

  try {
    if (toAdd.length) await chat.userRoleLink(role.id, toAdd);
    if (toRemove.length) await chat.userRoleUnlink(role.id, toRemove);
    await doMemberReload();
    message.success('成员已成功添加');
  } catch (error) {
    console.error('修改成员失败:', error);
    message.error('修改成员失败，请确认你拥有权限');
  }
};

const channelMemberRole = computed(() => roleList.value?.items?.find(role => role.id.endsWith('-member')));

const eligibleWorldMembers = computed(() => {
  return worldMembers.value.filter((member) => {
    const role = member.role || 'member';
    return role === 'owner' || role === 'admin' || role === 'member';
  });
});

const addEligibleWorldMembers = async () => {
  const role = channelMemberRole.value;
  if (!role) {
    message.error('未找到成员角色');
    return;
  }
  const candidateIds = eligibleWorldMembers.value.map(member => member.id).filter(Boolean) as string[];
  if (!candidateIds.length) {
    message.info('没有可添加的成员');
    return;
  }
  const existingIds = new Set(
    filterMembersByChannelId(role.id).map(member => member.user?.id).filter(Boolean) as string[],
  );
  const toAdd = candidateIds.filter(id => !existingIds.has(id));
  if (!toAdd.length) {
    message.info('所有成员已在频道中');
    return;
  }
  if (!(await dialogAskConfirm(dialog, '一键添加成员', `将添加 ${toAdd.length} 位成员到频道成员角色，是否继续？`))) {
    return;
  }
  try {
    await chat.userRoleLink(role.id, toAdd);
    await doMemberReload();
    message.success(`已添加 ${toAdd.length} 位成员`);
  } catch (error) {
    console.error('批量添加成员失败:', error);
    message.error('批量添加成员失败，请确认你拥有权限');
  }
};

const getFilteredMemberList = (lst?: UserRoleModel[]) => {
  const retLst = (lst ?? []).map(i => i.user).filter(i => i != undefined);
  return uniqBy(retLst, 'id') as any as UserInfo[]; // 部分版本中编译器对类型有误判
};

const canRemoveMember = (roleId: string, userId?: string) => {
  if (!userId) {
    return true;
  }
  if (!currentUserId.value) {
    return true;
  }
  return !(roleId.endsWith('-owner') && userId === currentUserId.value);
};
</script>

<template>
  <div class="overflow-y-auto" style="height: 60vh;">

    <div v-for="i in roleList?.items" class="border-b pb-1 mb-4">
      <!-- <div>{{ i }}</div> -->
      <h3 class="text-base font-semibold mt-2 text-gray-800 role-title">{{ i.name }}</h3>
      <div class="text-gray-500 role-desc mb-2">
        <span
          v-if="i.id.endsWith('-owner')"
          class="font-semibold"
        >你可以添加当前世界用户为成员，使之可查看此非公开频道。（只有先设定为成员才能在其他角色设定列表找到用户！）</span>
        <span v-if="i.id.endsWith('-ob')">此角色能够看到所有的子频道</span>
        <span v-if="i.id.endsWith('-bot')">此角色能够在所有子频道中收发消息</span>
        <span v-if="i.id.endsWith('-spectator')">旁观者仅可查看频道内容，无法发送消息</span>
      </div>

      <div class="flex justify-end mb-2" v-if="i.id.endsWith('-member')">
        <n-button
          size="small"
          secondary
          class="member-quick-add-btn"
          :disabled="eligibleWorldMembers.length === 0"
          @click="addEligibleWorldMembers"
        >
          一键添加世界成员到此频道
        </n-button>
      </div>

      <div class="flex flex-wrap space-x-2 ">
        <div class="relative group" v-for="j in filterMembersByChannelId(i.id)">
          <UserLabelV :name="j.user?.nick ?? j.user?.username" :src="j.user?.avatar" />
          <div class="flex justify-center">
            <n-button class=" opacity-0 group-hover:opacity-100 transition-opacity" size="tiny" type="error"
              :disabled="!canRemoveMember(j.roleId, j.user?.id)"
              @click="removeUserRole(j.user?.id, j.roleId)">
              移除
            </n-button>
          </div>
        </div>

        <n-popover trigger="click" placement="bottom">
          <template #trigger>

            <n-tooltip trigger="hover" placement="top">
              <template #trigger>
                <BtnPlus />
              </template>
              添加成员
            </n-tooltip>

          </template>
          <div class="max-h-60 overflow-y-auto pt-2">
            <MemberSelector v-if="i.id.endsWith('-member')" :memberList="worldMembers"
              :startSelectedList="getFilteredMemberList(filterMembersByChannelId(i.id))"
              @confirm="(lst, startLst) => selectedMembersSet(i, lst, startLst ?? [])" />
            <MemberSelector v-else-if="i.id.endsWith('-bot')" :memberList="botList"
              :startSelectedList="getFilteredMemberList(filterMembersByChannelId(i.id))"
              @confirm="(lst, startLst) => selectedMembersSet(i, lst, startLst ?? [])" />
            <MemberSelector v-else :memberList="getFilteredMemberList(memberList?.items)"
              :startSelectedList="getFilteredMemberList(filterMembersByChannelId(i.id))"
              @confirm="(lst, startLst) => selectedMembersSet(i, lst, startLst ?? [])" />
          </div>
        </n-popover>
      </div>
    </div>

  </div>
</template>

<style lang="scss">
:root[data-display-palette='night'] .role-title {
  color: #f4f4f5 !important;
}

:root[data-display-palette='night'] .role-desc {
  color: #d4d4d8 !important;
}

.member-quick-add-btn {
  --n-color: var(--n-card-color, var(--n-color, #f8fafc));
  --n-color-hover: var(--n-color-hover, var(--n-color, #eef2f7));
  --n-color-pressed: var(--n-color-pressed, var(--n-color, #e2e8f0));
  --n-text-color: var(--n-text-color-2, var(--n-text-color, #1f2937));
  --n-border: 1px solid var(--n-border-color, rgba(148, 163, 184, 0.4));
}

:root[data-display-palette='night'] .member-quick-add-btn {
  --n-color: var(--n-card-color, rgba(30, 41, 59, 0.65));
  --n-color-hover: var(--n-color-hover, rgba(51, 65, 85, 0.75));
  --n-color-pressed: var(--n-color-pressed, rgba(51, 65, 85, 0.9));
  --n-text-color: var(--n-text-color-2, #e2e8f0);
  --n-border: 1px solid var(--n-border-color, rgba(148, 163, 184, 0.3));
}
</style>
