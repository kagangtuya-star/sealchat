<script setup lang="tsx">
import { computed, cloneVNode, ref, watch } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { useDisplayStore } from '@/stores/display';
import AvatarVue from '@/components/avatar.vue';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import type { DropdownOption, DropdownRenderOption } from 'naive-ui';
import { NDropdown, NButton, NIcon } from 'naive-ui';
import { Plus, Star } from '@vicons/tabler';

const props = withDefaults(defineProps<{
  channelId?: string;
  disabled?: boolean;
}>(), {
  channelId: undefined,
  disabled: false,
});

const emit = defineEmits<{
  (event: 'create'): void;
  (event: 'manage'): void;
}>();

const chat = useChatStore();
const user = useUserStore();
const display = useDisplayStore();

const resolvedChannelId = computed(() => props.channelId || chat.curChannel?.id || '');

const identities = computed(() => {
  const id = resolvedChannelId.value;
  if (!id) {
    return [];
  }
  return chat.channelIdentities[id] || [];
});

const favoriteFolderIds = computed(() => {
  const id = resolvedChannelId.value;
  if (!id) {
    return [] as string[];
  }
  return chat.channelIdentityFavorites[id] || [];
});

const identityMembership = computed<Record<string, string[]>>(() => {
  const id = resolvedChannelId.value;
  if (!id) {
    return {};
  }
  return chat.channelIdentityMembership[id] || {};
});

const filterMode = ref<'all' | 'favorites'>(favoriteFolderIds.value.length ? 'favorites' : 'all');

watch([favoriteFolderIds, resolvedChannelId], () => {
  if (!favoriteFolderIds.value.length) {
    filterMode.value = 'all';
  }
});

const filteredIdentities = computed(() => {
  if (!favoriteFolderIds.value.length || filterMode.value === 'all') {
    return identities.value;
  }
  const favoriteSet = new Set(favoriteFolderIds.value);
  return identities.value.filter(identity => {
    const folders = identity.folderIds && identity.folderIds.length ? identity.folderIds : identityMembership.value[identity.id] || [];
    return folders.some(folderId => favoriteSet.has(folderId));
  });
});

const activeIdentity = computed(() => chat.getActiveIdentity(resolvedChannelId.value));

const fallbackName = computed(() => chat.curMember?.nick || user.info.nick || user.info.username || '默认身份');
const fallbackAvatar = computed(() => user.info.avatar || '');

const buildAttachmentUrl = (token?: string) => resolveAttachmentUrl(token);

const displayName = computed(() => activeIdentity.value?.displayName || fallbackName.value);
const displayColor = computed(() => activeIdentity.value?.color || '');
const avatarSrc = computed(() => {
  return buildAttachmentUrl(activeIdentity.value?.avatarAttachmentId) || fallbackAvatar.value;
});

const options = computed<DropdownOption[]>(() => {
  const list = filteredIdentities.value.map<DropdownOption>((item) => ({
    key: item.id,
    label: item.displayName,
    icon: () => (
      <AvatarVue
        size={24}
        border={false}
        src={buildAttachmentUrl(item.avatarAttachmentId) || fallbackAvatar.value}
      />
    ),
    class: item.id === activeIdentity.value?.id ? 'identity-option identity-option--active' : 'identity-option',
    extra: item.color,
  }));
  if (!list.length) {
    list.push({
      key: '__placeholder',
      label: filterMode.value === 'favorites' ? '收藏文件夹暂无角色' : '暂无频道角色',
      disabled: true,
    });
  }
  const actionLabel = filterMode.value === 'favorites' ? '显示全部角色' : '仅显示收藏角色';
  return [
    ...list,
    { type: 'divider', key: '__divider' },
    {
      key: '__toggle',
      label: actionLabel,
    },
    {
      key: '__create',
      label: '创建新角色',
      icon: () => (
        <NIcon size={18}>
          <Plus />
        </NIcon>
      ),
    },
    {
      key: '__manage',
      label: '管理角色',
    },
  ];
});

const renderOption: DropdownRenderOption = ({ node, option }) => {
  if (option.key === '__divider') {
    return node;
  }
  if (option.key === '__divider') {
    return node;
  }
  if (option.key === '__create' || option.key === '__manage' || option.key === '__toggle' || option.key === '__placeholder') {
    return cloneVNode(node, {
      class: [node.props?.class, 'identity-option-node', 'identity-option-node--action'],
    });
  }
  const color = (option as any).extra as string | undefined;
  const isActive = activeIdentity.value?.id === option.key;
  return cloneVNode(
    node,
    {
      class: [node.props?.class, 'identity-option-node', isActive ? 'identity-option-node--active' : ''],
    },
    {
      default: () => (
        <div class="identity-option">
          {option.icon?.()}
          <span class="identity-option__label">
            {color ? <span class="identity-option__dot" style={{ backgroundColor: color }}></span> : null}
            <span class="identity-option__name" style={color ? { color } : undefined}>{option.label as string}</span>
            {isActive ? <span class="identity-option__tag">当前</span> : null}
          </span>
        </div>
      ),
    },
  );
};

