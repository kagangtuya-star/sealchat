<script setup lang="tsx">
import { computed, cloneVNode } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import AvatarVue from '@/components/avatar.vue';
import { urlBase } from '@/stores/_config';
import type { DropdownOption, DropdownRenderOption } from 'naive-ui';
import { NDropdown, NButton, NIcon } from 'naive-ui';
import { Plus } from '@vicons/tabler';

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

const resolvedChannelId = computed(() => props.channelId || chat.curChannel?.id || '');

const identities = computed(() => {
  const id = resolvedChannelId.value;
  if (!id) {
    return [];
  }
  return chat.channelIdentities[id] || [];
});

const activeIdentity = computed(() => chat.getActiveIdentity(resolvedChannelId.value));

const fallbackName = computed(() => chat.curMember?.nick || user.info.nick || user.info.username || '默认身份');
const fallbackAvatar = computed(() => user.info.avatar || '');

const buildAttachmentUrl = (token?: string) => {
  const raw = (token || '').trim();
  if (!raw) {
    return '';
  }
  if (/^(https?:|blob:|data:|\/\/)/i.test(raw)) {
    return raw;
  }
  const normalized = raw.startsWith('id:') ? raw.slice(3) : raw;
  if (!normalized) {
    return '';
  }
  return `${urlBase}/api/v1/attachment/${normalized}`;
};

const displayName = computed(() => activeIdentity.value?.displayName || fallbackName.value);
const displayColor = computed(() => activeIdentity.value?.color || '');
const avatarSrc = computed(() => {
  return buildAttachmentUrl(activeIdentity.value?.avatarAttachmentId) || fallbackAvatar.value;
});

const options = computed<DropdownOption[]>(() => {
  const list = identities.value.map<DropdownOption>((item) => ({
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
  return [
    ...list,
    { type: 'divider', key: '__divider' },
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
  if (option.key === '__create' || option.key === '__manage') {
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
  const channelId = resolvedChannelId.value;
  if (!channelId || props.disabled) {
    return;
  }
  chat.setActiveIdentity(channelId, String(key));
  emit('identity-changed' as any);
};
</script>

<template>
  <n-dropdown
    trigger="click"
    :options="options"
    :show-arrow="false"
    placement="top-start"
    :disabled="!resolvedChannelId || disabled"
    :render-option="renderOption"
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
  border: 1px solid rgba(148, 163, 184, 0.35);
  background-color: rgba(248, 250, 252, 0.9);
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.12);
}

.identity-switcher__label {
  font-size: 0.8rem;
  font-weight: 600;
  color: #374151;
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
  border: 1px solid rgba(148, 163, 184, 0.45);
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
  border: 1px solid rgba(148, 163, 184, 0.45);
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
</style>
