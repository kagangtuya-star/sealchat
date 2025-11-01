<script setup lang="tsx">
import ChatItem from './components/chat-item.vue';
import { computed, ref, watch, h, onMounted, onBeforeMount, onBeforeUnmount, nextTick, type Component, reactive } from 'vue'
import { VirtualList } from 'vue-tiny-virtual-list';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { Event, Message } from '@satorijs/protocol'
import { useUserStore } from '@/stores/user';
import { ArrowBarToDown, Plus, Upload, Eye, EyeOff } from '@vicons/tabler'
import { NIcon, c, useDialog, useMessage, type MentionOption } from 'naive-ui';
import VueScrollTo from 'vue-scrollto'
import UploadSupport from './components/upload.vue'
import { liveQuery } from "dexie";
import { useObservable } from "@vueuse/rxjs";
import { db, getSrc, type Thumb } from '@/models';
import { throttle } from 'lodash-es';
import AvatarVue from '@/components/avatar.vue';
import { Howl, Howler } from 'howler';
import SoundMessageCreated from '@/assets/message.mp3';
import RightClickMenu from './components/ChatRightClickMenu.vue'
import AvatarClickMenu from './components/AvatarClickMenu.vue'
import { nanoid } from 'nanoid';
import { useUtilsStore } from '@/stores/utils';
import { contentEscape } from '@/utils/tools'
import IconNumber from '@/components/icons/IconNumber.vue'
import { computedAsync } from '@vueuse/core';
import type { UserEmojiModel } from '@/types';
import { Settings } from '@vicons/ionicons5';
import { dialogAskConfirm } from '@/utils/dialog';

// const uploadImages = useObservable<Thumb[]>(
//   liveQuery(() => db.thumbs.toArray()) as any
// )

const chat = useChatStore();
const user = useUserStore();

const emojiLoading = ref(false)
const uploadImages = computedAsync(async () => {
  if (user.emojiCount) {
    const resp = await user.emojiList();
    return resp.data.items;
  }
  return [];
}, [], emojiLoading);

const message = useMessage()
const dialog = useDialog()

// const virtualListRef = ref<InstanceType<typeof VirtualList> | null>(null);
const uploadSupportRef = ref<any>(null);
const messagesListRef = ref<HTMLElement | null>(null);
const textInputRef = ref<any>(null);

const rows = ref<Message[]>([]);

async function replaceUsernames(text: string) {
  const resp = await chat.guildMemberList('');
  const infoMap = (resp.data as any[]).reduce((obj, item) => {
    obj[item.nick] = item;
    return obj;
  }, {})

  // 匹配 @ 后跟着字母数字下划线的用户名
  const regex = /@(\S+)/g;

  // 使用 replace 方法来替换匹配到的用户名
  const replacedText = text.replace(regex, (match, username) => {
    if (username in infoMap) {
      const info = infoMap[username];
      return `<at id="${info.id}" name="${info.nick}" />`
    }
    return match;
  });

  return replacedText;
}

const instantMessages = reactive(new Set<Message>());

interface TypingPreviewItem {
  userId: string;
  displayName: string;
  avatar?: string;
  content: string;
  indicatorOnly: boolean;
}

const typingPreviewStorageKey = 'sealchat.typingPreviewEnabled';
const typingPreviewEnabled = ref(localStorage.getItem(typingPreviewStorageKey) === 'true');
const typingPreviewActive = ref(false);
const typingPreviewList = ref<TypingPreviewItem[]>([]);
let lastTypingChannelId = '';

const upsertTypingPreview = (item: TypingPreviewItem) => {
  typingPreviewList.value = typingPreviewList.value.filter((i) => i.userId !== item.userId);
  typingPreviewList.value.push(item);
};

const removeTypingPreview = (userId?: string) => {
  if (!userId) {
    return;
  }
  typingPreviewList.value = typingPreviewList.value.filter((item) => item.userId !== userId);
};

const resetTypingPreview = () => {
  typingPreviewList.value = [];
};

const sendTypingUpdate = throttle((enabled: boolean, content: string, channelId: string) => {
  chat.messageTyping(enabled, content, channelId);
}, 400, { leading: true, trailing: true });

