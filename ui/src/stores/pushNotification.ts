import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import defaultAvatarUrl from '@/assets/head3.png';
import { urlBase } from '@/stores/_config';

const STORAGE_KEY = 'sc-push-notification-enabled';

const isEmbedMode = (): boolean => {
    if (typeof window === 'undefined') return true;
    const hash = window.location.hash || '';
    return hash.startsWith('#/embed');
};

const resolveEmbedNotifyOwnerFromHash = (): boolean => {
    if (typeof window === 'undefined') return true;
    const hash = window.location.hash || '';
    if (!hash.startsWith('#/embed')) return true;
    const queryIndex = hash.indexOf('?');
    if (queryIndex === -1) return false;
    try {
        const params = new URLSearchParams(hash.slice(queryIndex + 1));
        const raw = params.get('notifyOwner');
        return raw === '1' || raw === 'true';
    } catch {
        return false;
    }
};

/**
 * 推送通知 Store
 * 
 * 使用浏览器原生 Notification API 实现前台推送通知
 * 当用户切换标签页时，仍可收到新消息通知
 */
export const usePushNotificationStore = defineStore('pushNotification', () => {
    // 用户开关状态（持久化到 localStorage）
    const enabled = ref(true);

    // 浏览器通知权限状态
    const permission = ref<NotificationPermission>('default');

    // 是否支持 Notification API
    const supported = computed(() => {
        return typeof window !== 'undefined' && 'Notification' in window;
    });

    // embed 模式下由 shell 指定是否允许通知（默认继承 URL 参数；未设置则全禁）
    const embedNotifyOwnerOverride = ref<boolean | null>(null);

    const embedNotifyOwnerEnabled = computed(() => {
        if (!isEmbedMode()) return true;
        if (embedNotifyOwnerOverride.value !== null) return embedNotifyOwnerOverride.value;
        return resolveEmbedNotifyOwnerFromHash();
    });

    // 是否可以发送通知
    const canNotify = computed(() => {
        return supported.value && enabled.value && permission.value === 'granted' && embedNotifyOwnerEnabled.value;
    });

    /**
     * 初始化：从 localStorage 恢复状态
     */
    const init = () => {
        if (typeof window === 'undefined') return;

        // 恢复开关状态
        const saved = localStorage.getItem(STORAGE_KEY);
        if (saved === 'false') {
            enabled.value = false;
        } else if (saved === 'true') {
            enabled.value = true;
        }

        // 检查当前权限状态
        if (supported.value) {
            permission.value = Notification.permission;
        }
    };

    /**
     * 请求通知权限
     */
    const requestPermission = async (): Promise<boolean> => {
        if (!supported.value) {
            console.warn('[PushNotification] Notification API not supported');
            return false;
        }

        if (permission.value === 'granted') {
            return true;
        }

        if (permission.value === 'denied') {
            console.warn('[PushNotification] Notification permission denied');
            return false;
        }

        try {
            const result = await Notification.requestPermission();
            permission.value = result;
            return result === 'granted';
        } catch (error) {
            console.error('[PushNotification] Failed to request permission:', error);
            return false;
        }
    };

    /**
     * 切换推送开关
     */
    const toggle = async (): Promise<void> => {
        if (enabled.value) {
            // 关闭推送
            enabled.value = false;
            localStorage.setItem(STORAGE_KEY, 'false');
            return;
        }

        // 开启推送：请求权限
        const granted = await requestPermission();
        if (granted) {
            enabled.value = true;
            localStorage.setItem(STORAGE_KEY, 'true');
        }
    };

    /**
     * embed 模式：由分屏壳页面动态设置当前窗格是否允许通知
     * - 非 embed 模式下该值不会生效
     */
    const setEmbedNotifyOwner = (enabled: boolean) => {
        embedNotifyOwnerOverride.value = !!enabled;
    };

    /**
     * 显示通知
     * @param title 通知标题（通常是频道名）
     * @param body 通知内容（用户名: 消息内容）
     * @param channelId 频道 ID（用于点击跳转）
     * @param icon 可选，通知图标 URL（默认使用默认头像）
     */
    const showNotification = (title: string, body: string, channelId: string, icon?: string): void => {
        const hasTopFocus = () => {
            try {
                return window.top ? window.top.document.hasFocus() : document.hasFocus();
            } catch {
                return document.hasFocus();
            }
        };

        if (!canNotify.value) {
            return;
        }

        // 如果页面有焦点，不显示通知
        if (hasTopFocus()) {
            return;
        }

        try {
            // Notification API 需要完整的绝对 URL
            let resolvedIcon = icon || defaultAvatarUrl;

            // 处理 id:xxx 格式的附件 ID
            if (resolvedIcon && resolvedIcon.startsWith('id:')) {
                const attachmentId = resolvedIcon.slice(3);
                resolvedIcon = `${urlBase}/api/v1/attachment/${attachmentId}`;
            }

            // 处理其他相对路径
            if (resolvedIcon && !resolvedIcon.startsWith('http') && !resolvedIcon.startsWith('data:') && !resolvedIcon.startsWith('blob:')) {
                // 相对路径转绝对路径
                resolvedIcon = new URL(resolvedIcon, window.location.origin).href;
            }

            const notification = new Notification(title, {
                body,
                icon: resolvedIcon,
                tag: `sealchat-channel-${channelId}`, // 同一频道的通知会合并
                requireInteraction: false,
            });

            // 点击通知：聚焦窗口并跳转到频道
            notification.onclick = () => {
                window.focus();
                notification.close();

                // 触发频道切换事件
                if (channelId) {
                    import('./chat').then(({ useChatStore }) => {
                        const chat = useChatStore();
                        chat.channelSwitchTo(channelId);
                    });
                }
            };

            // 5秒后自动关闭
            setTimeout(() => {
                notification.close();
            }, 5000);
        } catch (error) {
            console.error('[PushNotification] Failed to show notification:', error);
        }
    };

    // 初始化
    init();

    return {
        enabled,
        permission,
        supported,
        canNotify,
        requestPermission,
        toggle,
        setEmbedNotifyOwner,
        showNotification,
    };
});
