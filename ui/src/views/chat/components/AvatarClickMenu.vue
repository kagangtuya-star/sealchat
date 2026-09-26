<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import type { SatoriMessage } from '@/types';
import { useChatStore, chatEvent } from '@/stores/chat';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { NIcon, useMessage } from 'naive-ui';
import { Edit, Id, Message2, MessageCircle2, Minus, Plus, UserPlus } from '@vicons/tabler';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';
import { useDisplayStore } from '@/stores/display';
import { useAvatarCharacterStateStore, type AvatarStatMutationOperation } from '@/stores/avatarCharacterState';
import { resolveCharacterStat, type ResolvedCharacterStat } from '@/utils/characterStatDisplay';
import { resolveCharacterStatMutationTarget } from '@/utils/characterStatMutation';
import { resolveTemplateValue } from '@/utils/characterCardTemplate';
import type { TheaterCharacterStatTemplate } from '@/stores/channelCharacterSnapshot';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

const chat = useChatStore()
const message = useMessage()
const { t } = useI18n();
const user = useUserStore()
const display = useDisplayStore()
const avatarState = useAvatarCharacterStateStore()
interface CharacterCardOpenPayload {
  item: SatoriMessage | null;
  clientX: number;
  clientY: number;
}

const emit = defineEmits<{
  (event: 'open-character-card', payload: CharacterCardOpenPayload): void;
}>();

const avatarMenuClass = computed(() => (display.palette === 'night' ? 'avatar-menu--night' : 'avatar-menu--day'))
const avatarMenuTheme = computed(() => (display.palette === 'night' ? 'default dark' : 'default'))
const avatarMenuOptions = computed<MenuOptions>(() => ({
  ...chat.avatarMenu.optionsComponent,
  theme: avatarMenuTheme.value,
  customClass: avatarMenuClass.value,
  adjustPosition: { xDirection: 'right', yDirection: 'top' },
}))

const isSelf = computed(() => {
  const data = chat.avatarMenu.item;
  return !!data?.user?.id && data.user.id === user.info.id;
});

const showUserActions = computed(() => !!chat.avatarMenu.item?.user?.id && !isSelf.value);

const clickTalkTo = async () => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    if (isSelf.value) return;
    chat.avatarMenu.show = false;
    const ch = await chat.channelPrivateCreate(data.user.id);
    if (ch?.channel?.id) {
      chat.sidebarTab = 'privateChats';
      await chat.ChannelPrivateList()
      nextTick(async () => {
        await chat.channelSwitchTo(ch.channel.id);
      })
    }
  }
}

const clickWhisper = () => {
  const data = chat.avatarMenu.item as any;
  if (!data?.user?.id) {
    message.warning(t('whisper.userUnknown'));
    return;
  }
  if (isSelf.value) {
    message.warning(t('whisper.selfNotAllowed'));
    return;
  }
  const targetUser: User = {
    id: data.user.id,
    name: data.user.name || data.user.username || '',
    nick: data.member?.nick || data.user.nick || data.user.name || '未知成员',
    avatar: data.member?.avatar || data.user.avatar || '',
    discriminator: data.user.discriminator || '',
    is_bot: !!data.user.is_bot,
  };
  chat.addWhisperTarget(targetUser);
  chat.confirmWhisperTargets();
  chat.avatarMenu.show = false;
};

const clickFriendAdd = async () => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    if (isSelf.value) {
      message.warning('不能添加自己为好友');
      return;
    }
    chat.avatarMenu.show = false;
    try {
      const ret = await chat.friendRequestCreate(user.info.id, data.user.id, '');
      if (ret.status === 0) {
        message.success('好友请求已发送');
      } else {
        message.error('已经是好友，或者正在申请列表中');
      }
    } catch (error) {
      console.error('添加好友失败:', error);
      message.error('添加好友失败，可能正在请求或者已经是好友');
    }
  }
}


