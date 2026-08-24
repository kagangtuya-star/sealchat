import { computed, h, onBeforeUnmount, ref, type ComputedRef, type Ref } from 'vue';
import type { MentionOption } from 'naive-ui';
import AvatarVue from '@/components/avatar.vue';
import { ensurePinyinLoaded, matchText } from '@/utils/pinyinMatch';
import { shouldResetMentionOptionsOnSearchStart, sortMentionableMembersByMode } from '../mentionOptionOrdering';
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';

interface MentionSuggestionsOptions {
  chat: ReturnType<typeof useChatStore>;
  utils: ReturnType<typeof useUtilsStore>;
  inputIcMode: ComputedRef<'ic' | 'ooc'>;
  textToSend: Ref<string>;
  pauseKeydown: Ref<boolean>;
}

export const useMentionSuggestions = ({
  chat,
  utils,
  inputIcMode,
  textToSend,
  pauseKeydown,
}: MentionSuggestionsOptions) => {
  const atOptions = ref<MentionOption[]>([]);
  const atLoading = ref(true);
  const atPrefix = computed(() => chat.atOptionsOn ? ['@', '/', '.'] : ['@']);
  let atSearchRequestSeq = 0;
  const activeIntervals = new Set<ReturnType<typeof setInterval>>();

  const atRenderLabel = (option: MentionOption) => {
    switch (option.type) {
      case 'cmd':
        return h('div', { class: 'flex items-center space-x-1' }, [
          h('span', null, (option as any).data.info),
        ]);
      case 'at': {
        const data = (option as any).data || {};
        const identityType = data.identityType;
        const color = data.color || 'inherit';
        const isAll = data.userId === 'all';
        const children: any[] = [
          isAll
            ? h('span', { class: 'at-option-avatar at-option-avatar--all' }, '@')
            : h(AvatarVue, { size: 24, border: false, src: data.avatar }),
          h('span', { style: { color: isAll ? '#ef4444' : color } }, String(option.label ?? '')),
        ];
        if (identityType && identityType !== 'all') {
          children.push(h(
            'span',
            { class: `at-option-tag at-option-tag--${identityType}` },
            identityType === 'ic' ? '场内' : identityType === 'ooc' ? '场外' : '用户',
          ));
        }
        return h('div', { class: 'flex items-center space-x-2' }, children);
      }
      default:
        return undefined;
    }
  };

  const atHandleSearch = async (pattern: string, prefix: string) => {
    const requestSeq = ++atSearchRequestSeq;
    pauseKeydown.value = true;
    atLoading.value = true;

    const atElementCheck = () => {
      const els = document.getElementsByClassName('v-binder-follower-content');
      if (els.length) {
        return els[0].children.length > 0;
      }
      return false;
    };

    // 如果at框非正常消失，那么也一样要恢复回车键功能
    const interval = setInterval(() => {
      if (!atElementCheck()) {
        pauseKeydown.value = false;
        clearInterval(interval);
        activeIntervals.delete(interval);
      }
    }, 100);
    activeIntervals.add(interval);

    try {
      switch (prefix) {
        case '@': {
          if (shouldResetMentionOptionsOnSearchStart(prefix)) {
            atOptions.value = [];
          }
          await ensurePinyinLoaded();
          if (requestSeq !== atSearchRequestSeq) {
            return;
          }
          const channelId = chat.curChannel?.id;
          if (!channelId) {
            atOptions.value = [];
            break;
          }
          const result = await chat.fetchMentionableMembers(channelId);
          if (requestSeq !== atSearchRequestSeq) {
            return;
          }
          const list: MentionOption[] = [];
          if (result.canAtAll) {
            const allMatches = !pattern || matchText(pattern, '全体成员') || pattern.toLowerCase() === 'all';
            if (allMatches) {
              list.push({
                type: 'at',
                value: '<at id="all" name="全体成员"/>',
                label: '全体成员',
                data: { userId: 'all', displayName: '全体成员', identityType: 'all' },
              });
            }
          }
          const sortedItems = sortMentionableMembersByMode(
            result.items || [],
            inputIcMode.value === 'ooc' ? 'ooc' : 'ic',
          );
          for (const item of sortedItems) {
            if (pattern && !matchText(pattern, item.displayName)) {
              continue;
            }
            const escapedName = item.displayName.replace(/"/g, '&quot;');
            list.push({
              type: 'at',
              value: `<at id="${item.userId}" name="${escapedName}"/>`,
              label: item.displayName,
              data: item,
            });
          }
          atOptions.value = list.slice(0, 10);
          break;
        }
        case '.':
        case '/':
          if (chat.atOptionsOn) {
            atOptions.value = [['x', 'x d100']].map((i) => ({
              type: 'cmd',
              value: i[0],
              label: i[0],
              data: { info: '/x 简易骰点指令，如：/x d100 (100面骰)' },
            }));
            for (const [id, data] of Object.entries(utils.botCommands)) {
              for (const [k, v] of Object.entries(data)) {
                atOptions.value.push({
                  type: 'cmd',
                  value: k,
                  label: k,
                  data: {
                    info: `/${k} ` + (v as any).split('\n', 1)[0].replace(/^\.\S+/, ''),
                  },
                });
              }
            }
          }
          break;
      }
    } finally {
      if (requestSeq === atSearchRequestSeq) {
        atLoading.value = false;
      }
    }
  };

  onBeforeUnmount(() => {
    atSearchRequestSeq += 1;
    activeIntervals.forEach((interval) => clearInterval(interval));
    activeIntervals.clear();
  });

  return {
    atOptions,
    atLoading,
    atPrefix,
    atRenderLabel,
    atHandleSearch,
  };
};