const stopTypingPreviewNow = () => {
  sendTypingUpdate.cancel();
  if (typingPreviewActive.value && lastTypingChannelId) {
    chat.messageTyping(false, '', lastTypingChannelId);
  }
  typingPreviewActive.value = false;
  lastTypingChannelId = '';
};

const emitTypingPreview = () => {
  if (chat.connectState !== 'connected') return;
  const channelId = chat.curChannel?.id;
  if (!channelId) return;

  const raw = textToSend.value;
  if (raw.trim().length === 0) {
    stopTypingPreviewNow();
    return;
  }

  typingPreviewActive.value = true;
  lastTypingChannelId = channelId;
  const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
  const content = typingPreviewEnabled.value ? truncated : '';
  sendTypingUpdate(true, content, channelId);
};

const typingPreviewTooltip = computed(() =>
  typingPreviewEnabled.value ? '关闭实时输入预览' : '开启实时输入预览'
);

const toggleTypingPreview = () => {
  typingPreviewEnabled.value = !typingPreviewEnabled.value;
};

const textToSend = ref('');
const send = throttle(async () => {
  if (chat.connectState !== 'connected') {
    message.error('尚未连接，请稍等');
    return;
  }
  let t = textToSend.value;
  if (t.trim() === '') {
    message.error('不能发送空消息');
    return;
  }
  if (t.length > 10000) {
    message.error('消息过长，请分段发送');
    return;
  }
  let replyTo = chat.curReplyTo || undefined;
  stopTypingPreviewNow();
  textToSend.value = '';
  chat.curReplyTo = null;

  const now = Date.now();
  const tmpMsg: Message = {
    "id": nanoid(),
    "createdAt": now,
    "updatedAt": now,
    "content": t,
    "user": user.info,
    "member": chat.curMember || undefined,
    "quote": replyTo,
  };

  (tmpMsg as any).failed = false;
  rows.value.push(tmpMsg);
  instantMessages.add(tmpMsg);

  try {
    t = contentEscape(t)
    t = await replaceUsernames(t)

    tmpMsg.content = t;
    const newMsg = await chat.messageCreate(t, replyTo?.id);
    for (let [k, v] of Object.entries(newMsg)) {
      (tmpMsg as any)[k] = v;
    }
    instantMessages.delete(tmpMsg);
    // 从rows中删除tmpMsg，用id做匹配
    const index = rows.value.findIndex(msg => msg.id === tmpMsg.id);
    if (index !== -1) {
      rows.value.splice(index, 1);
    }
  } catch (e) {
    message.error('发送失败,您可能没有权限在此频道发送消息');
    console.error('消息发送失败', e);

    const index = rows.value.findIndex(msg => msg.id === tmpMsg.id);
    if (index !== -1) {
      (rows.value[index] as any).failed = true;
      // rows.value.splice(index, 1);
    }
  }

  scrollToBottom();
}, 500);

watch(textToSend, () => {
  emitTypingPreview();
});

watch(typingPreviewEnabled, (enabled) => {
  localStorage.setItem(typingPreviewStorageKey, enabled ? 'true' : 'false');
  if (!typingPreviewActive.value || !lastTypingChannelId) {
    return;
  }
  const raw = textToSend.value;
  if (raw.trim().length === 0) {
    return;
  }
  const truncated = raw.length > 500 ? raw.slice(0, 500) : raw;
  sendTypingUpdate.cancel();
  chat.messageTyping(true, enabled ? truncated : '', lastTypingChannelId);
});

const toBottom = () => {
  scrollToBottom();
  showButton.value = false;
}

const doUpload = () => {
  uploadSupportRef.value.openUpload();
}

const isMe = (item: Message) => {
  return user.info.id === item.user?.id;
}

const scrollToBottom = () => {
  // virtualListRef.value?.scrollToBottom();
  nextTick(() => {
    const elLst = messagesListRef.value;
    if (elLst) {
      elLst.scrollTop = elLst.scrollHeight;
    }
  });
}

const utils = useUtilsStore();

const emit = defineEmits(['drawer-show'])

