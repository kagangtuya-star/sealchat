<script lang="tsx" setup>
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import type { FriendRequestModel, SChannel, UserInfo } from '@/types';
import { dialogAskConfirm } from '@/utils/dialog';
import type { Channel } from 'diagnostics_channel';
import { useDialog, useMessage } from 'naive-ui';
import { computed, ref } from 'vue';
import useRequest from 'vue-hooks-plus/es/useRequest'

const user = useUserStore();
const chat = useChatStore();

const dialog = useDialog();
const message = useMessage();

const loadList = async () => {
  await chat.ChannelPrivateList();
  return chat.channelTreePrivate;
}

// 获取好友列表
const { data: userRelationList, run: friendListReload, loading, cancel } = useRequest(loadList, {
  manual: true,
  pollingInterval: 10000, // 10s一次
  pollingWhenHidden: false,
  onSuccess: (val) => {
    // TODO: 这样做其实并不太好，不过我是初用 useRequest，之后再调整吧
    // chat.channelTreePrivate = val;
  }
})
friendListReload();


// 获取用户申请列表
const { data: userRequestList, loading: loadingUserRequestList } = useRequest(chat.friendRequestList, {
  pollingInterval: 10000, // 10s一次
  pollingWhenHidden: false,
})


// 我正在加的用户的列表
const { data: userRequestingList, run: requestingReload } = useRequest(chat.friendRequestingList, {
  pollingInterval: 10000, // 10s一次
  pollingWhenHidden: false,
})

// 是否正在加对方好友
const isFriendRequesting = (userId: string) => {
  return userRequestingList.value?.some(request => request.receiverId === userId) || false;
};


const friendsList = computed(() => {
  return userRelationList.value?.filter(item => item.friendInfo?.isFriend === true) || [];
});

const strangersList = computed(() => {
  return userRelationList.value?.filter(item => item.friendInfo?.isFriend === false) || [];
});

const unreadCountPrivateFriend = computed(() => {
  return friendsList.value.reduce((sum, friend) => {
    return sum + (chat.unreadCountMap[friend.id] || 0);
  }, 0);
});

const unreadCountPrivateStranger = computed(() => {
  return strangersList.value.reduce((sum, stranger) => {
    return sum + (chat.unreadCountMap[stranger.id] || 0);
  }, 0);
});


const addFriend = async (userId: string) => {
  try {
    await chat.friendRequestCreate(user.info.id, userId, '');
    requestingReload();
  } catch (error) {
    console.error('添加好友失败:', error);
  }
}


const doChannelSwitch = async (i: SChannel) => {
  await chat.channelSwitchTo(i.id);
}

const friendRequestModalShow = ref(false);
const friendRequestItem = ref<FriendRequestModel>();

const modelShow = (show = true, req?: FriendRequestModel, note = '') => {
  friendRequestModalShow.value = show;
  if (req) {
    req.userInfoTemp = req.userInfoSender;
    friendRequestItem.value = req;
  }
}

const friendDelete = async (userId: string | undefined) => {
  if (!userId) return;
  if (await dialogAskConfirm(dialog, '是否解除好友关系？')) {
    try {
      await chat.friendDelete(userId);
      message.success('已解除好友关系');
      friendListReload();
    } catch (error) {
      console.error('解除好友关系失败:', error);
      message.error('解除好友关系失败');
    }
  }
}


const handleApprove = async (accept: boolean) => {
  if (!friendRequestItem.value?.id) return;
  try {
    const ret = await chat.friendRequestApprove(friendRequestItem.value.id, accept);
    requestingReload();
    friendListReload();
    if (ret) {
      message.success('已同意好友请求');
    } else {
      message.error('操作未能完成');
    }
  } catch (error) {
    console.error('同意好友请求失败:', error);
    message.error('同意好友请求失败');
  }
}

const handleAccept = async () => {
  await handleApprove(true);
}

const handleReject = async () => {
  await handleApprove(false);
}
</script>

