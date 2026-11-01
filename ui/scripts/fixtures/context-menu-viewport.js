import { createApp, reactive, computed, nextTick } from 'vue';
import { ContextMenu, ContextMenuItem, ContextMenuGroup } from '@imengyu/vue3-context-menu';
import '@imengyu/vue3-context-menu/lib/vue3-context-menu.css';
import '../../src/assets/overlays.scss';

const state = reactive({ show: false, hasImage: false, extra: 0, night: false,
  optionsComponent: { x: 0, y: 0, minWidth: 230, closeWhenScroll: false, zIndex: 3 },
});
const display = reactive({ palette: 'day' });
let archiveClicks = 0;
const values = {
  chat: { messageMenu: state },
  contextMenuOptions: new Function('computed', 'chat', 'display', window.menuOptionsSource + '\nreturn contextMenuOptions;')(computed, { messageMenu: state }, display),
  favoriteMenu: { available: true, worldVisible: true, worldLabel: '收藏消息', personalLabel: '收藏到个人' },
  insertTargetMenuLabel: '新消息插到其上', t: () => '发送悄悄话', extraItems: computed(() => state.extra),
};
for (const name of ['canAddToClueBox','canWhisper','canSetMessageInsertTarget','canPinByRule','showArchiveAction','canEdit','canRemoveMessage','canMultiNpcRepeat','canArchiveMessagesBefore']) values[name] = true;
for (const name of ['canUnpinByRule','showUnarchiveAction','isSelfMessage']) values[name] = false;
for (const name of ['handleQuickReaction','openFullEmojiPicker','addToMyEmoji','clickCopy','clickCopyMessageLink','clickForward','clickFavorite','clickAddToClueBox','clickWhisper','clickReplyTo','clickToggleMessageInsertTarget','clickPin','clickUnpin','clickArchive','clickUnarchive','clickEdit','clickResizeImage','clickMultiSelect','clickDelete','clickRemove','clickMultiNpcRepeat','clickJumpToChannelStart']) values[name] = () => {};
values.clickArchiveMessagesBefore = () => archiveClicks++;
createApp({ template: window.menuTemplate, setup: () => values,
  components: { ContextMenu, ContextMenuItem, ContextMenuGroup,
    ReactionQuickPicker: { template: '<div style="height:45px">👍 ❤️ 😆 😮 😢 😡 ＋</div>' },
  },
}).mount('#app');

const settle = async () => { await nextTick(); await new Promise(resolve => setTimeout(resolve, 100)); };
const item = text => [...document.querySelectorAll('.mx-context-menu-item .label')].find(el => el.textContent === text);
const fullyVisible = el => { const r = el.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth; };
const report = { userAgent: navigator.userAgent, viewport: { width: innerWidth, height: innerHeight }, checks: [] };
const check = (ok, name) => { report.checks.push({ name, passed: !!ok }); if (!ok) throw new Error(name); };
async function open(extra, night) {
  state.show = false; await settle();
  Object.assign(state, { extra, night, show: true });
  Object.assign(state.optionsComponent, { x: innerWidth - 20, y: innerHeight - 20 });
  display.palette = night ? 'night' : 'day';
  document.documentElement.style.cssText = night
    ? '--sc-bg-elevated:#29292e;--sc-bg-hover:#39393e;--sc-text-primary:#eee;--sc-text-secondary:#bbb;--sc-font-family:Arial,sans-serif'
    : '--sc-bg-elevated:#fff;--sc-bg-hover:#eee;--sc-text-primary:#333;--sc-text-secondary:#666;--sc-font-family:Arial,sans-serif';
  await settle();
}
async function showMore() {
  const more = item('更多');
  check(more && fullyVisible(more), 'More is reachable inside the viewport');
  more.closest('.mx-context-menu-item').dispatchEvent(new MouseEvent('mouseenter'));
  await settle();
  const last = item('归档该消息前的所有消息');
  check(last && fullyVisible(last), 'The last submenu label is completely visible');
  const surface = last.closest('.mx-context-menu');
  check(surface.classList.contains('sc-message-context-menu'), 'Submenus retain the public theme surface hook');
  check(getComputedStyle(surface).color === (state.night ? 'rgb(238, 238, 238)' : 'rgb(51, 51, 51)'), 'Day/night surface tokens remain effective');
  const r = last.getBoundingClientRect(), hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
  check(last.closest('.mx-context-menu-item').contains(hit), 'The last action passes pointer hit testing');
  last.closest('.mx-context-menu-item').click(); await settle();
}
(async () => {
  for (const night of [false, true]) { await open(0, night); await showMore(); }
  await open(8, false);
  const root = document.querySelector('.mx-context-menu');
  for (let i = 0; i < 6 && !fullyVisible(item('更多')); i++) {
    root.dispatchEvent(new WheelEvent('wheel', { deltaY: 2000, bubbles: true, cancelable: true }));
    await settle();
  }
  check(fullyVisible(item('更多')), 'A long menu scrolls to its final group');
  await showMore();
  check(archiveClicks === 3, 'Original item callbacks still execute exactly once per click');
  report.passed = true;
})().catch(error => { report.passed = false; report.error = error.message; }).finally(() => {
  document.getElementById('result').textContent = JSON.stringify(report);
});