let firstLoad = false;
onMounted(async () => {
  await chat.tryInit();
  await utils.configGet();
  await utils.commandsRefresh();

  chat.channelRefreshSetup()

  const elInput = textInputRef.value;
  if (elInput) {
    // 注: n-mention 不支持这个事件监听，所以这里手动监听
    elInput.$el.getElementsByTagName('textarea')[0].onkeydown = keyDown;
  }

  const sound = new Howl({
    src: [SoundMessageCreated],
    html5: true
  });

  chatEvent.off('message-deleted', '*');
  chatEvent.on('message-deleted', (e?: Event) => {
    console.log('delete', e?.message?.id)
    for (let i of rows.value) {
      if (i.id === e?.message?.id) {
        i.content = '';
        (i as any).is_revoked = true;
      }
      if (i.quote) {
        if (i.quote?.id === e?.message?.id) {
          i.quote.content = '';
          (i as any).quote.is_revoked = true;
        }
      }
    }
  });

  chatEvent.off('message-created', '*');
  chatEvent.on('message-created', (e?: Event) => {
    if (e && e.message && e.channel?.id == chat.curChannel?.id) {
      if (e.message.user?.id !== user.info.id) {
        // 不是自己发的消息，播放声音
        sound.play();
        rows.value.push(e.message);
      } else {
        // 自己发的消息，校准一下instantMessages
        let postByCurrentClient = false;
        for (let i of instantMessages) {
          if (i.id === e.message.id) {
            postByCurrentClient = true;
            instantMessages.delete(i);
            break;
          }
        }
        // 这里可能遇到多端登录情况
        if (!postByCurrentClient) {
          rows.value.push(e.message);
        }
      }
      removeTypingPreview(e.message.user?.id);
      if (!showButton.value) {
        scrollToBottom();
      }
    }
  });

  chatEvent.off('typing-preview', '*');
  chatEvent.on('typing-preview', (e?: Event) => {
    if (!e?.channel || e.channel.id !== chat.curChannel?.id) {
      return;
    }
    const typingUserId = e.user?.id;
    if (!typingUserId || typingUserId === user.info.id) {
      return;
    }
    if (!e.typing?.enabled) {
      removeTypingPreview(typingUserId);
      return;
    }
  upsertTypingPreview({
      userId: typingUserId,
      displayName: e.member?.nick || e.user?.nick || '未知成员',
      avatar: e.member?.avatar || e.user?.avatar || '',
      content: e.typing?.content || '',
      indicatorOnly: !e.typing?.content,
    });
  });

  chatEvent.off('channel-deleted', '*');
  chatEvent.on('channel-deleted', (e) => {
    if (e) {
      // 当前频道没了，直接进行重载
      chat.channelSwitchTo(chat.channelTree[0].id);
    }
  })

  chatEvent.on('channel-member-updated', (e) => {
    if (e) {
      // 此事件只有member
      for (let i of rows.value) {
        if (i.user?.id === e.member?.user?.id) {
          (i as any).member.nick = e?.member?.nick
        }
      }
      if ((chat.curMember as any).id === (e as any).member?.id) {
        chat.curMember = e.member as any;
      }
    }
  })

  chatEvent.on('connected', async (e) => {
    // 重连了之后，重新加载这之间的数据
    console.log('尝试获取重连数据')
    stopTypingPreviewNow();
    resetTypingPreview();
    if (rows.value.length > 0) {
      let now = Date.now();
      const lastCreatedAt = rows.value[rows.value.length - 1].createdAt || now;

      // 获取断线期间消息
      const messages = await chat.messageListDuring(chat.curChannel?.id || '', lastCreatedAt, now)
      console.log('时间起始', lastCreatedAt, now)
      console.log('相关数据', messages)
      if (messages.next) {
        //  如果大于30个，那么基本上清除历史
        messagesNextFlag.value = messages.next || "";
        rows.value = rows.value.filter((i) => i.createdAt || now > lastCreatedAt);
      }
      // 插入新数据
      rows.value.push(...messages.data);
      // 为防止混乱，重新排序
      rows.value.sort((a, b) => (a.createdAt || now) - (b.createdAt || now));

      // 滚动到最下方
      nextTick(() => {
        scrollToBottom();
        showButton.value = false;
      })
    } else {
      await loadMessages();
    }
  })

  chatEvent.on('channel-switch-to', (e) => {
    if (!firstLoad) return;
    stopTypingPreviewNow();
    resetTypingPreview();
    rows.value = []
    showButton.value = false;
    // 具体不知道原因，但是必须在这个位置reset才行
    // virtualListRef.value?.reset();
    loadMessages();
  })

  await loadMessages();
  firstLoad = true;
})

