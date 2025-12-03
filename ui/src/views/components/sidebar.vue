<script setup lang="tsx">
import router from '@/router';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { useWorldGlossaryStore } from '@/stores/worldGlossary';
import { Plus } from '@vicons/tabler';
import { Menu, SettingsSharp, ChevronDown, ChevronForward } from '@vicons/ionicons5';
import { NIcon, useDialog, useMessage } from 'naive-ui';
import { ref, type Component, h, defineAsyncComponent, watch, onMounted, onUnmounted } from 'vue';
import Notif from '../notif.vue'
import UserProfile from './user-profile.vue'
import { useI18n } from 'vue-i18n'
import { setLocale, setLocaleByNavigator } from '@/lang';
import type { Channel } from '@satorijs/protocol';
import type { SChannel } from '@/types';
import IconNumber from '@/components/icons/IconNumber.vue'
import IconFluentMention24Filled from '@/components/icons/IconFluentMention24Filled.vue'
import ChannelSettings from './ChannelSettings/ChannelSettings.vue'
import ChannelCreate from './ChannelCreate.vue'
import UserLabel from '@/components/UserLabel.vue'
import { Setting } from '@icon-park/vue-next';
import SidebarPrivate from './sidebar-private.vue';
import ChannelSortModal from './ChannelSortModal.vue';

const { t } = useI18n()

const notifShow = ref(false)
const userProfileShow = ref(false)
const adminShow = ref(false)
const chat = useChatStore();
const user = useUserStore();
const worldGlossary = useWorldGlossaryStore();

const renderIcon = (icon: Component) => {
  return () => {
    return h(NIcon, null, {
      default: () => h(icon)
    })
  }
}

const message = useMessage()
const usernameOverlap = ref(false);
const dialog = useDialog()

const showModal = ref(false);

const doChannelSwitch = async (i: Channel) => {
  const success = await chat.channelSwitchTo(i.id);
  if (!success) {
    message.error('切换频道失败，你可能没有权限');
  }
}

const showModal2 = ref(false);
const channelToSettings = ref<SChannel | undefined>(undefined);
const doSetting = async (i: Channel) => {
  channelToSettings.value = i;
  showModal2.value = true;
}

const handleOpenMemberSettings = () => {
  if (!chat.curChannel) {
    return;
  }
  channelToSettings.value = chat.curChannel as SChannel;
  showModal2.value = true;
};

chatEvent.on('channel-member-settings-open', handleOpenMemberSettings);
onUnmounted(() => {
  chatEvent.off('channel-member-settings-open', handleOpenMemberSettings as any);
});

import { useSpeechRecognition } from '@vueuse/core'

// const {
//   isSupported,
//   isListening,
//   isFinal,
//   result,
//   start,
//   stop,
// } = useSpeechRecognition()

const speech = useSpeechRecognition({
  lang: 'zh-CN',
  interimResults: true,
  continuous: true,
})

const { isListening, isSupported, stop, result } = speech

if (isSupported.value) {
  // @ts-expect-error missing types
  const SpeechGrammarList = window.SpeechGrammarList || window.webkitSpeechGrammarList
  if (SpeechGrammarList) {
    const speechRecognitionList = new SpeechGrammarList()
    // speechRecognitionList.addFromString(grammar, 1)
    speech.recognition!.grammars = speechRecognitionList

    watch(speech.result, () => {
    })
  }
}

const startA = () => {
  speech.result.value = ''
  speech.start()
}

import { useSpeechSynthesis } from '@vueuse/core'

const voice = ref<SpeechSynthesisVoice>(undefined as unknown as SpeechSynthesisVoice)
const voices = ref<SpeechSynthesisVoice[]>([])

const synth = useSpeechSynthesis(speech.result, {
  voice,
  pitch: 1,
  rate: 1,
  volume: 1,
})

onMounted(() => {
  if (speech.isSupported.value) {
    // load at last
    setTimeout(() => {
      const synth = window.speechSynthesis
      voices.value = synth.getVoices()
      voice.value = voices.value[0]
    })
  }
})

const speak = () => {
  if (synth.status.value === 'pause') {
    console.log('resume')
    window.speechSynthesis.resume()
  }
  else {
    synth.speak()
  }
}

const parentId = ref('');

const canShowDissolve = (channel?: SChannel) => {
  if (!channel?.id) return false;
  const userId = user.info.id;
  if (!userId) return false;
  return chat.isChannelOwner(channel.id, userId) || chat.isChannelAdmin(channel.id, userId);
};

