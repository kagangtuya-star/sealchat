<script setup lang="tsx">
import type { MenuOptions } from '@imengyu/vue3-context-menu';
import type { User } from '@satorijs/protocol';
import type { SatoriMessage } from '@/types';
import { useChatStore, chatEvent } from '@/stores/chat';
import { computed, nextTick, ref } from 'vue';
import { NIcon, useMessage } from 'naive-ui';
import { Edit, Id, Message2, MessageCircle2, UserPlus } from '@vicons/tabler';
import { useUserStore } from '@/stores/user';
import { useI18n } from 'vue-i18n';
import { useDisplayStore } from '@/stores/display';
import { useAvatarCharacterStateStore } from '@/stores/avatarCharacterState';
import { resolveCharacterStat, type ResolvedCharacterStat } from '@/utils/characterStatDisplay';
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
interface WorldDataField { path: string; label: string; original: number | null }
const isWritableWorldTemplatePath = (raw: unknown) => {
  if (typeof raw !== 'string') return false;
  const path = raw.trim();
  if (!/^[\p{L}\p{N}_-]{1,128}$/u.test(path)) return false;
  return !Number.isFinite(Number(path));
};
const writableWorldFields = computed<WorldDataField[]>(() => {
  const data = cardData.value;
  if (data.source !== 'world' || data.ready !== true || data.attrs === null || !clickedIdentityId.value) return [];
  const fields: WorldDataField[] = [];
  const seen = new Set<string>();
  for (const item of cardData.value.template.items) {
    for (const [key, suffix] of [['current', '当前值'], ['max', '最大值'], ['min', '最小值']] as const) {
      const source = item[key];
      if (!source || !('path' in source) || !isWritableWorldTemplatePath(source.path)) continue;
      const path = source.path.trim();
      if (seen.has(path)) continue;
      seen.add(path);
      const current = data.attrs[path];
      fields.push({ path, label: `${item.name} ${suffix}`, original: typeof current === 'number' && Number.isFinite(current) ? current : null });
    }
  }
  return fields;
});
const canSetWorldData = computed(() => {
  const item = chat.avatarMenu.item;
  const clickedUserId = String(item?.user?.id || item?.user_id || item?.userId || '').trim();
  const userId = String(user.info.id || '').trim();
  return cardData.value.source === 'world' && cardData.value.ready === true && cardData.value.attrs !== null
    && !!clickedIdentityId.value && !!clickedChannelId.value && writableWorldFields.value.length > 0
    && !chat.isObserver && !!userId && (clickedUserId === userId
      || chat.isChannelAdmin(clickedChannelId.value, userId) || chat.isChannelOwner(clickedChannelId.value, userId));
});
const worldDataDialog = ref<{ channelId: string; identityId: string; fields: WorldDataField[] } | null>(null);
const worldDataValues = ref<Record<string, number | null>>({});
const worldDataSaving = ref(false);
const openWorldDataDialog = () => {
  if (!canSetWorldData.value) return;
  worldDataDialog.value = { channelId: clickedChannelId.value, identityId: clickedIdentityId.value, fields: writableWorldFields.value.map(field => ({ ...field })) };
  worldDataValues.value = Object.fromEntries(worldDataDialog.value.fields.map(field => [field.path, field.original]));
  chat.avatarMenu.show = false;
};
const closeWorldDataDialog = (show: boolean) => {
  if (!show && !worldDataSaving.value) worldDataDialog.value = null;
};
const saveWorldData = async () => {
  const target = worldDataDialog.value;
  if (!target || worldDataSaving.value) return;
  if (chat.curChannel?.id !== target.channelId) {
    message.error('频道已切换，请重新打开填写窗口');
    return;
  }
  if (avatarState.getEffectiveSource(target.channelId) !== 'world') {
    message.error('数据来源已变化，请重新打开填写窗口');
    return;
  }
  if (!avatarState.isWorldStateReady(target.channelId)) {
    message.warning('数据正在重新加载，请重新打开填写窗口');
    return;
  }
  if (target.fields.some(field => {
    const value = worldDataValues.value[field.path];
    return value != null && (typeof value !== 'number' || !Number.isFinite(value));
  })) {
    message.error('请输入有效数值');
    return;
  }
  const values = target.fields.map(field => ({ field, value: worldDataValues.value[field.path] }))
    .filter((entry): entry is { field: WorldDataField; value: number } => typeof entry.value === 'number' && Number.isFinite(entry.value));
  if (!values.length) {
    message.warning('请至少填写一个数值');
    return;
  }
  worldDataSaving.value = true;
  try {
    for (const { field, value } of values) {
      if (value === field.original) continue;
      await avatarState.applyStatOperation(target.channelId, { identityId: target.identityId, path: field.path, op: 'set', value });
      field.original = value;
    }
    worldDataDialog.value = null;
    message.success('角色数据已保存');
  } catch (error: any) {
    message.error(error?.response?.err || error?.message || '角色数据保存失败');
  } finally {
    worldDataSaving.value = false;
  }
};
const stats = computed<ResolvedCharacterStat[]>(() => {
  if (!clickedIdentityId.value || !cardData.value.attrs) return [];
  return cardData.value.template.items
    .map(item => resolveCharacterStat(item, cardData.value.attrs!, true))
    .filter((item): item is ResolvedCharacterStat => !!item);
});
const statValue = (stat: ResolvedCharacterStat) => stat.current === null ? '—' : stat.max === null ? String(stat.current) : `${stat.current}/${stat.max}`;
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
        <button v-if="canSetWorldData" type="button" class="avatar-menu-card__set-data" @click="openWorldDataDialog">设置数据</button>
        <div v-if="cardData.source === 'world' && !cardData.ready" class="avatar-menu-card__empty">数据加载中</div>
        <div v-else-if="!cardData.attrs" class="avatar-menu-card__empty">数据不可用</div>
        <div v-else-if="!stats.length" class="avatar-menu-card__empty">暂无状态项</div>
        <div v-for="stat in stats" :key="stat.id" class="avatar-menu-stat" :style="{ color: statTextColor(stat) }">
          <div class="avatar-menu-stat__line">
            <span class="avatar-menu-stat__name">{{ stat.name }}</span>
            <span class="avatar-menu-stat__value" :style="{ color: statTextColor(stat) }">{{ statValue(stat) }}</span>
          </div>
          <div v-if="stat.displayMode === 'bar' && stat.current !== null" class="avatar-menu-stat__bar">
            <span class="avatar-menu-stat__fill" :style="{ left: `${stat.fillLeft}%`, width: `${stat.fillWidth}%`, backgroundColor: stat.barColor || undefined }" />
            <span v-if="stat.min !== null && stat.min < 0 && stat.max !== null && stat.max > 0" class="avatar-menu-stat__zero" :style="{ left: `${stat.zeroLeft}%` }" />
          </div>
          <div v-else-if="stat.displayMode === 'icon' && stat.current !== null" class="avatar-menu-stat__icons">
            <span v-for="(fill, index) in stat.iconFills" :key="index" class="avatar-menu-stat__icon">
              <span class="avatar-menu-stat__icon-base">
                <img v-if="stat.iconType === 'image'" :src="resolveAttachmentUrl(stat.iconValue)" alt="">
                <span v-else>{{ stat.iconValue }}</span>
              </span>
              <span class="avatar-menu-stat__icon-fill" :style="{ width: `${fill * 100}%` }">
                <img v-if="stat.iconType === 'image'" :src="resolveAttachmentUrl(stat.iconValue)" alt="">
                <span v-else>{{ stat.iconValue }}</span>
              </span>
            </span>
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
  <n-modal :show="!!worldDataDialog" :mask-closable="!worldDataSaving" @update:show="closeWorldDataDialog">
    <n-card title="设置角色数据" style="width: min(380px, 92vw);" :bordered="false" role="dialog" aria-modal="true">
      <div class="world-data-fields">
        <label v-for="field in worldDataDialog?.fields || []" :key="field.path" class="world-data-field">
          <span>{{ field.label }}</span>
          <n-input-number v-model:value="worldDataValues[field.path]" size="small" :show-button="false" :disabled="worldDataSaving" placeholder="未填写" />
        </label>
      </div>
      <template #footer>
        <div class="world-data-actions">
          <n-button size="small" :disabled="worldDataSaving" @click="worldDataDialog = null">取消</n-button>
          <n-button size="small" type="primary" :loading="worldDataSaving" @click="saveWorldData">保存</n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.avatar-menu-card { width: 18rem; max-width: min(18rem, calc(100vw - 1rem)); padding: 0.45rem 0.7rem 0.15rem; --avatar-stat-text: #172033; --avatar-stat-muted: #667386; --avatar-stat-track: rgba(15, 23, 42, .11); --avatar-stat-fill: #608bb4; }
.avatar-menu-card--night { --avatar-stat-text: #e2e8f0; --avatar-stat-muted: #a2aec0; --avatar-stat-track: rgba(255, 255, 255, .14); --avatar-stat-fill: #729bc2; }
.avatar-menu-card__title { overflow: hidden; color: var(--avatar-stat-muted); font-size: 11px; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }
.avatar-menu-card__stats { display: grid; grid-template-columns: minmax(0, 1fr); gap: 5px; max-height: min(16rem, 40vh); margin-top: 5px; overflow-y: auto; }
.avatar-menu-card__empty { color: var(--avatar-stat-muted); font-size: 11px; }
.avatar-menu-card__set-data { justify-self: end; padding: 0; border: 0; background: none; color: var(--avatar-stat-muted); cursor: pointer; font-size: 11px; }
.avatar-menu-card__set-data:hover { color: var(--avatar-stat-text); }
.world-data-fields { display: grid; gap: 10px; max-height: 50vh; overflow-y: auto; }
.world-data-field { display: grid; gap: 4px; font-size: 12px; }
.world-data-actions { display: flex; justify-content: flex-end; gap: 8px; }
.avatar-menu-stat { min-width: 0; color: var(--avatar-stat-text); }
.avatar-menu-stat__line { display: flex; justify-content: space-between; gap: 0.5rem; min-width: 0; font-size: 11px; font-variant-numeric: tabular-nums; line-height: 14px; }
.avatar-menu-stat__name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.avatar-menu-stat__value { flex: none; color: var(--avatar-stat-muted); }
.avatar-menu-stat__bar { position: relative; height: 16px; margin-top: 2px; overflow: hidden; border-radius: 3px; background: var(--avatar-stat-track); }
.avatar-menu-stat__fill { position: absolute; top: 0; bottom: 0; background: var(--avatar-stat-fill); opacity: .78; }
.avatar-menu-stat__zero { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--avatar-stat-text); opacity: .7; }
.avatar-menu-stat__icons { display: flex; flex-wrap: wrap; gap: 2px; margin-top: 2px; }
.avatar-menu-stat__icon { position: relative; display: inline-block; width: 15px; height: 15px; overflow: hidden; font-size: 14px; line-height: 15px; }
.avatar-menu-stat__icon-base, .avatar-menu-stat__icon-fill { position: absolute; inset: 0; display: block; overflow: hidden; white-space: nowrap; }
.avatar-menu-stat__icon-base { filter: grayscale(1); opacity: .28; }
.avatar-menu-stat__icon-fill { right: auto; }
.avatar-menu-stat__icon img, .avatar-menu-stat__icon-base > span, .avatar-menu-stat__icon-fill > span { display: block; width: 15px; height: 15px; object-fit: contain; }
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