<template>
  <n-collapse default-expanded-names="friends">
    <n-collapse-item :title="`好友申请 (${userRequestList?.length ?? 0})`" name="invite">
      <!-- 这里添加陌生人列表 -->
      <div v-for="item in userRequestList" :key="item.id" class="sider-item">
        <UserLabel
          style="max-width: 11rem"
          :name="item.userInfoSender?.nick"
          :src="item.userInfoSender?.avatar"
          :decoration="item.userInfoSender?.avatarDecoration"
        />
        <div>
          <n-button size="tiny" type="info" secondary @click="modelShow(true, item, item.note)">查看</n-button>
        </div>
      </div>
    </n-collapse-item>

    <n-collapse-item :title="`好友 (${friendsList.length})`" name="friends">
      <template #header>
        <div class="flex items-center w-full">
          <span>好友 ({{ friendsList.length }})</span>
          <div class="label-unread ml-3" v-if="unreadCountPrivateFriend">
            {{ unreadCountPrivateFriend }}
          </div>
        </div>
      </template>

      <!-- 这里添加好友列表 -->
      <div @click="doChannelSwitch(item)" v-for="item in friendsList" :key="item.id" class="sider-item"
        :class="item.id === chat.curChannel?.id ? ['active'] : []">
        <UserLabel
          style="max-width: 11rem"
          :name="item.friendInfo?.userInfo?.nick"
          :src="item.friendInfo?.userInfo?.avatar"
          :decoration="item.friendInfo?.userInfo?.avatarDecoration"
        />
        <div class="flex space-x-1 items-center">
          <div class="" v-if="chat.unreadCountMap[item.id]">
            <div class="label-unread">
              {{ chat.unreadCountMap[item.id] > 99 ? '99+' : chat.unreadCountMap[item.id] }}
            </div>
          </div>

          <n-button size="tiny" type="info" secondary
            @click.stop="friendDelete(item.friendInfo?.userInfo?.id)">解除</n-button>
        </div>
      </div>
    </n-collapse-item>
    <n-collapse-item :title="`陌生人 (${strangersList.length})`" name="strangers">
      <template #header>
        <div class="flex items-center w-full">
          <span>陌生人 ({{ strangersList.length }})</span>
          <div class="label-unread ml-3" v-if="unreadCountPrivateStranger">
            {{ unreadCountPrivateStranger }}
          </div>
        </div>
      </template>

      <!-- 这里添加陌生人列表 -->
      <div @click="doChannelSwitch(item)" v-for="item in strangersList" :key="item.id" class="sider-item"
        :class="item.id === chat.curChannel?.id ? ['active'] : []">
        <UserLabel
          style="max-width: 11rem"
          :name="item.friendInfo?.userInfo?.nick"
          :src="item.friendInfo?.userInfo?.avatar"
          :decoration="item.friendInfo?.userInfo?.avatarDecoration"
        />
        <div class="flex space-x-1 items-center">
          <n-tooltip trigger="hover" v-if="!isFriendRequesting(item.friendInfo?.userInfo?.id ?? '')">
            <template #trigger>
              <n-button size="tiny" type="primary" @click.stop="addFriend(item.friendInfo?.userInfo?.id ?? '')">
                + 好友
              </n-button>
            </template>
            添加 {{ item.friendInfo?.userInfo?.nick }} 为好友
          </n-tooltip>
          <div v-else>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-text type="success">等待验证</n-text>
              </template>
              已发送好友请求，等待对方验证
            </n-tooltip>
          </div>

          <div class="" v-if="chat.unreadCountMap[item.id]">
            <div class="label-unread">
              {{ chat.unreadCountMap[item.id] > 99 ? '99+' : chat.unreadCountMap[item.id] }}
            </div>
          </div>
        </div>
      </div>
    </n-collapse-item>

  </n-collapse>

  <n-modal v-model:show="friendRequestModalShow" preset="dialog" positive-text="同意" negative-text="拒绝"
    @positive-click="handleAccept" @negative-click="handleReject" :icon="undefined">
    <template #icon>
      <span></span>
    </template>

    <template #header>
      <div class="flex items-center -ml-8">
        <UserLabel
          :name="friendRequestItem?.userInfoTemp?.nick"
          :src="friendRequestItem?.userInfoTemp?.avatar"
          :decoration="friendRequestItem?.userInfoTemp?.avatarDecoration"
        />
        <span class="ml-2">({{ friendRequestItem?.userInfoTemp?.username }}) 的好友申请</span>
      </div>
    </template>

    <div class="mt-4">
      <p>留言: {{ friendRequestItem?.note || '无' }}</p>
    </div>
  </n-modal>
</template>