onBeforeUnmount(() => {
  stopTypingPreviewNow();
  resetTypingPreview();
});

const messagesNextFlag = ref("");

const loadMessages = async () => {
  resetTypingPreview();
  const messages = await chat.messageList(chat.curChannel?.id || '');
  messagesNextFlag.value = messages.next || "";
  rows.value.push(...messages.data);

  nextTick(() => {
    scrollToBottom();
    showButton.value = false;
  })
}

const showButton = ref(false)
const onScroll = (evt: any) => {
  // 会打断输入，不要blur
  // if (textInputRef.value?.blur) {
  //   (textInputRef.value as any).blur()
  // }
  // console.log(222, messagesListRef.value?.scrollTop, messagesListRef.value?.scrollHeight)
  if (messagesListRef.value) {
    const elLst = messagesListRef.value;
    const offset = elLst.scrollHeight - (elLst.clientHeight + elLst.scrollTop);
    showButton.value = offset > 200;

    if (elLst.scrollTop === 0) {
      //  首次加载前不触发
      if (!firstLoad) return;
      reachTop(evt);
    }
  }
  // const vl = virtualListRef.value;
  // showButton.value = vl.clientRef.itemRefEl.clientHeight - vl.getOffset() > vl.clientRef.itemRefEl.clientHeight / 2
}

const pauseKeydown = ref(false);
const keyDown = function (e: KeyboardEvent) {
  if (pauseKeydown.value) return;

  // 检查是否为移动端
  if (/Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)) {
    // 如果是移动端,直接返回,不执行后续代码
    return;
  }

  if (e.key === 'Enter' && (!e.ctrlKey) && (!e.shiftKey)) {
    send();
    e.preventDefault();

    // if (textInputRef.value?.blur) {
    //   (textInputRef.value as any).blur()
    // }
  }
}

const atOptions = ref<MentionOption[]>([])
const atLoading = ref(true)
const atRenderLabel = (option: MentionOption) => {
  switch (option.type) {
    case 'cmd':
      return <div class="flex items-center space-x-1">
        <span>{(option as any).data.info}</span>
      </div>
    case 'at':
      return <div class="flex items-center space-x-1">
        <AvatarVue size={24} border={false} src={(option as any).data?.avatar} />
        <span>{option.label}</span>
      </div>
  }
}

const atPrefix = computed(() => chat.atOptionsOn ? ['@', '/', '.'] : ['@']);

const atHandleSearch = async (pattern: string, prefix: string) => {
  pauseKeydown.value = true;
  atLoading.value = true;

  const atElementCheck = () => {
    const els = document.getElementsByClassName("v-binder-follower-content");
    if (els.length) {
      return els[0].children.length > 0;
    }
    return false;
  }

  // 如果at框非正常消失，那么也一样要恢复回车键功能
  let x = setInterval(() => {
    if (!atElementCheck()) {
      pauseKeydown.value = false;
      clearInterval(x);
    }
  }, 100)

  const cmdCheck = () => {
    const text = textToSend.value.trim();
    if (text.startsWith(prefix)) {
      return true;
    }
  }

  switch (prefix) {
    case '@': {
      const lst = (await chat.guildMemberList('')).data.map((i: any) => {
        return {
          type: 'at',
          value: i.nick,
          label: i.nick,
          data: i,
        }
      })
      atOptions.value = lst;
      break;
    }
    case '.': case '/':
      // 好像暂时没法组织他弹出
      // if (!cmdCheck()) {
      //   atLoading.value = false;
      //   pauseKeydown.value = false;
      //   return;
      // }

      if (chat.atOptionsOn) {
        atOptions.value = [[`x`, 'x d100'],].map((i) => {
          return {
            type: 'cmd',
            value: i[0],
            label: i[0],
            data: {
              "info": '/x 简易骰点指令，如：/x d100 (100面骰)'
            }
          }
        });

        for (let [id, data] of Object.entries(utils.botCommands)) {
          for (let [k, v] of Object.entries(data)) {
            atOptions.value.push({
              type: 'cmd',
              value: k,
              label: k,
              data: {
                "info": `/${k} ` + (v as any).split('\n', 1)[0].replace(/^\.\S+/, '')
              }
            })
          }
        }
      }
      break;
  }

  atLoading.value = false;
}

