import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import defaultAvatarUrl from '@/assets/head3.png';
import { urlBase } from '@/stores/_config';

const STORAGE_KEY = 'sc-push-notification-enabled';

/**
 * 推送通知 Store
 * 
 * 使用浏览器原生 Notification API 实现前台推送通知
 * 当用户切换标签页时，仍可收到新消息通知
 */
export const usePushNotificationStore = defineStore('pushNotification', () => {
    // 用户开关状态（持久化到 localStorage）
    const enabled = ref(false);

    // 浏览器通知权限状态
    const permission = ref<NotificationPermission>('default');

    // 是否支持 Notification API
    const supported = computed(() => {
        return typeof window !== 'undefined' && 'Notification' in window;
    });

    // 是否可以发送通知
    const canNotify = computed(() => {
        return supported.value && enabled.value && permission.value === 'granted';
    });

    /**
     * 初始化：从 localStorage 恢复状态
     */
    const init = () => {
        if (typeof window === 'undefined') return;

        // 恢复开关状态
        const saved = localStorage.getItem(STORAGE_KEY);
        if (saved === 'true') {
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
     * 显示通知
     * @param title 通知标题（通常是频道名）
     * @param body 通知内容（用户名: 消息内容）
     * @param channelId 频道 ID（用于点击跳转）
     * @param icon 可选，通知图标 URL（默认使用默认头像）
     */
    const showNotification = (title: string, body: string, channelId: string, icon?: string): void => {
        console.log('[PushNotification] showNotification called:', {
            title, body, channelId, icon,
            canNotify: canNotify.value,
            hasFocus: document.hasFocus(),
            permission: permission.value
        });

        if (!canNotify.value) {
            console.log('[PushNotification] Cannot notify - canNotify is false');
            return;
        }

        // 如果页面有焦点，不显示通知
        if (document.hasFocus()) {
            console.log('[PushNotification] Page has focus, skipping notification');
            return;
        }

        try {
            // Notification API 需要完整的绝对 URL
            let resolvedIcon = icon || defaultAvatarUrl;
            console.log('[PushNotification] Icon before resolve:', resolvedIcon);

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
            console.log('[PushNotification] Icon after resolve:', resolvedIcon);

            const notification = new Notification(title, {
                body,
                icon: resolvedIcon,
                tag: `sealchat-channel-${channelId}`, // 同一频道的通知会合并
                requireInteraction: false,
            });
            console.log('[PushNotification] Notification created successfully');

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
        showNotification,
    };
});