const handleSelect = async (key: string | number) => {
  if (key === '__create') {
    emit('create');
    return;
  }
  if (key === '__manage') {
    emit('manage');
    return;
  }
  if (key === '__toggle') {
    if (favoriteFolderIds.value.length) {
      filterMode.value = filterMode.value === 'favorites' ? 'all' : 'favorites';
    } else {
      filterMode.value = 'all';
    }
    return;
  }
  if (key === '__placeholder') {
    return;
  }
  const channelId = resolvedChannelId.value;
  if (!channelId || props.disabled) {
    return;
  }
  chat.setActiveIdentity(channelId, String(key));
  emit('identity-changed' as any);
};

const showFavoriteBadge = computed(() => filterMode.value === 'favorites' && favoriteFolderIds.value.length > 0);
</script>

<template>
  <n-dropdown
    trigger="click"
    :options="options"
    :show-arrow="false"
    placement="top-start"
    :disabled="!resolvedChannelId || disabled"
    :render-option="renderOption"
    :overlay-class="isNightPalette ? 'identity-dropdown--night' : undefined"
    @select="handleSelect"
  >
    <n-button
      tertiary
      size="small"
      class="identity-switcher"
      :disabled="!resolvedChannelId || disabled"
    >
      <AvatarVue
        :size="28"
        :border="false"
        :src="avatarSrc"
        class="identity-switcher__avatar"
      />
      <span
        v-if="displayColor"
        class="identity-switcher__color"
        :style="{ backgroundColor: displayColor }"
      />
      <span
        class="identity-switcher__label"
        :style="displayColor ? { color: displayColor } : undefined"
      >
        {{ displayName }}
      </span>
      <n-icon v-if="showFavoriteBadge" :component="Star" size="12" class="identity-switcher__favorite" />
    </n-button>
  </n-dropdown>
</template>

<style scoped>
.identity-switcher {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.6rem;
  border-radius: 999px;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  background-color: var(--sc-bg-elevated, rgba(248, 250, 252, 0.9));
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
  color: var(--sc-text-primary, #374151);
  transition: background-color 0.25s ease, color 0.25s ease, border-color 0.25s ease;
}

.identity-switcher__label {
  font-size: 0.8rem;
  font-weight: 600;
  color: inherit;
  max-width: 6.5rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.identity-switcher__avatar {
  border-radius: 9999px;
  overflow: hidden;
}

.identity-switcher__color {
  width: 10px;
  height: 10px;
  border-radius: 9999px;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.45));
}

.identity-switcher__favorite {
  color: #fbbf24;
  margin-left: 0.15rem;
}

.identity-option {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  min-width: 11rem;
}

.identity-option--active .identity-option__name {
  font-weight: 600;
}

.identity-option__label {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.identity-option__dot {
  width: 12px;
  height: 12px;
  border-radius: 9999px;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.45));
}

.identity-option__name {
  font-size: 0.95rem;
}

.identity-option__tag {
  background: rgba(59, 130, 246, 0.12);
  color: #2563eb;
  font-size: 0.75rem;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
}

.identity-option--action {
  font-weight: 500;
  color: #1f2937;
}

.identity-option-node {
  padding: 0.3rem 0.6rem;
  border-radius: 8px;
}

.identity-option-node--active {
  background: rgba(59, 130, 246, 0.08);
}

.identity-option-node--action {
  font-weight: 500;
}

:global(.identity-dropdown--night .n-dropdown-menu) {
  background-color: #0f172a;
  color: rgba(248, 250, 252, 0.95);
}

:global(.identity-dropdown--night .n-dropdown-option) {
  color: rgba(248, 250, 252, 0.95);
}

:global(.identity-dropdown--night .n-dropdown-option:hover),
:global(.identity-dropdown--night .n-dropdown-option.n-dropdown-option--active) {
  background-color: rgba(59, 130, 246, 0.25);
  color: #fff;
}

:global(.identity-dropdown--night .n-dropdown-divider) {
  background-color: rgba(148, 163, 184, 0.35);
}
</style>
const isNightPalette = computed(() => display.palette === 'night');