let recentReachTopNext = '';

const reachTop = throttle(async (evt: any) => {
  console.log('reachTop', messagesNextFlag.value)
  if (recentReachTopNext === messagesNextFlag.value) return;
  recentReachTopNext = messagesNextFlag.value;

  if (messagesNextFlag.value) {
    const messages = await chat.messageList(chat.curChannel?.id || '', messagesNextFlag.value);
    messagesNextFlag.value = messages.next || "";

    let oldId = '';
    if (rows.value.length) {
      oldId = rows.value[0].id || '';
    }

    rows.value.unshift(...messages.data);

    nextTick(() => {
      // 注意: el会变，如果不在下一帧取的话
      const el = document.getElementById(oldId)
      VueScrollTo.scrollTo(el, {
        container: messagesListRef.value,
        duration: 0,
        offset: 0,
      })
    })
    // virtualListRef.value?.scrollToIndex(messages.data.length);
  }
}, 1000)

const sendEmoji = throttle(async (i: UserEmojiModel) => {
  const resp = await chat.messageCreate(`<img src="id:${i.attachmentId}" />`);
  emojiPopoverShow.value = false;
  if (!resp) {
    message.error('发送失败,您可能没有权限在此频道发送消息');
    return;
  }
  toBottom();
}, 1000);

const avatarLongpress = (data: any) => {
  if (data.user) {
    textToSend.value += `@${data.user.nick} `;
    textInputRef.value?.focus();
  }
}

const selectedEmojiIds = ref<string[]>([]);

const emojiSelectedDelete = async () => {
  if (!await dialogAskConfirm(dialog)) return;

  if (selectedEmojiIds.value.length > 0) {
    await user.emojiDelete(selectedEmojiIds.value);
    // 例如：调用API删除表情，然后更新本地状态
    console.log('删除选中的表情：', selectedEmojiIds.value);
    // 删除后清空选中状态
    selectedEmojiIds.value = [];
    user.emojiCount++;
  } else {
    console.log('没有选中的表情可删除');
  }
}

const emojiPopoverShow = ref(false);
const isManagingEmoji = ref(false);
</script>

