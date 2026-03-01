<script setup lang="tsx">
import { computed, cloneVNode, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useUserStore } from '@/stores/user';
import { useDisplayStore } from '@/stores/display';
import AvatarVue from '@/components/avatar.vue';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import type { DropdownOption, DropdownMixedOption, DropdownRenderOption } from 'naive-ui';
import { NDropdown, NButton, NIcon, NTooltip, NPopover } from 'naive-ui';
import { Plus, Star, AlertTriangle, Camera, LayoutList, Settings } from '@vicons/tabler';
import IcOocRoleConfigPanel from './IcOocRoleConfigPanel.vue';
import { useI18n } from 'vue-i18n';

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
  (event: 'avatar-setup'): void;
}>();

const { t } = useI18n();

const chat = useChatStore();
const user = useUserStore();
const display = useDisplayStore();
const cardStore = useCharacterCardStore();

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
const MAX_VISIBLE_ROLE_COUNT = 9;
const sortedIdentities = computed(() => {
  const channelId = resolvedChannelId.value;
  const list = filteredIdentities.value.slice();
  if (!channelId || list.length <= 1) {
    return list;
  }
  return list.sort((a, b) => {
    const aLast = chat.getIdentityLastSpokenAt(channelId, a.id);
    const bLast = chat.getIdentityLastSpokenAt(channelId, b.id);
    if (aLast !== bLast) {
      return aLast - bLast;
    }
    return (a.sortOrder || 0) - (b.sortOrder || 0);
  });
});
const identityOptionCount = computed(() => Math.max(sortedIdentities.value.length, 1));

const activeIdentity = computed(() => chat.getActiveIdentity(resolvedChannelId.value));

const fallbackName = computed(() => chat.curMember?.nick || user.info.nick || user.info.username || '默认身份');
const fallbackAvatar = computed(() => user.info.avatar || '');

const buildAttachmentUrl = (token?: string) => resolveAttachmentUrl(token);

const displayName = computed(() => activeIdentity.value?.displayName || fallbackName.value);
const displayColor = computed(() => activeIdentity.value?.color || '');

// Mobile detection for responsive display
const isMobile = ref(false);
const MOBILE_BREAKPOINT = 768;
const MAX_NAME_LENGTH_MOBILE = 4;
const MOBILE_DROPDOWN_VIEWPORT_RATIO = 0.62;

const updateIsMobile = () => {
  const isNarrowViewport = window.innerWidth <= MOBILE_BREAKPOINT;
  const isCoarsePointer = window.matchMedia?.('(hover: none) and (pointer: coarse)')?.matches ?? false;
  const hasTouchPoints = (navigator?.maxTouchPoints || 0) > 0;
  const isMobileUa = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator?.userAgent || '');
  isMobile.value = isNarrowViewport || isCoarsePointer || (hasTouchPoints && isMobileUa);
};

// Displayed name: on mobile, if name exceeds 4 characters, show "切换" instead
const displayedButtonLabel = computed(() => {
  const name = displayName.value;
  if (isMobile.value && name.length > MAX_NAME_LENGTH_MOBILE) {
    return '切换';
  }
  return name;
});

const avatarSrc = computed(() => {
  return buildAttachmentUrl(activeIdentity.value?.avatarAttachmentId) || fallbackAvatar.value;
});
const toggleActionLabel = computed(() => (
  filterMode.value === 'favorites' ? '显示全部角色' : '仅显示收藏角色'
));
const renderActionIconByKey = (key: string) => {
  if (key === '__toggle') {
    return (
      <NIcon size={18}>
        {filterMode.value === 'favorites' ? <LayoutList /> : <Star />}
      </NIcon>
    );
  }
  if (key === '__create') {
    return (
      <NIcon size={18}>
        <Plus />
      </NIcon>
    );
  }
  return (
    <NIcon size={18}>
      <Settings />
    </NIcon>
  );
};

