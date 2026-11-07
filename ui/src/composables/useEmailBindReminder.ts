import { ref, onMounted, onUnmounted, defineAsyncComponent } from 'vue';
import { api } from '@/stores/_config';
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';

const PREF_KEY = 'email_bind_prompt_dismissed';
const DELAY_MS = 60 * 1000;

export const EmailBindPrompt = defineAsyncComponent(
  () => import('@/components/EmailBindPrompt.vue')
);

export function useEmailBindReminder() {
  const user = useUserStore();
  const utils = useUtilsStore();
  const showPrompt = ref(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  const shouldShow = async (): Promise<boolean> => {
    const config = utils.config;
    if (!config?.emailAuth?.enabled) return false;
    const info = user.info;
    if (!info || info.email) return false;
    try {
      const res = await api.get('/api/v1/user/preferences', { params: { key: PREF_KEY } });
      if (res.data?.exists && res.data?.value === 'true') return false;
    } catch {
      // API failure: show prompt (fallback)
    }
    return true;
  };

  const dismiss = async () => {
    try {
      await api.post('/api/v1/user/preferences', { key: PREF_KEY, value: 'true' });
    } catch {
      // ignore
    }
  };

  const start = async () => {
    if (!(await shouldShow())) return;
    timer = setTimeout(() => {
      showPrompt.value = true;
    }, DELAY_MS);
  };

  const stop = () => {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  };

  onMounted(() => {
    void start();
  });

  onUnmounted(() => {
    stop();
  });

  return {
    showPrompt,
    dismiss,
  };
}