<template>
  <div class="flex flex-col h-full justify-between">
    <div class="chat overflow-y-auto h-full px-4 pt-6" v-show="rows.length > 0" @scroll="onScroll"
      ref="messagesListRef">
      <!-- <VirtualList itemKey="id" :list="rows" :minSize="50" ref="virtualListRef" @scroll="onScroll"
              @toBottom="reachBottom" @toTop="reachTop"> -->
      <template v-for="itemData in rows">
        <!-- {{itemData}} -->
        <chat-item :avatar="itemData.member?.avatar || itemData.user?.avatar" :username="itemData.member?.nick ?? '未知'"
          :content="itemData.content" :is-rtl="isMe(itemData)" :item="itemData"
          @avatar-longpress="avatarLongpress(itemData)" />
      </template>

      <template v-for="preview in typingPreviewList" :key="preview.userId">
        <div class="typing-preview-item">
          <AvatarVue :border="false" :size="40" :src="preview.avatar" />
          <div :class="['typing-preview-bubble', preview.indicatorOnly ? '' : 'typing-preview-bubble--content']">
            <div class="typing-preview-bubble__header">
              <div class="typing-preview-bubble__meta">
                <span class="typing-preview-bubble__name">{{ preview.displayName }}</span>
                <span class="typing-preview-bubble__tag">
                  {{ preview.indicatorOnly ? '正在输入' : '实时内容' }}
                </span>
              </div>
              <span class="typing-dots">
                <span></span>
                <span></span>
                <span></span>
              </span>
            </div>
            <div class="typing-preview-bubble__body" :class="{ 'typing-preview-bubble__placeholder': preview.indicatorOnly }">
              <template v-if="preview.indicatorOnly">
                正在输入
              </template>
              <template v-else>
                {{ preview.content }}
              </template>
            </div>
          </div>
        </div>
      </template>

      <!-- <VirtualList itemKey="id" :list="rows" :minSize="50" ref="virtualListRef" @scroll="onScroll"
              @toBottom="reachBottom" @toTop="reachTop">
              <template #default="{ itemData }">
                <chat-item :avatar="imgAvatar" :username="itemData.member?.nick" :content="itemData.content"
                  :is-rtl="isMe(itemData)" :createdAt="itemData.createdAt" />
              </template>
            </VirtualList> -->
    </div>
    <div v-if="rows.length === 0" class="flex h-full items-center text-2xl justify-center text-gray-400">说点什么吧</div>

    <div style="right: 20px ;bottom: 70px;" class=" fixed" v-if="showButton">
      <n-button size="large" circle color="#e5e7eb" @click="toBottom">
        <template #icon>
          <n-icon class="text-black">
            <ArrowBarToDown />
          </n-icon>
        </template>
      </n-button>
    </div>

    <!-- flex-grow -->
    <div class="edit-area flex justify-between space-x-2 my-2 px-2 relative">

      <!-- 左下，快捷指令栏 -->
      <div class="absolute  px-4 py-2" style="top: -2.7rem; left: 0rem" v-if="true">
        <div class="bg-white">
          <n-button @click="emit('drawer-show')" size="small" v-if="utils.isSmallPage">
            <template #icon>
              <n-icon :component="IconNumber"></n-icon>
            </template>
          </n-button>
        </div>
      </div>

      <div class="absolute bg-sky-300 rounded px-4 py-2" style="top: -4rem; right: 1rem" v-if="chat.curReplyTo">
        正在回复: {{ chat.curReplyTo.member?.nick }}
        <n-button @click="chat.curReplyTo = null">取消</n-button>
      </div>

      <div class="flex justify-between relative w-full">
        <!-- 输入框左侧按钮，因为n-mention不支持#prefix和#suffix，所以单独拿出来了 -->
        <div class="absolute" style="z-index: 1; left: 0.5rem; top: .55rem;">
          <n-popover v-model:show="emojiPopoverShow" trigger="click">
            <template #trigger>
              <n-button text>
                <template #icon>
                  <n-icon :component="Plus" size="20" />
                </template>
              </n-button>
            </template>

            <div class="flex justify-between items-center">
              <div class="text-base mb-1">{{ $t('inputBox.emojiTitle') }}</div>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button text size="small" @click="isManagingEmoji = !isManagingEmoji">
                    <template #icon>
                      <n-icon :component="Settings" />
                    </template>
                  </n-button>
                </template>
                表情管理
              </n-tooltip>
            </div>

            <div v-if="!uploadImages?.length" class="flex justify-center w-full py-4 px-4">
              <div class="w-56">当前没有收藏的表情，可以在聊天窗口的图片上<b class="px-1">长按</b>或<b class="px-1">右键</b>添加</div>
            </div>

            <template v-else>
              <template v-if="isManagingEmoji">
                <n-checkbox-group v-model:value="selectedEmojiIds">
                  <div class="grid grid-cols-4 gap-4 pt-2 pb-4">
                    <div class="cursor-pointer" v-for="i in uploadImages" :key="i.id">
                      <n-checkbox :value="i.id" class="mt-2">
                        <img :src="getSrc(i)"
                          style="width: 4.8rem; height: 4.8rem; object-fit: contain; cursor: pointer;" />
                      </n-checkbox>
                    </div>
                  </div>
                </n-checkbox-group>

                <div class="flex justify-end space-x-2 mb-4">
                  <n-button type="info" size="small" @click="emojiSelectedDelete" :disabled="selectedEmojiIds.length === 0">
                    删除选中
                  </n-button>
                  <n-button type="default" size="small" @click="() => { isManagingEmoji = false; selectedEmojiIds = []; }" class="mr-2">
                    退出管理
                  </n-button>
                </div>
              </template>

              <template v-else>
                <div class="grid grid-cols-4 gap-4 pt-2 pb-4">
                  <div class="cursor-pointer" v-for="i in uploadImages" :key="i.id">
                    <img @click="sendEmoji(i)" :src="getSrc(i)"
                      style="width: 4.8rem; height: 4.8rem; object-fit: contain;" />
                  </div>
                </div>
              </template>
            </template>
          </n-popover>
        </div>

        <div class="absolute flex items-center space-x-1" style="z-index: 1; right: 0.6rem; top: .55rem;">
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button text class="typing-toggle" :class="{ 'typing-toggle--active': typingPreviewEnabled }"
                @click="toggleTypingPreview">
                <template #icon>
                  <n-icon :component="typingPreviewEnabled ? Eye : EyeOff" size="20" />
                </template>
              </n-button>
            </template>
            {{ typingPreviewTooltip }}
          </n-tooltip>

          <n-popover trigger="hover">
            <template #trigger>
              <n-button text @click="doUpload">
                <template #icon>
                  <n-icon :component="Upload" size="20" />
                </template>
              </n-button>
            </template>
            <span>上传图片</span>
          </n-popover>
        </div>

        <n-mention type="textarea" :rows="1" autosize v-model:value="textToSend" :on-keydown="keyDown"
          ref="textInputRef" class="chat-text" :placeholder="$t('inputBox.placeholder')" :options="atOptions"
          :loading="atLoading" @search="atHandleSearch" @select="pauseKeydown = false" placement="top-start"
          :prefix="atPrefix" :render-label="atRenderLabel">
        </n-mention>
      </div>
      <div class="flex" style="align-items: end; padding-bottom: 1px;">
        <n-button class="" type="primary" @click="send" :disabled="chat.connectState !== 'connected'">{{
          $t('inputBox.send') }}</n-button>
      </div>
    </div>
  </div>

  <RightClickMenu />
  <AvatarClickMenu />
  <upload-support ref="uploadSupportRef" />