const ensureChannelManagePermission = async (channelId: string) => {
  if (!channelId) return false;
  const userId = user.info.id;
  if (!userId) return false;
  if (chat.isChannelOwner(channelId, userId)) {
    return true;
  }
  try {
    if (await chat.hasChannelPermission(channelId, 'func_channel_manage_info', userId)) {
      return true;
    }
    if (await chat.hasChannelPermission(channelId, 'func_channel_manage_role_root', userId)) {
      return true;
    }
  } catch (error) {
    console.warn('check channel manage perm failed', error);
  }
  return false;
};

const handleChannelDissolve = async (channel: SChannel) => {
  if (!channel?.id) return;
  const allowed = await ensureChannelManagePermission(channel.id);
  if (!allowed) {
    message.error('只有群主或管理员可以解散该频道');
    return;
  }

  dialog.warning({
    title: '解散频道',
    content: `确认要解散「${channel.name}」吗？该操作不可恢复。`,
    positiveText: '解散',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await chat.channelDissolve(channel.id);
        message.success('频道已解散');
      } catch (error: any) {
        message.error(error?.response?.data?.error || '解散失败，请重试');
        return false;
      }
      return true;
    },
  });
};

const handleSelect = async (key: string, data: any) => {
  switch (key) {
    case 'enter':
      await doChannelSwitch(data.item);
      break;
    case 'addSubChannel':
      // 实现添加子频道的逻辑
      parentId.value = data.item.id;
      showModal.value = true;
      break;
    case 'manage':
      // 实现管理频道的逻辑
      doSetting(data.item);
      break;
    case 'leave':
      // 实现退出频道的逻辑
      alert('未实现');
      break;
    case 'dissolve':
      await handleChannelDissolve(data.item as SChannel);
      break;
    default:
      break;
  }
}

const suffix = (item: SChannel) => {
  if (item.permType === 'non-public') {
    return '[*]'
  }
  return ''
}


const aaa = ref(false);

const toggleParentCollapse = (channelId: string) => {
  chat.toggleChannelCollapse(channelId);
};

const handleCollapseAllChannels = () => {
  chat.collapseAllChannelGroups(true);
};

const handleWorldSelect = async (value: string) => {
  if (!value) return;
  try {
    await chat.switchWorld(value, { force: true });
    router.push({ name: 'home' });
  } catch (error) {
    message.error('切换世界失败');
  }
};

const handleChannelSortEntry = () => {
  showSortModal.value = true;
};

const showSortModal = ref(false);

const goWorldLobby = () => {
  router.push({ name: 'world-lobby' });
};

const goWorldManage = () => {
  if (chat.currentWorldId) {
    router.push({ name: 'world-detail', params: { worldId: chat.currentWorldId } });
  } else {
    router.push({ name: 'world-lobby' });
  }
};

const handleOpenWorldGlossary = () => {
  const worldId = chat.currentWorldId;
  if (!worldId) {
    message.warning('请选择一个世界');
    return;
  }
  worldGlossary.ensureKeywords(worldId, { force: true });
  worldGlossary.setManagerVisible(true);
};
</script>