const showFriendAdd = computed(() => {
  const data = chat.avatarMenu.item;
  if (data && data.user) {
    // 不显示加好友选项的情况:
    // 1. 点击的是自己的头像
    // 2. 点击的用户已经是好友
    if (!data.user?.id) return false;

    if (isSelf.value) {
      return false;
    }

    let ret = true;
    // 如果已经是好友，返回false
    chat.channelTreePrivate.map(channel => {
      if (channel.friendInfo?.userInfo?.id === data.user?.id) {
        if (channel.friendInfo?.isFriend) ret = false;
      }
    })

    return ret;
  }
  return false;
});

const nick = computed(() => {
  const item = chat.avatarMenu.item;
  const roleName = String(item?.identity?.displayName || item?.sender_identity_name || item?.sender_member_name || item?.member?.nick || item?.user?.name || '未知').trim();
  const username = String(item?.user?.username || '').trim();
  return username && username !== roleName ? `${roleName}（${username}）` : roleName;
});

const clickedChannelId = computed(() => String(chat.avatarMenu.item?.channel?.id || chat.curChannel?.id || '').trim());
const clickedIdentityId = computed(() => {
  const item = chat.avatarMenu.item;
  return String(item?.senderIdentityId || item?.sender_identity_id || item?.senderRoleId || item?.sender_role_id || item?.identity?.id || '').trim();
});
const clickedSharedIdentityId = computed(() => {
  const item = chat.avatarMenu.item;
  return String(item?.senderSharedIdentityId || item?.sender_shared_identity_id || item?.identity?.sharedIdentityId || '').trim();
});
const cardData = computed(() => avatarState.resolveAvatarCardData(clickedChannelId.value, clickedIdentityId.value, clickedSharedIdentityId.value));
const clickedWorldOwnerUserId = computed(() => {
  const fromState = String(cardData.value.ownerUserId || '').trim();
  if (fromState) return fromState;

  // 仅作为旧缓存/旧 payload 的兼容 fallback；最新协议优先使用 world state.userId。
  return String(chat.avatarMenu.item?.user?.id || '').trim();
});
const offlineStatusKey = ref('');
const onChannelPresence = (event: any) => {
  if (!chat.avatarMenu.show || cardData.value.source !== 'bot' || event?.channel?.id !== clickedChannelId.value) return;
  const targetUserId = String(chat.avatarMenu.item?.user?.id || '').trim();
  const key = `${clickedChannelId.value}:${clickedIdentityId.value}:${cardData.value.sourceCardId}`;
  if (!targetUserId || !Array.isArray(event?.presence)) return;
  const online = event.presence.some((entry: any) => String(entry?.user?.id || '') === targetUserId);
  if (!online) {
    if (offlineStatusKey.value !== key) {
      offlineStatusKey.value = key;
      avatarState.invalidateBotMutationStatus(clickedChannelId.value);
    }
  } else if (offlineStatusKey.value === key) {
    offlineStatusKey.value = '';
    void avatarState.refreshBotMutationStatus(clickedChannelId.value, clickedIdentityId.value, cardData.value.sourceCardId);
  }
};
onMounted(() => chatEvent.on('channel-presence-updated' as any, onChannelPresence));
onBeforeUnmount(() => chatEvent.off('channel-presence-updated' as any, onChannelPresence));
const isWritableWorldTemplatePath = (raw: unknown) => {
  if (typeof raw !== 'string') return false;
  const path = raw.trim();
  if (!/^[\p{L}\p{N}_-]{1,128}$/u.test(path)) return false;
  return !Number.isFinite(Number(path));
};
const resolveCachedChannelUserRank = (channelId: string, userId: string): number | null => {
  if (!channelId || !userId) return null;
  if (chat.getChannelOwnerId(channelId) === userId) return 4;
  const roleMap = chat.channelMemberRoleMap[channelId];
  if (!roleMap) return null;
  const roles = roleMap[userId];
  if (!roles) return null;
  return roles.reduce((rank, roleId) => {
    if (roleId.endsWith('-owner')) return Math.max(rank, 4);
    if (roleId.endsWith('-admin')) return Math.max(rank, 3);
    if (roleId.endsWith('-member')) return Math.max(rank, 2);
    if (roleId.endsWith('-spectator')) return Math.max(rank, 1);
    return rank;
  }, 0);
};
const canEditWorld = computed(() => {
  const userId = String(user.info.id || '').trim();
  if (cardData.value.source !== 'world' || cardData.value.ready !== true || cardData.value.attrs === null
    || !clickedIdentityId.value || !clickedChannelId.value
    || chat.isObserver || !userId) return false;
  const clickedUserId = clickedWorldOwnerUserId.value;
  if (!clickedUserId) return false;
  if (clickedUserId === userId) return true;
  const worldId = chat.currentWorldId;
  const detail = worldId ? chat.worldDetailMap[worldId] : undefined;
  const delegationEnabled = detail?.allowManageOtherUserChannelIdentities === true
    || detail?.world?.allowManageOtherUserChannelIdentities === true;
  if (delegationEnabled !== true) return false;
  const operatorRank = resolveCachedChannelUserRank(clickedChannelId.value, userId);
  const targetRank = resolveCachedChannelUserRank(clickedChannelId.value, clickedUserId);
  if (operatorRank !== null && targetRank !== null) {
    return operatorRank > 0 && targetRank > 0 && operatorRank >= targetRank;
  }
  return Boolean(detail?.memberRole);
});
watch(() => [chat.avatarMenu.show, clickedChannelId.value, clickedIdentityId.value,
  cardData.value.source, cardData.value.sourceCardId, avatarState.getSettings(clickedChannelId.value).serverRevision] as const,
		([open, channelId, identityId, source, sourceCardId], previous) => {
			  const previousOpen = previous?.[0] === true;
			  const previousChannelId = String(previous?.[1] || '').trim();
			  if (previousOpen && !open) {
			    avatarState.stopBotMutationStatusChecks(channelId || previousChannelId || undefined);
			    return;
			  }
		  if (previous && (channelId !== previous[1] || identityId !== previous[2] || source !== previous[3] || sourceCardId !== previous[4])) {
		    avatarState.invalidateBotMutationStatus(previousChannelId || channelId || undefined);
		    if (channelId && channelId !== previousChannelId) avatarState.invalidateBotMutationStatus(channelId);
		  }
	  if (open && source === 'bot' && channelId && identityId && sourceCardId) {
    void avatarState.refreshBotMutationStatus(channelId, identityId, sourceCardId);
  }
}, { immediate: true });
watch(() => avatarState.mutationError, value => {
  if (!value) return;
  message.error(value);
  avatarState.mutationError = '';
});

