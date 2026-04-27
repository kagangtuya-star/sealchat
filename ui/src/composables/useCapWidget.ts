import { nextTick, onBeforeUnmount, ref } from 'vue';
import { urlBase } from '@/stores/_config';

type CapScene = 'signup' | 'signin' | 'passwordReset' | 'password_reset' | 'password-reset';

type CapWidgetElement = HTMLElement & {
  reset?: () => void;
  token?: string;
};

let capScriptPromise: Promise<void> | null = null;

const resolveCapScenePath = (scene: CapScene) => {
  switch (scene) {
    case 'signin':
      return 'signin';
    case 'passwordReset':
    case 'password_reset':
    case 'password-reset':
      return 'password-reset';
    default:
      return 'signup';
  }
};

const resolveAssetUrl = (path: string) => new URL(path, window.location.href).toString();

const ensureCapScript = async () => {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return;
  }
  window.CAP_CUSTOM_WASM_URL = resolveAssetUrl('vendor/cap/cap_wasm_bg.wasm');
  if (customElements.get('cap-widget')) {
    return;
  }
  if (!capScriptPromise) {
    capScriptPromise = new Promise<void>((resolve, reject) => {
      const existing = document.getElementById('cap-widget-script') as HTMLScriptElement | null;
      if (existing) {
        existing.addEventListener('load', () => resolve(), { once: true });
        existing.addEventListener('error', () => reject(new Error('Cap script load failed')), { once: true });
        return;
      }
      const script = document.createElement('script');
      script.id = 'cap-widget-script';
      script.src = resolveAssetUrl('vendor/cap/cap.min.js');
      script.async = true;
      script.defer = true;
      script.onload = () => resolve();
      script.onerror = () => reject(new Error('Cap script load failed'));
      document.head.appendChild(script);
    }).catch((err) => {
      capScriptPromise = null;
      throw err;
    });
  }
  await capScriptPromise;
};

export const useCapWidget = (scene: CapScene) => {
  const container = ref<HTMLDivElement | null>(null);
  const token = ref('');
  const error = ref('');
  const loading = ref(false);
  const widget = ref<CapWidgetElement | null>(null);

  const destroy = () => {
    token.value = '';
    error.value = '';
    loading.value = false;
    if (widget.value?.reset) {
      widget.value.reset();
    }
    widget.value?.remove();
    widget.value = null;
    if (container.value) {
      container.value.innerHTML = '';
    }
  };

  const render = async () => {
    if (typeof window === 'undefined') {
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      await ensureCapScript();
      await nextTick();
      if (!container.value) {
        error.value = 'Cap 初始化失败';
        return;
      }
      destroy();
      const el = document.createElement('cap-widget') as CapWidgetElement;
      el.setAttribute('data-cap-api-endpoint', `${urlBase}/api/v1/captcha/cap/${resolveCapScenePath(scene)}/`);
      el.setAttribute('data-cap-disable-haptics', 'true');
      el.style.display = 'block';
      el.style.width = '100%';
      el.style.setProperty('--cap-widget-width', '100%');
      el.style.setProperty('--cap-widget-height', '58px');
      el.addEventListener('solve', (event: Event) => {
        token.value = ((event as CustomEvent<{ token?: string }>).detail?.token || '').trim();
        error.value = '';
      });
      el.addEventListener('reset', () => {
        token.value = '';
      });
      el.addEventListener('error', () => {
        token.value = '';
        error.value = '验证码加载失败，请重试';
      });
      container.value.appendChild(el);
      widget.value = el;
    } catch (err) {
      console.error(err);
      error.value = '无法加载 Cap 验证码，请稍后重试';
    } finally {
      loading.value = false;
    }
  };

  const reset = async () => {
    destroy();
    await render();
  };

  onBeforeUnmount(() => {
    destroy();
  });

  return {
    container,
    token,
    error,
    loading,
    render,
    reset,
    destroy,
  };
};

declare global {
  interface Window {
    CAP_CUSTOM_WASM_URL?: string;
  }
}