</template>

<style lang="scss" scoped>
.chat>.virtual-list__client {
  @apply px-4 pt-4;

  &>div {
    margin-bottom: -1rem;
  }
}

.chat-item {
  @apply pb-8; // margin会抖动，pb不会
}

.typing-preview-item {
  display: flex;
  align-items: flex-end;
  gap: 0.75rem;
  margin-top: 0.75rem;
  font-size: 0.9375rem;
  color: #4b5563;
}

.typing-preview-bubble {
  max-width: 28rem;
  padding: 0.6rem 0.9rem;
  border-radius: 1rem;
  border: 1px dashed rgba(107, 114, 128, 0.65);
  background-color: rgba(243, 244, 246, 0.85);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(2px);
}

.typing-preview-bubble--content {
  border-color: rgba(59, 130, 246, 0.55);
  background-color: rgba(219, 234, 254, 0.95);
  color: #1d4ed8;
}

.typing-preview-bubble__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.35rem;
}

.typing-preview-bubble__meta {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.typing-preview-bubble__name {
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: #4b5563;
}

.typing-preview-bubble--content .typing-preview-bubble__name {
  color: #1e3a8a;
}

.typing-preview-bubble__tag {
  font-size: 0.625rem;
  padding: 0.1rem 0.4rem;
  border-radius: 9999px;
  background-color: rgba(156, 163, 175, 0.18);
  color: #4b5563;
  font-weight: 500;
}

.typing-preview-bubble--content .typing-preview-bubble__tag {
  background-color: rgba(59, 130, 246, 0.18);
  color: #1d4ed8;
}

.typing-preview-bubble__body {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  font-size: 0.9375rem;
}

.typing-preview-bubble__placeholder {
  color: #6b7280;
}

.typing-dots {
  display: inline-flex;
  align-items: center;
}

.typing-dots span {
  width: 0.35rem;
  height: 0.35rem;
  margin-left: 0.18rem;
  border-radius: 9999px;
  background-color: rgba(107, 114, 128, 0.9);
  animation: typing-dots 1.2s infinite ease-in-out;
}

.typing-preview-bubble--content .typing-dots span {
  background-color: rgba(37, 99, 235, 0.85);
}

.typing-dots span:first-child {
  margin-left: 0;
}

.typing-dots span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-dots span:nth-child(3) {
  animation-delay: 0.4s;
}

.typing-toggle {
  color: #9ca3af;
  transition: color 0.2s ease;
}

.typing-toggle--active {
  color: #2563eb;
}

.typing-toggle:hover {
  color: #3b82f6;
}

@keyframes typing-dots {
  0%, 80%, 100% {
    transform: scale(0.4);
    opacity: 0.35;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}
</style>

<style lang="scss">
.chat>.virtual-list__client {
  &>div {
    margin-bottom: -1rem;
  }
}

.chat-text>.n-input>.n-input-wrapper {
  @apply bg-gray-200;
}

.chat-text>.n-input>.n-input-wrapper {
  padding-left: 2rem;
  padding-right: 4rem;
}
</style>