const sourcePath = (item: TheaterCharacterStatTemplate, slot: 'current' | 'max') => {
  const source = item[slot];
  return source && 'path' in source ? source.path.trim() : '';
};
const operationContext = (item: TheaterCharacterStatTemplate, slot: 'current' | 'max') => ({
  channelId: clickedChannelId.value, identityId: clickedIdentityId.value,
  source: cardData.value.source, sourceCardId: cardData.value.sourceCardId,
  statId: item.id, slot, sourcePath: sourcePath(item, slot), op: 'set' as const, value: 0,
});
const canEditSlot = (item: TheaterCharacterStatTemplate, slot: 'current' | 'max') => {
  const data = cardData.value;
  const path = sourcePath(item, slot);
  if (!data.attrs || !path || !chat.avatarMenu.show || clickedChannelId.value !== chat.curChannel?.id) return false;
  if (data.source === 'world') return canEditWorld.value && isWritableWorldTemplatePath(path)
    && (Object.prototype.hasOwnProperty.call(data.attrs, path) || resolveTemplateValue(data.attrs, path) == null);
  if (!avatarState.getBotMutationStatus(clickedChannelId.value, clickedIdentityId.value, data.sourceCardId)?.editable) return false;
  return !!resolveCharacterStatMutationTarget(path, data.attrs, { allowRootInit: true, directOnly: slot === 'max' });
};
interface StatRow { stat: ResolvedCharacterStat; item: TheaterCharacterStatTemplate; editCurrent: boolean; editMax: boolean }
const statRows = computed<StatRow[]>(() => {
  const data = cardData.value;
  if (!clickedIdentityId.value || !data.attrs) return [];
  return data.template.items.flatMap(item => {
    const currentContext = operationContext(item, 'current');
    const maxContext = operationContext(item, 'max');
    const base = resolveCharacterStat(item, data.attrs!, true);
    if (!base) return [];
    const current = avatarState.getOptimisticStatValue(currentContext, base.current);
    const max = avatarState.getOptimisticStatValue(maxContext, base.max);
    const rendered = resolveCharacterStat({ ...item,
      ...(current !== null ? { current: { value: current } } : {}),
      ...(max !== null ? { max: { value: max } } : {}),
    }, data.attrs!, true) || base;
    return [{ stat: rendered, item, editCurrent: canEditSlot(item, 'current'), editMax: canEditSlot(item, 'max') }];
  });
});

