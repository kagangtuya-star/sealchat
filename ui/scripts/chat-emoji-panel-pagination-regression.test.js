import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const chatPath = resolve(scriptDir, '../src/views/chat/chat.vue');
const source = readFileSync(chatPath, 'utf8');

test('chat emoji panel uses sentinel-based pagination', () => {
  assert.match(source, /const emojiPanelContentRef = ref<HTMLElement \| null>\(null\);/, 'missing emoji panel content ref');
  assert.match(source, /const emojiPanelLoadMoreSentinelRef = ref<HTMLElement \| null>\(null\);/, 'missing emoji panel sentinel ref');
  assert.match(source, /useIntersectionObserver\(/, 'missing emoji panel observer');
});

test('chat emoji panel can append more collection pages', () => {
  assert.match(source, /const loadMoreEmojiPanelItems = async \(\) =>/, 'missing emoji panel load-more helper');
  assert.match(source, /await gallery\.loadItems\(tabId,\s*\{[\s\S]*append:\s*true/, 'missing append page request');
  assert.match(source, /const tabId = activeEmojiTab\.value;/, 'missing active tab guard');
});

test('chat emoji panel auto-fills short content', () => {
  assert.match(source, /const maybeLoadMoreEmojiPanelForShortContent = async \(\) =>/, 'missing short-content auto fill helper');
  assert.match(source, /scrollHeight <= container\.scrollHeight \+ 40|scrollHeight <= container\.clientHeight \+ 40/, 'missing short-content fill check');
  assert.match(source, /emojiPanelAutoFillPending/, 'missing auto fill guard');
});