<template>
  <div class="w-full h-full sc-sidebar sc-sidebar-fill">
    <n-tabs type="segment" v-model:value="chat.sidebarTab" tab-class="sc-sidebar-fill" pane-class="sc-sidebar-fill">
      <n-tab-pane name="channels" tab="频道">
        <div class="px-2 py-2 flex flex-wrap gap-2 items-center">
          <div class="flex-1 min-w-[180px]">
            <n-select
              class="w-full"
              size="small"
              filterable
              placeholder="选择世界"
              :options="chat.joinedWorldOptions"
              :value="chat.currentWorldId"
              @update:value="handleWorldSelect"
            />
          </div>
          <div class="flex gap-2 flex-wrap">
            <n-button quaternary size="tiny" @click="goWorldLobby">
              世界大厅
            </n-button>
            <n-button quaternary size="tiny" @click="goWorldManage">
              世界管理
            </n-button>
            <n-button
              quaternary
              size="tiny"
              :type="worldGlossary.managerVisible ? 'primary' : 'default'"
              @click="handleOpenWorldGlossary"
            >
              术语管理
            </n-button>
          </div>
        </div>
        <template #tab>
          <span>频道</span>
          <div class="ml-1" v-if="chat.unreadCountPublic">
            <div class="label-unread">
              {{ chat.unreadCountPublic }}
            </div>
          </div>
        </template>

        <!-- 频道列表内容将在这里显示 -->
        <div class="space-y-1 flex flex-col px-2">
          <template v-if="chat.curChannel">
            <!-- <template v-if="false"> -->
            <template v-for="i in chat.channelTree">
              <div class="sider-item" :class="i.id === chat.curChannel?.id ? ['active'] : []"
                @click="doChannelSwitch(i)">

                <div class="flex space-x-1 items-center">
                  <n-button
                    v-if="(i.children?.length ?? 0) > 0"
                    quaternary
                    circle
                    size="tiny"
                    class="channel-collapse-trigger"
                    :aria-expanded="!chat.channelCollapseState[i.id]"
                    :aria-label="chat.channelCollapseState[i.id] ? '展开子频道' : '折叠子频道'"
                    @click.stop="toggleParentCollapse(i.id)"
                  >
                    <template #icon>
                      <n-icon :component="chat.channelCollapseState[i.id] ? ChevronForward : ChevronDown" />
                    </template>
                  </n-button>
                  <template v-if="(i.type === 3 || (i as any).isPrivate)">
                    <!-- 私聊 -->
                    <n-icon :component="IconFluentMention24Filled"></n-icon>
                    <span>{{ `${i.name}` }}</span>
                  </template>

                  <template v-else>
                    <!-- 公开频道 -->
                    <n-icon :component="IconNumber"></n-icon>
                    <span class="text-more" style="max-width: 10rem">{{ `${i.name}${suffix(i)} (${(i as any).membersCount})` }}</span>
                  </template>
                </div>

        <div class="right-num" v-if="chat.unreadCountMap[i.id]">
          <div class="label-unread">
            {{ chat.unreadCountMap[i.id] > 99 ? '99+' : chat.unreadCountMap[i.id] }}
          </div>
        </div>

                <div class="right">
                  <div class="flex justify-center space-x-1">
                    <n-dropdown trigger="click" :options="[
                      { label: '进入', key: 'enter', item: i },
                      { label: '添加子频道', key: 'addSubChannel', show: !Boolean(i.parentId), item: i },
                      { label: '频道管理', key: 'manage', item: i },
                      { label: '退出', key: 'leave', item: i, show: i.permType === 'non-public' },
                      { label: '解散', key: 'dissolve', item: i, show: canShowDissolve(i as SChannel) }
                    ]" @select="handleSelect">
                      <n-button @click.stop quaternary circle size="tiny">
                        <template #icon>
                          <n-icon>
                            <Menu />
                          </n-icon>
                        </template>
                      </n-button>
                    </n-dropdown>
                    <n-button quaternary circle size="tiny" @click.stop="handleSelect('manage', { item: i })">
                      <template #icon>
                        <SettingsSharp />
                      </template>
                    </n-button>
                  </div>
                </div>

              </div>

              <!-- 当前频道的用户列表（已注释：避免在侧栏重复展示在线成员） -->
              <!--
              <div class="pl-5 mt-2 space-y-2" v-if="i.id == chat.curChannel.id && chat.curChannelUsers.length">
                <UserLabel :name="j.nick" :src="j.avatar" v-for="j in chat.curChannelUsers" />
              </div>
              -->

              <div v-if="(i.children?.length ?? 0) > 0 && !chat.channelCollapseState[i.id]">
                <template v-for="child in i.children">
                  <div class="sider-item" :class="child.id === chat.curChannel?.id ? ['active'] : []"
                    @click="doChannelSwitch(child)">
                    <div class="flex space-x-1 items-center pl-4">
                      <n-button
                        v-if="(child.children?.length ?? 0) > 0"
                        quaternary
                        circle
                        size="tiny"
                        class="channel-collapse-trigger"
                        :aria-expanded="!chat.channelCollapseState[child.id]"
                        :aria-label="chat.channelCollapseState[child.id] ? '展开子频道' : '折叠子频道'"
                        @click.stop="toggleParentCollapse(child.id)"
                      >
                        <template #icon>
                          <n-icon :component="chat.channelCollapseState[child.id] ? ChevronForward : ChevronDown" />
                        </template>
                      </n-button>
                      <template v-if="(child.type === 3 || (child as any).isPrivate)">
                        <n-icon :component="IconFluentMention24Filled"></n-icon>
                        <span>{{ `${child.name}` }}</span>
                      </template>
                      <template v-else>
                        <n-icon :component="IconNumber"></n-icon>
                        <span class="text-more" style="max-width: 9.5rem">{{ `${child.name}${suffix(child)} (${(child as any).membersCount})` }}</span>
                      </template>
                    </div>

                    <div class="right-num" v-if="chat.unreadCountMap[child.id]">
                      <div class="label-unread">
                        {{ chat.unreadCountMap[child.id] > 99 ? '99+' : chat.unreadCountMap[child.id] }}
                      </div>
                    </div>

                    <div class="right">
                      <div class="flex justify-center space-x-1">
                        <n-dropdown trigger="click" :options="[
                          { label: '进入', key: 'enter', item: child },
                          { label: '频道管理', key: 'manage', item: child },
                          { label: '退出', key: 'leave', item: i, show: i.permType === 'non-public' },
                          { label: '解散', key: 'dissolve', item: child, show: canShowDissolve(child as SChannel) }
                        ]" @select="handleSelect">
                          <n-button @click.stop quaternary circle size="tiny">
                            <template #icon>
                              <n-icon>
                                <Menu />
                              </n-icon>
                            </template>
                          </n-button>
                        </n-dropdown>

                        <n-button quaternary circle size="tiny" @click.stop="handleSelect('manage', { item: child })">
                          <template #icon>
                            <SettingsSharp />
                          </template>
                        </n-button>
                      </div>

                    </div>
                  </div>

                  <!-- 当前频道的用户列表（已注释：避免在侧栏重复展示在线成员） -->
                  <!--
                  <div class="pl-8 mt-2 space-y-2" v-if="child.id == chat.curChannel.id && chat.curChannelUsers.length">
                    <UserLabel :name="j.nick" :src="j.avatar" v-for="j in chat.curChannelUsers" />
                  </div>
                  -->
                </template>
              </div>

            </template>

          </template>
          <template v-else>
            <div class="px-6">加载中 ...</div>
          </template>

          <div class="sider-item" @click="parentId = ''; showModal = true">
            <div class="flex space-x-1 items-center font-bold">
              <n-icon :component="Plus"></n-icon>
              <span>{{ t('channelListNew') }}</span>
            </div>
          </div>

          <div class="sidebar-footer-actions">
            <n-button size="tiny" quaternary block @click="handleCollapseAllChannels">
              折叠全部
            </n-button>
            <n-button size="tiny" quaternary block @click="handleChannelSortEntry">
              频道排序
            </n-button>
          </div>
        </div>
      </n-tab-pane>
      <n-tab-pane name="privateChats" tab="私聊">
        <template #tab>
          <span>私聊</span>
          <div class="ml-1" v-if="chat.unreadCountPrivate">
            <div class="label-unread">
              {{ chat.unreadCountPrivate }}
            </div>
          </div>
        </template>
        <!-- 私聊列表内容将在这里显示 -->
        <SidebarPrivate />
      </n-tab-pane>
    </n-tabs>
  </div>


  <!-- <div v-if="!isSupported">
      Your browser does not support SpeechRecognition API,
      <a href="https://caniuse.com/mdn-api_speechrecognition" target="_blank">more details</a>
    </div>
    <div v-else class="mt-8">
      <n-button v-if="!isListening" @click="startA">
        按下说话
      </n-button>
      <n-button v-if="isListening" class="orange" @click="stop">
        停止
      </n-button>
      <div v-if="isListening" class="">
        {{ speech.result }}
      </div>

      <div class="mt-8">
        <select v-model="voice" px-8 border-0 bg-transparent h-9 rounded appearance-none>
          <option bg="$vp-c-bg" disabled>
            Select Language
          </option>
          <option v-for="(voice, i) in voices" :key="i" bg="$vp-c-bg" :value="voice">
            {{ `${voice.name} (${voice.lang})` }}
          </option>
        </select>

        <n-button @click="speak">复读</n-button>
      </div>
    </div> -->
  <ChannelCreate v-model:show="showModal" :parentId="parentId" />
  <ChannelSettings :channel="channelToSettings" v-model:show="showModal2" />
  <ChannelSortModal v-model:show="showSortModal" />

</template>

<style lang="scss">
.sider-item {
  border-radius: 0.5rem;
  padding: 0.5rem;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: var(--sc-text-primary);
  transition: background-color 0.2s ease, color 0.2s ease;
}

.sider-item:hover {
  background-color: var(--sc-sidebar-hover);
}

.sider-item.active {
  background-color: var(--sc-sidebar-active);
}

.sider-item > .right-num {
  display: flex;
  align-items: center;
}

.sider-item > .right {
  display: none;
}

.sider-item:hover > .right {
  display: flex;
}

.sider-item:hover > .right-num {
  display: none;
}

.channel-collapse-trigger {
  width: 22px;
  height: 22px;
  line-height: 1;
}

.sidebar-footer-actions {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0 0.5rem 0.75rem;
}

.sidebar-footer-actions .n-button {
  justify-content: center;
}
</style>