const editing = ref<{ key: string; value: string | number } | null>(null);
watch(() => [clickedChannelId.value, clickedIdentityId.value, cardData.value.source, cardData.value.sourceCardId],
  () => { editing.value = null; });
const editKey = (statId: string, slot: 'current' | 'max') => `${statId}:${slot}`;
const startEdit = (row: StatRow, slot: 'current' | 'max') => {
  if (slot === 'current' ? !row.editCurrent : !row.editMax) return;
  const value = row.stat[slot];
  editing.value = { key: editKey(row.stat.id, slot), value: value === null ? '' : String(value) };
  nextTick(() => (document.querySelector('.avatar-menu-stat__input') as HTMLInputElement | null)?.focus());
};
const cancelEdit = () => { editing.value = null; };
const submitEdit = (row: StatRow, slot: 'current' | 'max') => {
  const key = editKey(row.stat.id, slot);
  if (editing.value?.key !== key) return;
  const input = String(editing.value.value ?? '').trim();
  editing.value = null;
  if (!input) return;
  const value = Number(input);
  if (!Number.isFinite(value)) { message.error('请输入有效数值'); return; }
  avatarState.queueStatMutation({ ...operationContext(row.item, slot), value });
};
const changeCurrent = (row: StatRow, delta: number) => {
  if (!row.editCurrent) return;
  avatarState.queueStatMutation({ ...operationContext(row.item, 'current'), op: 'add', value: delta } as AvatarStatMutationOperation);
};
const statTextColor = (stat: ResolvedCharacterStat) => {
  const color = String(stat.textColor || '').trim();
  if (!/^#[0-9a-fA-F]+$/.test(color) || ![4, 7, 9].includes(color.length)) return undefined;
  if (display.palette === 'night') return color;
  const hex = color.length === 4 ? color.slice(1).split('').map(part => part + part).join('') : color.slice(1, 7);
  const red = parseInt(hex.slice(0, 2), 16);
  const green = parseInt(hex.slice(2, 4), 16);
  const blue = parseInt(hex.slice(4, 6), 16);
  return (red * 0.2126 + green * 0.7152 + blue * 0.0722) / 255 < 0.6 ? color : undefined;
};

const showIdentitySettings = computed(() => {
  const data = chat.avatarMenu.item;
  if (!data?.user?.id) {
    return false;
  }
  return isSelf.value;
});

const openIdentitySettings = () => {
  chat.avatarMenu.show = false;
  chatEvent.emit('channel-identity-open');
};

const clickCharacterCard = (event: MouseEvent) => {
  const item = chat.avatarMenu.item;
  chat.avatarMenu.show = false;
  emit('open-character-card', {
    item,
    clientX: event.clientX,
    clientY: event.clientY,
  });
};

</script>

<template>
  <context-menu v-model:show="chat.avatarMenu.show" :options="avatarMenuOptions">
    <div class="avatar-menu-card" :class="display.palette === 'night' ? 'avatar-menu-card--night' : 'avatar-menu-card--day'">
      <div class="avatar-menu-card__title" :title="nick">{{ nick }}</div>
      <div v-if="clickedIdentityId" class="avatar-menu-card__stats">
        <div v-if="cardData.source === 'world' && !cardData.ready" class="avatar-menu-card__empty">数据加载中</div>
        <div v-else-if="!cardData.attrs" class="avatar-menu-card__empty">数据不可用</div>
        <div v-else-if="!statRows.length" class="avatar-menu-card__empty">暂无状态项</div>
        <div v-for="row in statRows" :key="row.stat.id" class="avatar-menu-stat" :style="{ color: statTextColor(row.stat) }">
          <div class="avatar-menu-stat__line">
            <span class="avatar-menu-stat__name">{{ row.stat.name }}</span>
          </div>
          <div class="avatar-menu-stat__body" @pointerdown.stop @pointerup.stop @mousedown.stop @mouseup.stop @click.stop>
            <div v-if="row.stat.displayMode === 'bar'" class="avatar-menu-stat__bar">
              <span class="avatar-menu-stat__fill" :style="{ left: `${row.stat.fillLeft}%`, width: `${row.stat.fillWidth}%`, backgroundColor: row.stat.barColor || undefined }" />
              <span v-if="row.stat.min !== null && row.stat.min < 0 && row.stat.max !== null && row.stat.max > 0" class="avatar-menu-stat__zero" :style="{ left: `${row.stat.zeroLeft}%` }" />
            </div>
            <div v-else-if="row.stat.current !== null" class="avatar-menu-stat__icons">
              <span v-for="(fill, index) in row.stat.iconFills" :key="index" class="avatar-menu-stat__icon">
              <span class="avatar-menu-stat__icon-base">
                <img v-if="row.stat.iconType === 'image'" :src="resolveAttachmentUrl(row.stat.iconValue)" alt="">
                <span v-else>{{ row.stat.iconValue }}</span>
              </span>
              <span class="avatar-menu-stat__icon-fill" :style="{ width: `${fill * 100}%` }">
                <img v-if="row.stat.iconType === 'image'" :src="resolveAttachmentUrl(row.stat.iconValue)" alt="">
                <span v-else>{{ row.stat.iconValue }}</span>
              </span>
              </span>
            </div>
            <div class="avatar-menu-stat__controls" :style="{ color: statTextColor(row.stat) }">
              <button v-if="row.editCurrent && row.stat.current !== null" type="button" class="avatar-menu-stat__step" aria-label="减少当前值" @click="changeCurrent(row, -1)"><NIcon :size="16"><Minus /></NIcon></button>
              <input v-if="editing?.key === editKey(row.stat.id, 'current')" v-model="editing.value" class="avatar-menu-stat__input" type="number" step="any" aria-label="当前值" @keydown.enter.stop.prevent="submitEdit(row, 'current')" @keydown.esc.stop.prevent="cancelEdit" @blur="submitEdit(row, 'current')">
              <button v-else-if="row.editCurrent" type="button" class="avatar-menu-stat__number" @click="startEdit(row, 'current')">{{ row.stat.current ?? '—' }}</button>
              <span v-else class="avatar-menu-stat__number">{{ row.stat.current ?? '—' }}</span>
              <span v-if="row.stat.max !== null || row.editMax" class="avatar-menu-stat__separator">/</span>
              <input v-if="editing?.key === editKey(row.stat.id, 'max')" v-model="editing.value" class="avatar-menu-stat__input" type="number" step="any" aria-label="最大值" @keydown.enter.stop.prevent="submitEdit(row, 'max')" @keydown.esc.stop.prevent="cancelEdit" @blur="submitEdit(row, 'max')">
              <button v-else-if="row.editMax" type="button" class="avatar-menu-stat__number" @click="startEdit(row, 'max')">{{ row.stat.max ?? '—' }}</button>
              <span v-else-if="row.stat.max !== null" class="avatar-menu-stat__number">{{ row.stat.max }}</span>
              <button v-if="row.editCurrent && row.stat.current !== null" type="button" class="avatar-menu-stat__step" aria-label="增加当前值" @click="changeCurrent(row, 1)"><NIcon :size="16"><Plus /></NIcon></button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <context-menu-sperator />
    <div class="avatar-menu-actions">
      <button v-if="showUserActions" type="button" class="avatar-menu-action" @click="clickWhisper">
        <NIcon :size="15" class="avatar-menu-action__icon"><MessageCircle2 /></NIcon>
        <span>{{ t('inputBox.whisperButton') }}</span>
      </button>
      <button v-if="showUserActions" type="button" class="avatar-menu-action" @click="clickTalkTo">
        <NIcon :size="15" class="avatar-menu-action__icon"><Message2 /></NIcon>
        <span>私聊</span>
      </button>
      <button v-if="showFriendAdd" type="button" class="avatar-menu-action" @click="clickFriendAdd">
        <NIcon :size="15" class="avatar-menu-action__icon"><UserPlus /></NIcon>
        <span>添加好友</span>
      </button>
      <button type="button" class="avatar-menu-action" @click="clickCharacterCard">
        <NIcon :size="15" class="avatar-menu-action__icon"><Id /></NIcon>
        <span>人物卡</span>
      </button>
      <button v-if="showIdentitySettings" type="button" class="avatar-menu-action" @click="openIdentitySettings">
        <NIcon :size="15" class="avatar-menu-action__icon"><Edit /></NIcon>
        <span>修改角色</span>
      </button>
    </div>
  </context-menu>
</template>

<style scoped>
.avatar-menu-card { width: 18rem; max-width: min(18rem, calc(100vw - 1rem)); padding: 0.45rem 0.7rem 0.15rem; --avatar-stat-text: #172033; --avatar-stat-muted: #667386; --avatar-stat-track: rgba(15, 23, 42, .11); --avatar-stat-fill: #608bb4; }
.avatar-menu-card--night { --avatar-stat-text: #e2e8f0; --avatar-stat-muted: #a2aec0; --avatar-stat-track: rgba(255, 255, 255, .14); --avatar-stat-fill: #729bc2; }
.avatar-menu-card__title { overflow: hidden; color: var(--avatar-stat-muted); font-size: 11px; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }
.avatar-menu-card__stats { display: grid; grid-template-columns: minmax(0, 1fr); gap: 5px; max-height: min(16rem, 40vh); margin-top: 5px; overflow-y: auto; }
.avatar-menu-card__empty { color: var(--avatar-stat-muted); font-size: 11px; }
.avatar-menu-stat { min-width: 0; color: var(--avatar-stat-text); }
.avatar-menu-stat__line { display: flex; justify-content: space-between; gap: 0.5rem; min-width: 0; font-size: 11px; font-variant-numeric: tabular-nums; line-height: 14px; }
.avatar-menu-stat__name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.avatar-menu-stat__body { display: flex; align-items: center; gap: 5px; min-height: 18px; }
.avatar-menu-stat__bar { position: relative; flex: 1; min-width: 1.5rem; height: 13px; overflow: hidden; border-radius: 3px; background: var(--avatar-stat-track); }
.avatar-menu-stat__fill { position: absolute; top: 0; bottom: 0; background: var(--avatar-stat-fill); opacity: .78; }
.avatar-menu-stat__zero { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--avatar-stat-text); opacity: .7; }
.avatar-menu-stat__icons { display: flex; flex: 1; flex-wrap: wrap; gap: 2px; min-width: 1.5rem; max-height: 32px; overflow: hidden; }
.avatar-menu-stat__icon { position: relative; display: inline-block; width: 15px; height: 15px; overflow: hidden; font-size: 14px; line-height: 15px; }
.avatar-menu-stat__icon-base, .avatar-menu-stat__icon-fill { position: absolute; inset: 0; display: block; overflow: hidden; white-space: nowrap; }
.avatar-menu-stat__icon-base { filter: grayscale(1); opacity: .28; }
.avatar-menu-stat__icon-fill { right: auto; }
.avatar-menu-stat__icon img, .avatar-menu-stat__icon-base > span, .avatar-menu-stat__icon-fill > span { display: block; width: 15px; height: 15px; object-fit: contain; }
.avatar-menu-stat__controls { display: inline-flex; flex: none; align-items: center; gap: 1px; margin-left: auto; color: var(--avatar-stat-muted); font-size: 11px; font-variant-numeric: tabular-nums; }
.avatar-menu-stat__step, .avatar-menu-stat__number { display: inline-flex; align-items: center; justify-content: center; min-width: 17px; height: 18px; padding: 0 1px; border: 0; border-radius: 3px; background: transparent; color: inherit; font: inherit; }
button.avatar-menu-stat__step, button.avatar-menu-stat__number { cursor: pointer; }
button.avatar-menu-stat__step:hover, button.avatar-menu-stat__number:hover { background: var(--avatar-stat-track); color: var(--avatar-stat-text); }
.avatar-menu-stat__separator { padding: 0 1px; }
.avatar-menu-stat__input { width: 3.1rem; height: 18px; padding: 0 2px; border: 1px solid var(--avatar-stat-muted); border-radius: 3px; background: transparent; color: var(--avatar-stat-text); font: inherit; text-align: center; appearance: textfield; }
.avatar-menu-stat__input::-webkit-inner-spin-button, .avatar-menu-stat__input::-webkit-outer-spin-button { margin: 0; appearance: none; }
:deep(.context-menu.avatar-menu--night),
:deep(.mx-context-menu.avatar-menu--night) {
  background: rgba(15, 23, 42, 0.95);
  border-color: rgba(148, 163, 184, 0.35);
  color: #e2e8f0;
}

:deep(.context-menu.avatar-menu--night .context-menu-item),
:deep(.mx-context-menu.avatar-menu--night .mx-context-menu-item) {
  color: inherit;
}

:deep(.context-menu.avatar-menu--night .context-menu-item:hover),
:deep(.mx-context-menu.avatar-menu--night .mx-context-menu-item:hover) {
  background: rgba(255, 255, 255, 0.08);
}

:deep(.context-menu.avatar-menu--day),
:deep(.mx-context-menu.avatar-menu--day) {
  background: rgba(248, 250, 252, 0.98);
  border-color: rgba(15, 23, 42, 0.08);
  color: #0f172a;
}

:deep(.context-menu.avatar-menu--day .context-menu-item),
:deep(.mx-context-menu.avatar-menu--day .mx-context-menu-item) {
  color: inherit;
}

:deep(.context-menu.avatar-menu--day .context-menu-item:hover),
:deep(.mx-context-menu.avatar-menu--day .mx-context-menu-item:hover) {
  background: rgba(15, 23, 42, 0.06);
}

.avatar-menu-actions {
  display: flex;
  gap: 0.2rem;
  min-width: 18rem;
  padding: 0.2rem 0.65rem 0.25rem;
}

.avatar-menu-action {
  display: inline-flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
  align-items: center;
  justify-content: center;
  gap: 0.08rem;
  padding: 0.22rem 0.3rem;
  border: 0;
  border-radius: 0.35rem;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
  line-height: 1.1;
  white-space: nowrap;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.avatar-menu-action__icon {
  flex-shrink: 0;
}

:deep(.context-menu.avatar-menu--night .avatar-menu-action:hover),
:deep(.mx-context-menu.avatar-menu--night .avatar-menu-action:hover) {
  background: rgba(255, 255, 255, 0.08);
}

:deep(.context-menu.avatar-menu--night .avatar-menu-action:active),
:deep(.mx-context-menu.avatar-menu--night .avatar-menu-action:active) {
  background: rgba(255, 255, 255, 0.14);
}

:deep(.context-menu.avatar-menu--day .avatar-menu-action:hover),
:deep(.mx-context-menu.avatar-menu--day .avatar-menu-action:hover) {
  background: rgba(15, 23, 42, 0.06);
}

:deep(.context-menu.avatar-menu--day .avatar-menu-action:active),
:deep(.mx-context-menu.avatar-menu--day .avatar-menu-action:active) {
  background: rgba(15, 23, 42, 0.1);
}
</style>
