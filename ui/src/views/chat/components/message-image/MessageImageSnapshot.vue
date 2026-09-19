<script setup lang="ts">
import { ref } from 'vue';
import type {
  MessageImageSnapshotGroup,
  MessageImageSnapshotPalette,
} from './messageImageSnapshot';

const props = defineProps<{
  groups: MessageImageSnapshotGroup[];
  palette: MessageImageSnapshotPalette;
}>();

const failedAvatars = ref(new Set<string>());

const markAvatarFailed = (key: string) => {
  failedAvatars.value = new Set(failedAvatars.value).add(key);
};

const avatarFallback = (name: string) => Array.from(name.trim() || '匿').slice(0, 2).join('');
</script>

<template>
  <div
    class="message-image-snapshot"
    data-message-image-snapshot
    :style="{
      '--snapshot-background': props.palette.background,
      '--snapshot-primary-text': props.palette.primaryText,
      '--snapshot-secondary-text': props.palette.secondaryText,
      '--snapshot-subtle-border': props.palette.subtleBorder,
      '--snapshot-code-background': props.palette.codeBackground,
      '--snapshot-link': props.palette.link,
    }"
  >
    <section v-for="group in props.groups" :key="group.key" class="message-image-snapshot__group">
      <div class="message-image-snapshot__avatar" aria-hidden="true">
        <span v-if="!group.avatarUrl || failedAvatars.has(group.key)">
          {{ avatarFallback(group.senderName) }}
        </span>
        <img
          v-if="group.avatarUrl && !failedAvatars.has(group.key)"
          :src="group.avatarUrl"
          alt=""
          @error="markAvatarFailed(group.key)"
        >
      </div>
      <main class="message-image-snapshot__main">
        <header class="message-image-snapshot__header">
          <span
            class="message-image-snapshot__name"
            :style="group.senderColor ? { color: group.senderColor } : undefined"
          >
            {{ group.senderName }}
          </span>
        </header>
        <div v-if="group.whisperLabel" class="message-image-snapshot__whisper">
          {{ group.whisperLabel }}
        </div>
        <div class="message-image-snapshot__entries">
          <div
            v-for="entry in group.entries"
            :key="entry.id"
            :class="[
              'message-image-snapshot__entry',
              `message-image-snapshot__entry--${entry.tone}`,
            ]"
          >
            <span v-if="entry.tone === 'ooc'" class="message-image-snapshot__tone">OOC</span>
            <span v-else-if="entry.tone === 'archived'" class="message-image-snapshot__tone">已归档</span>
            <div class="message-image-snapshot__content" v-html="entry.contentHtml" />
          </div>
        </div>
      </main>
    </section>
  </div>
</template>

<style scoped>
.message-image-snapshot {
  width: 100%;
  box-sizing: border-box;
  padding: 14px 16px;
  color: var(--snapshot-primary-text);
  background: var(--snapshot-background);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", Arial, sans-serif;
  font-size: 13px;
  line-height: 1.48;
  letter-spacing: 0;
}

.message-image-snapshot__group {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.message-image-snapshot__group + .message-image-snapshot__group {
  margin-top: 9px;
}

.message-image-snapshot__avatar {
  position: relative;
  display: flex;
  flex: 0 0 30px;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  min-width: 30px;
  min-height: 30px;
  overflow: hidden;
  border-radius: 50%;
  color: var(--snapshot-primary-text);
  background: var(--snapshot-code-background);
  font-size: 10px;
  font-weight: 600;
}

.message-image-snapshot__avatar img {
  position: absolute;
  inset: 0;
  width: 30px;
  height: 30px;
  max-width: 30px;
  max-height: 30px;
  object-fit: cover;
}

.message-image-snapshot__main {
  flex: 1 1 auto;
  min-width: 0;
}

.message-image-snapshot__header {
  margin-bottom: 2px;
  line-height: 1.35;
}

.message-image-snapshot__name {
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-image-snapshot__whisper {
  margin: 0 0 2px;
  color: var(--snapshot-secondary-text);
  font-size: 10px;
}

.message-image-snapshot__entries {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.message-image-snapshot__entry {
  padding: 1px 0;
  margin: 0;
  border: 0;
  background: transparent;
  line-height: 1.48;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.message-image-snapshot__entry--ooc,
.message-image-snapshot__entry--archived {
  padding-left: 7px;
  border-left: 1px solid var(--snapshot-subtle-border);
}

.message-image-snapshot__entry--archived {
  opacity: 0.72;
}

.message-image-snapshot__tone {
  display: inline-block;
  margin-right: 5px;
  color: var(--snapshot-secondary-text);
  font-size: 10px;
  font-weight: 600;
  vertical-align: 1px;
}

.message-image-snapshot__content {
  display: inline;
  letter-spacing: 0;
}

.message-image-snapshot__content :deep(p),
.message-image-snapshot__content :deep(span),
.message-image-snapshot__content :deep(strong),
.message-image-snapshot__content :deep(em),
.message-image-snapshot__content :deep(a),
.message-image-snapshot__content :deep(code),
.message-image-snapshot__content :deep(blockquote) {
  letter-spacing: inherit;
}

.message-image-snapshot__content :deep(p),
.message-image-snapshot__content :deep(ul),
.message-image-snapshot__content :deep(ol),
.message-image-snapshot__content :deep(blockquote),
.message-image-snapshot__content :deep(pre),
.message-image-snapshot__content :deep(h1),
.message-image-snapshot__content :deep(h2),
.message-image-snapshot__content :deep(h3),
.message-image-snapshot__content :deep(h4),
.message-image-snapshot__content :deep(h5),
.message-image-snapshot__content :deep(h6) {
  margin: 0;
}

.message-image-snapshot__content :deep(ul),
.message-image-snapshot__content :deep(ol) {
  padding-left: 20px;
}

.message-image-snapshot__content :deep(blockquote) {
  padding-left: 9px;
  border-left: 2px solid var(--snapshot-subtle-border);
  color: var(--snapshot-secondary-text);
}

.message-image-snapshot__content :deep(code),
.message-image-snapshot__content :deep(pre) {
  background: var(--snapshot-code-background);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.message-image-snapshot__content :deep(code) {
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 0.9em;
}

.message-image-snapshot__content :deep(pre) {
  margin: 3px 0;
  padding: 8px;
  overflow: hidden;
  white-space: pre-wrap;
}

.message-image-snapshot__content :deep(pre code) {
  padding: 0;
}

.message-image-snapshot__content :deep(a) {
  color: var(--snapshot-link);
  text-decoration: underline;
}

.message-image-snapshot__content :deep(img) {
  display: block;
  width: auto;
  max-width: 100%;
  max-height: 360px;
  margin: 4px 0;
  object-fit: contain;
}

.message-image-snapshot__content :deep(.mention-capsule) {
  color: var(--snapshot-link);
  font-weight: 500;
}
</style>