const renderMobileActionRow = () => (
  <div class="identity-action-bar-inline" onMousedown={consumeDropdownActionPointer} onClick={consumeDropdownActionPointer}>
    <button
      type="button"
      class="identity-action-bar-inline__btn"
      title={toggleActionLabel.value}
      aria-label={toggleActionLabel.value}
      onMousedown={consumeDropdownActionPointer}
      onClick={handleMobileToggleAction}
    >
      {renderActionIconByKey('__toggle')}
    </button>
    {canManageIdentities.value ? (
      <>
        <button
          type="button"
          class="identity-action-bar-inline__btn"
          title="创建新角色"
          aria-label="创建新角色"
          onMousedown={consumeDropdownActionPointer}
          onClick={handleMobileCreateAction}
        >
          {renderActionIconByKey('__create')}
        </button>
        <button
          type="button"
          class="identity-action-bar-inline__btn"
          title="管理角色"
          aria-label="管理角色"
          onMousedown={consumeDropdownActionPointer}
          onClick={handleMobileManageAction}
        >
          {renderActionIconByKey('__manage')}
        </button>
      </>
    ) : null}
  </div>
);

const options = computed<DropdownMixedOption[]>(() => {
  const list = sortedIdentities.value.map<DropdownOption>((item) => ({
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
  const result: DropdownMixedOption[] = [
    ...list,
    { type: 'divider', key: '__divider' },
  ];
  if (isMobile.value) {
    result.push({
      type: 'render',
      key: '__mobile_actions',
      render: renderMobileActionRow,
      props: {
        class: 'identity-action-row-inline',
      },
    });
    return result;
  }
  result.push({
    key: '__toggle',
    label: toggleActionLabel.value,
    class: 'identity-option identity-option--action identity-action identity-action--toggle',
    icon: () => renderActionIconByKey('__toggle'),
  });
  if (canManageIdentities.value) {
    result.push(
      {
        key: '__create',
        label: '创建新角色',
        class: 'identity-option identity-option--action identity-action identity-action--create',
        icon: () => renderActionIconByKey('__create'),
      },
      {
        key: '__manage',
        label: '管理角色',
        class: 'identity-option identity-option--action identity-action identity-action--manage',
        icon: () => renderActionIconByKey('__manage'),
      },
    );
  }
  return result;
});

const keepDropdownOpenAfterToggle = ref(false);
const consumeDropdownActionPointer = (event: MouseEvent) => {
  event.preventDefault();
  event.stopPropagation();
};
const applyToggleFilterMode = (keepOpenAfterSelect = false) => {
  if (favoriteFolderIds.value.length) {
    filterMode.value = filterMode.value === 'favorites' ? 'all' : 'favorites';
  } else {
    filterMode.value = 'all';
  }
  keepDropdownOpenAfterToggle.value = keepOpenAfterSelect;
  if (dropdownVisible.value) {
    syncDropdownMenuLayout();
  }
};
const handleMobileToggleAction = (event: MouseEvent) => {
  consumeDropdownActionPointer(event);
  applyToggleFilterMode(false);
};
const handleMobileCreateAction = (event: MouseEvent) => {
  consumeDropdownActionPointer(event);
  dropdownVisible.value = false;
  emit('create');
};
const handleMobileManageAction = (event: MouseEvent) => {
  consumeDropdownActionPointer(event);
  dropdownVisible.value = false;
  emit('manage');
};

const renderOption: DropdownRenderOption = ({ node, option }) => {
  if (option.key === '__divider') {
    return node;
  }
  if (option.key === '__create' || option.key === '__manage' || option.key === '__toggle') {
    const label = String(option.label || '');
    const actionKey = String(option.key || '');
    return cloneVNode(
      node,
      {
        class: [node.props?.class, 'identity-option-node', 'identity-option-node--action'],
      },
      {
        default: () => (
          <div
            class="identity-action-option"
            title={label}
            aria-label={label}
          >
            <span class="identity-action-option__icon">
              {renderActionIconByKey(actionKey)}
            </span>
            <span class="identity-action-option__text">{label}</span>
          </div>
        ),
      },
    );
  }
  if (option.key === '__placeholder') {
    return cloneVNode(node, {
      class: [node.props?.class, 'identity-option-node', 'identity-option-node--placeholder'],
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
    applyToggleFilterMode(true);
    return;
  }
  if (key === '__mobile_actions') {
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
  if (isObserverMode.value) {
    emit('identity-changed' as any);
    return;
  }
  try {
    await cardStore.syncCardForIdentity(channelId, String(key), {
      preserveWhenUnbound: true,
    });
  } catch (e) {
    console.warn('Failed to sync character card for identity', e);
  }
  emit('identity-changed' as any);
};

const showFavoriteBadge = computed(() => filterMode.value === 'favorites' && favoriteFolderIds.value.length > 0);

// IC/OOC mapping warning logic
const icOocRoleConfigPanelVisible = ref(false);

// Check if IC/OOC auto-switch is enabled but no mapping is configured
const icOocConfig = computed(() => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return { icRoleId: null, oocRoleId: null };
  return chat.getChannelIcOocRoleConfig(channelId);
});

const isAutoSwitchEnabled = computed(() => display.settings.autoSwitchRoleOnIcOocToggle);

// Check if user only has one role (need to create second role for IC/OOC mapping)
const hasOnlyOneRole = computed(() => identities.value.length === 1);
const hasNoRoles = computed(() => identities.value.length === 0);

const isObserverMode = computed(() => chat.isObserver || chat.observerMode || !!chat.observerWorldId);

const canManageIdentities = computed(() => {
  if (isObserverMode.value) return false;
  const worldId = chat.currentWorldId;
  if (!worldId) return false;
  const detail = chat.worldDetailMap[worldId];
  const role = detail?.memberRole;
  return role === 'owner' || role === 'admin' || role === 'member';
});

const isMappingMissing = computed(() => {
  if (!canManageIdentities.value) return false;
  if (!isAutoSwitchEnabled.value) return false;
  // If only one role, can't configure IC/OOC mapping properly
  if (hasOnlyOneRole.value || hasNoRoles.value) return true;
  const config = icOocConfig.value;
  // Show warning if either IC or OOC role is not configured
  return !config.icRoleId || !config.oocRoleId;
});

// Warning message based on what's missing
const warningMessage = computed(() => {
  // If no roles, suggest creating roles
  if (hasNoRoles.value) {
    return '请先创建角色以使用场内/场外切换';
  }
  // If only one role, guide to create second role
  if (hasOnlyOneRole.value) {
    return '请创建第二个角色以配置场内/场外映射';
  }
  const config = icOocConfig.value;
  if (!config.icRoleId && !config.oocRoleId) {
    return '尚未配置场内/场外角色映射，点击立即设置';
  }
  if (!config.icRoleId) {
    return '尚未配置场内（IC）角色，点击立即设置';
  }
  if (!config.oocRoleId) {
    return '尚未配置场外（OOC）角色，点击立即设置';
  }
  return '';
});

const handleOpenConfig = () => {
  // If only one role or no roles, emit create event to guide user to create role
  if (hasNoRoles.value || hasOnlyOneRole.value) {
    emit('create');
    return;
  }
  icOocRoleConfigPanelVisible.value = true;
};

const isNightPalette = computed(() => display.palette === 'night');
const dropdownVisible = ref(false);
const dropdownOverlayClass = computed(() => (
  isNightPalette.value
    ? 'identity-dropdown-overlay identity-dropdown--night'
    : 'identity-dropdown-overlay'
));
const dropdownMenuClass = computed(() => (
  isNightPalette.value
    ? 'identity-dropdown-menu identity-dropdown-menu--night'
    : 'identity-dropdown-menu'
));
const dropdownMenuProps = () => ({
  class: dropdownMenuClass.value,
});

// Avatar setup badge logic
const showAvatarSetupBadge = computed(() => {
  if (!canManageIdentities.value) return false;
  // Show badge when user has no avatar AND there's no active channel identity avatar
  if (!user.hasDefaultAvatar) return false;
  // If using a channel identity with a custom avatar, don't show
  if (activeIdentity.value?.avatarAttachmentId) return false;
  return true;
});

const handleAvatarSetup = () => {
  emit('avatar-setup');
};

const getDropdownMenuElement = (): HTMLElement | null => {
  if (typeof document === 'undefined') {
    return null;
  }
  const matches = Array.from(
    document.querySelectorAll<HTMLElement>(
      '.identity-dropdown-menu, .identity-dropdown-overlay .n-dropdown-menu, .identity-dropdown-overlay.n-dropdown-menu',
    ),
  ).filter(node => node.querySelector('.identity-option-node'));
  return matches[matches.length - 1] ?? null;
};

const ensureDropdownMenuHooks = (menuEl: HTMLElement) => {
  menuEl.classList.add('identity-dropdown-menu');
  menuEl.classList.toggle('identity-dropdown-menu--night', isNightPalette.value);

  const followerEl = menuEl.closest<HTMLElement>('.v-binder-follower-content');
  if (!followerEl) {
    return;
  }
  followerEl.classList.add('identity-dropdown-overlay');
  followerEl.classList.toggle('identity-dropdown--night', isNightPalette.value);
};

const applyDropdownMenuLayout = (): boolean => {
  const menuEl = getDropdownMenuElement();
  if (!menuEl) {
    return false;
  }
  ensureDropdownMenuHooks(menuEl);
  const optionEls = Array.from(menuEl.querySelectorAll<HTMLElement>('.n-dropdown-option'));
  const rowHeight = optionEls[0]?.offsetHeight || 36;
  const dividerHeight = menuEl.querySelector<HTMLElement>('.n-dropdown-divider')?.offsetHeight || 8;
  const visibleRoleCount = Math.min(identityOptionCount.value, MAX_VISIBLE_ROLE_COUNT);
  const actionCount = isMobile.value ? 1 : (1 + (canManageIdentities.value ? 2 : 0)); // mobile action bar or desktop actions
  const menuPadding = 8;
  const desiredHeight = Math.ceil(rowHeight * visibleRoleCount + rowHeight * actionCount + dividerHeight + menuPadding);
  const viewportHeight = Math.max(
    320,
    Math.floor(window.visualViewport?.height || window.innerHeight || 0),
  );
  const mobileMaxHeight = Math.max(
    rowHeight * 4,
    Math.floor(viewportHeight * MOBILE_DROPDOWN_VIEWPORT_RATIO) - 12,
  );
  const maxHeight = isMobile.value ? Math.min(desiredHeight, mobileMaxHeight) : desiredHeight;
  const shouldScroll = identityOptionCount.value > MAX_VISIBLE_ROLE_COUNT || maxHeight < desiredHeight;
  menuEl.classList.toggle('identity-dropdown-menu--mobile', isMobile.value);
  menuEl.style.maxHeight = `${maxHeight}px`;
  menuEl.style.overflowY = shouldScroll ? 'auto' : 'hidden';
  if (shouldScroll) {
    menuEl.scrollTop = menuEl.scrollHeight;
  }
  return true;
};

const syncDropdownMenuLayout = (attempt = 0) => {
  if (typeof window === 'undefined') {
    return;
  }
  void nextTick(() => {
    window.requestAnimationFrame(() => {
      const applied = applyDropdownMenuLayout();
      if (!applied && attempt < 8) {
        window.setTimeout(() => {
          syncDropdownMenuLayout(attempt + 1);
        }, 16 * (attempt + 1));
      }
    });
  });
};

const handleDropdownShowUpdate = (show: boolean) => {
  if (!show && keepDropdownOpenAfterToggle.value) {
    keepDropdownOpenAfterToggle.value = false;
    dropdownVisible.value = true;
    syncDropdownMenuLayout();
    return;
  }
  keepDropdownOpenAfterToggle.value = false;
  dropdownVisible.value = show;
  if (show) {
    syncDropdownMenuLayout();
  }
};

const handleViewportResize = () => {
  updateIsMobile();
  if (dropdownVisible.value) {
    syncDropdownMenuLayout();
  }
};

onMounted(() => {
  updateIsMobile();
  window.addEventListener('resize', handleViewportResize);
  window.visualViewport?.addEventListener('resize', handleViewportResize);
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleViewportResize);
  window.visualViewport?.removeEventListener('resize', handleViewportResize);
});

const sortedIdentitySignature = computed(() => sortedIdentities.value.map(item => item.id).join('|'));
watch([dropdownVisible, sortedIdentitySignature, () => canManageIdentities.value, isMobile, isNightPalette], ([visible]) => {
  if (!visible) {
    return;
  }
  syncDropdownMenuLayout();
});
</script>

<template>
  <div class="identity-switcher-wrapper">
    <!-- IC/OOC Mapping Warning Button -->
    <n-tooltip v-if="isMappingMissing" trigger="hover" placement="top">
      <template #trigger>
        <n-button
          quaternary
          circle
          size="tiny"
          class="ic-ooc-warning-button"
          @click="handleOpenConfig"
        >
          <template #icon>
            <n-icon :component="AlertTriangle" size="16" />
          </template>
        </n-button>
      </template>
      <span class="warning-tooltip-content">{{ warningMessage }}</span>
    </n-tooltip>

    <!-- Avatar Setup Badge -->
    <n-tooltip v-if="showAvatarSetupBadge" trigger="hover" placement="top">
      <template #trigger>
        <n-button
          quaternary
          circle
          size="tiny"
          class="avatar-setup-badge"
          @click="handleAvatarSetup"
        >
          <template #icon>
            <n-icon :component="Camera" size="16" />
          </template>
        </n-button>
      </template>
      <span class="warning-tooltip-content">{{ t('avatarPrompt.badge') }}</span>
    </n-tooltip>

    <n-dropdown
      trigger="click"
      :options="options"
      :show="dropdownVisible"
      :show-arrow="false"
      placement="top-start"
      :disabled="!resolvedChannelId || disabled"
      :render-option="renderOption"
      :menu-props="dropdownMenuProps"
      :content-class="dropdownOverlayClass"
      @update:show="handleDropdownShowUpdate"
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
          {{ displayedButtonLabel }}
        </span>
        <n-icon v-if="showFavoriteBadge" :component="Star" size="12" class="identity-switcher__favorite" />
      </n-button>
    </n-dropdown>

    <!-- IC/OOC Role Config Panel -->
    <IcOocRoleConfigPanel
      v-model:show="icOocRoleConfigPanelVisible"
      :channel-id="resolvedChannelId"
    />
  </div>
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
  color: var(--sc-text-primary, #1f2937);
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

:global(.identity-action-row-inline) {
  padding: 0.24rem 0.35rem;
}

:global(.identity-action-bar-inline) {
  display: inline-flex;
  align-items: center;
  gap: 0.38rem;
  min-width: 11rem;
}

:global(.identity-action-bar-inline__btn) {
  appearance: none;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  background: var(--sc-chip-bg, rgba(148, 163, 184, 0.12));
  color: var(--sc-text-primary, #1f2937);
  border-radius: 10px;
  width: 2rem;
  height: 2rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: border-color 0.2s ease, background-color 0.2s ease, transform 0.15s ease;
}

:global(.identity-action-bar-inline__btn:hover) {
  border-color: var(--sc-border-strong, rgba(148, 163, 184, 0.55));
  background: var(--sc-bg-layer, rgba(148, 163, 184, 0.2));
}

:global(.identity-action-bar-inline__btn:active) {
  transform: scale(0.96);
}

.identity-action-option {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 11rem;
}

.identity-action-option__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.2rem;
  height: 1.2rem;
  color: inherit;
  flex: 0 0 auto;
}

.identity-action-option__text {
  display: inline-flex;
  align-items: center;
}

.identity-option-node--placeholder {
  opacity: 0.78;
}

:global(.identity-dropdown-menu.identity-dropdown-menu--night) {
  background-color: #0f172a;
  color: rgba(248, 250, 252, 0.95);
}

:global(.identity-dropdown-menu.identity-dropdown-menu--night .n-dropdown-option) {
  color: rgba(248, 250, 252, 0.95);
}

:global(.identity-dropdown-menu.identity-dropdown-menu--night .n-dropdown-option:hover),
:global(.identity-dropdown-menu.identity-dropdown-menu--night .n-dropdown-option.n-dropdown-option--active) {
  background-color: rgba(59, 130, 246, 0.25);
  color: #fff;
}

:global(.identity-dropdown-menu.identity-dropdown-menu--night .n-dropdown-divider) {
  background-color: rgba(148, 163, 184, 0.35);
}

:global(.identity-dropdown-menu) {
  max-height: min(62vh, calc(36px * 11 + 8px));
  overflow-y: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
  touch-action: pan-y;
  scrollbar-width: thin;
  scrollbar-color: var(--sc-scrollbar-thumb, var(--sc-border-mute, rgba(148, 163, 184, 0.45))) transparent;
}

:global(.identity-dropdown-menu::-webkit-scrollbar) {
  width: var(--sc-scrollbar-size, 4px);
  height: var(--sc-scrollbar-size, 4px);
}

:global(.identity-dropdown-menu::-webkit-scrollbar-track) {
  background: transparent;
}

:global(.identity-dropdown-menu::-webkit-scrollbar-thumb) {
  background-color: var(--sc-scrollbar-thumb, var(--sc-border-mute, rgba(148, 163, 184, 0.45)));
  border-radius: 999px;
}

:global(.identity-dropdown-menu::-webkit-scrollbar-thumb:hover) {
  background-color: var(--sc-scrollbar-thumb-hover, var(--sc-border-strong, rgba(148, 163, 184, 0.6)));
}

/* Wrapper for identity switcher and warning */
.identity-switcher-wrapper {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

/* IC/OOC Warning Button */
.ic-ooc-warning-button {
  color: #f59e0b;
  animation: warning-glow 2s ease-in-out infinite;
  transition: color 0.2s ease, transform 0.15s ease, box-shadow 0.2s ease;
  filter: drop-shadow(0 0 4px rgba(245, 158, 11, 0.6));
}

.ic-ooc-warning-button:hover {
  color: #d97706;
  transform: scale(1.15);
  animation: none;
  filter: drop-shadow(0 0 8px rgba(245, 158, 11, 0.9));
}

@keyframes warning-glow {
  0%, 100% {
    filter: drop-shadow(0 0 4px rgba(245, 158, 11, 0.6));
    opacity: 1;
  }
  50% {
    filter: drop-shadow(0 0 10px rgba(245, 158, 11, 0.9));
    opacity: 0.85;
  }
}

/* Warning tooltip content */
.warning-tooltip-content {
  font-size: 0.8rem;
  line-height: 1.4;
  max-width: 200px;
}

/* Mobile responsive adjustments */
@media (max-width: 768px) {
  .identity-switcher {
    gap: 0.25rem;
    min-width: 40px;
    min-height: 40px;
    padding: 0.35rem;
    justify-content: center;
  }

  .identity-switcher__label,
  .identity-switcher__favorite {
    display: none;
  }

  .identity-switcher__color {
    width: 8px;
    height: 8px;
    flex: 0 0 auto;
  }

  .identity-switcher-wrapper {
    gap: 0.25rem;
  }

  .ic-ooc-warning-button,
  .avatar-setup-badge {
    min-width: 32px;
    min-height: 32px;
  }

  .ic-ooc-warning-button {
    padding: 0.15rem;
  }

  .warning-tooltip-content {
    font-size: 0.75rem;
    max-width: 160px;
  }

  :global(.identity-dropdown-menu.identity-dropdown-menu--mobile .identity-action-row-inline) {
    padding: 0.22rem 0.25rem;
  }

  :global(.identity-dropdown-menu.identity-dropdown-menu--mobile .identity-action-bar-inline) {
    width: 100%;
    min-width: 0;
    justify-content: space-between;
    gap: 0.3rem;
  }

  :global(.identity-dropdown-menu.identity-dropdown-menu--mobile .identity-action-bar-inline__btn) {
    flex: 1 1 0;
    min-width: 0;
    width: auto;
    height: 2.1rem;
    border-radius: 11px;
  }
}

/* Avatar Setup Badge */
.avatar-setup-badge {
  color: #3b82f6;
  animation: avatar-badge-pulse 2s ease-in-out infinite;
  transition: color 0.2s ease, transform 0.15s ease, box-shadow 0.2s ease;
  filter: drop-shadow(0 0 4px rgba(59, 130, 246, 0.5));
}

.avatar-setup-badge:hover {
  color: #2563eb;
  transform: scale(1.15);
  animation: none;
  filter: drop-shadow(0 0 8px rgba(59, 130, 246, 0.8));
}

@keyframes avatar-badge-pulse {
  0%, 100% {
    filter: drop-shadow(0 0 4px rgba(59, 130, 246, 0.5));
    opacity: 1;
  }
  50% {
    filter: drop-shadow(0 0 10px rgba(59, 130, 246, 0.8));
    opacity: 0.85;
  }
}
</style>
