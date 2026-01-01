<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { zhCN, dateZhCN, jaJP, dateJaJP } from 'naive-ui'
import { darkTheme } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import type { GlobalTheme, GlobalThemeOverrides } from 'naive-ui'
import { i18n } from './lang'
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import dayjs from 'dayjs'
import { useDisplayStore } from '@/stores/display'

const display = useDisplayStore()

const naiveTheme = computed<GlobalTheme | null>(() => (display.settings.palette === 'night' ? darkTheme : null))

const themeOverrides = computed<GlobalThemeOverrides>(() => {
  const isNight = display.settings.palette === 'night'
  return {
    common: {
      primaryColor: '#3388de',
      primaryColorHover: '#3388de',
      primaryColorPressed: '#3859b3',
      textColor: isNight ? '#f4f4f5' : '#0f172a',
      textColor2: isNight ? 'rgba(248, 250, 252, 0.8)' : '#475569',
      textColor3: isNight ? 'rgba(248, 250, 252, 0.65)' : '#475569',
      bodyColor: isNight ? '#1b1b20' : '#ffffff',
    },
    Button: {},
  }
})

const locale = ref<any>(zhCN);
const dateLocale = ref<any>(dateZhCN);

watch(i18n.global.locale, (newVal) => {
  dayjs.locale(newVal);

  switch (newVal) {
    case 'en':
      locale.value = null;
      dateLocale.value = null;
      break;
    case 'zh-cn':
      locale.value = zhCN;
      dateLocale.value = dateZhCN;
      break;
    case 'ja':
      locale.value = jaJP;
      dateLocale.value = dateJaJP;
      break;
  }
})

const handleContextMenu = (e: MouseEvent) => {
  if (display.settings.disableContextMenu) {
    e.preventDefault()
  }
}

onMounted(() => {
  document.addEventListener('contextmenu', handleContextMenu)
})

onUnmounted(() => {
  document.removeEventListener('contextmenu', handleContextMenu)
})
</script>

<template>
  <n-config-provider :locale="locale" :date-locale="dateLocale" :theme="naiveTheme" :theme-overrides="themeOverrides" style="height: 100%;">
    <n-message-provider>
      <n-dialog-provider>
        <RouterView />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
header {
  line-height: 1.5;
  max-height: 100vh;
}

.logo {
  display: block;
  margin: 0 auto 2rem;
}

nav {
  width: 100%;
  font-size: 12px;
  text-align: center;
  margin-top: 2rem;
}

nav a.router-link-exact-active {
  color: var(--color-text);
}

nav a.router-link-exact-active:hover {
  background-color: transparent;
}

nav a {
  display: inline-block;
  padding: 0 1rem;
  border-left: 1px solid var(--color-border);
}

nav a:first-of-type {
  border: 0;
}

@media (min-width: 1024px) {
  header {
    display: flex;
    place-items: center;
    padding-right: calc(var(--section-gap) / 2);
  }

  .logo {
    margin: 0 2rem 0 0;
  }

  header .wrapper {
    display: flex;
    place-items: flex-start;
    flex-wrap: wrap;
  }

  nav {
    text-align: left;
    margin-left: -1rem;
    font-size: 1rem;

    padding: 1rem 0;
    margin-top: 1rem;
  }
}
</style>

<!-- Global unscoped styles for custom theme override -->
<style>
/* ==========================================================================
   CUSTOM THEME GLOBAL OVERRIDES
   When custom theme is active (data-custom-theme='true' on :root),
   these styles ensure ALL UI components use custom theme colors.
   ========================================================================== */

/* Base root text color */
:root[data-custom-theme='true'] {
  color: var(--sc-text-primary);
}

/* --------------------------------------------------------------------------
   BACKGROUNDS - Surfaces, Cards, Panels
   -------------------------------------------------------------------------- */

/* Main surfaces */
:root[data-custom-theme='true'] body,
:root[data-custom-theme='true'] .chat,
:root[data-custom-theme='true'] .world-panel,
:root[data-custom-theme='true'] .channel-list,
:root[data-custom-theme='true'] .sidebar,
:root[data-custom-theme='true'] .sc-sidebar,
:root[data-custom-theme='true'] .panel,
:root[data-custom-theme='true'] .view-container {
  background-color: var(--sc-bg-surface) !important;
}

/* Elevated surfaces */
:root[data-custom-theme='true'] .n-card,
:root[data-custom-theme='true'] .n-modal,
:root[data-custom-theme='true'] .n-drawer,
:root[data-custom-theme='true'] .n-drawer-content,
:root[data-custom-theme='true'] .n-popover,
:root[data-custom-theme='true'] .n-tooltip,
:root[data-custom-theme='true'] .n-dialog,
:root[data-custom-theme='true'] .n-message,
:root[data-custom-theme='true'] .n-notification {
  --n-color: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Header bar */
:root[data-custom-theme='true'] .sc-header,
:root[data-custom-theme='true'] .header,
:root[data-custom-theme='true'] .app-header,
:root[data-custom-theme='true'] .toolbar-header {
  background-color: var(--sc-bg-header) !important;
}

/* --------------------------------------------------------------------------
   NAIVE UI COMPONENTS - Comprehensive Coverage
   -------------------------------------------------------------------------- */

/* Dropdown menus */
:root[data-custom-theme='true'] .n-dropdown-menu,
:root[data-custom-theme='true'] .n-dropdown,
:root[data-custom-theme='true'] .n-dropdown-option,
:root[data-custom-theme='true'] .n-base-select-menu,
:root[data-custom-theme='true'] .n-base-select-option {
  --n-color: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Tabs */
:root[data-custom-theme='true'] .n-tabs,
:root[data-custom-theme='true'] .n-tabs-nav,
:root[data-custom-theme='true'] .n-tabs-wrapper,
:root[data-custom-theme='true'] .n-tabs-tab-wrapper,
:root[data-custom-theme='true'] .n-tab-pane {
  background-color: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .n-tabs-tab {
  background-color: transparent !important;
}

/* Collapse panels */
:root[data-custom-theme='true'] .n-collapse,
:root[data-custom-theme='true'] .n-collapse-item,
:root[data-custom-theme='true'] .n-collapse-item__header,
:root[data-custom-theme='true'] .n-collapse-item__content-wrapper,
:root[data-custom-theme='true'] .n-collapse-item__content-inner {
  background-color: var(--sc-bg-surface) !important;
}

/* Lists */
:root[data-custom-theme='true'] .n-list,
:root[data-custom-theme='true'] .n-list-item,
:root[data-custom-theme='true'] .n-thing {
  background-color: var(--sc-bg-surface) !important;
}

/* Menu */
:root[data-custom-theme='true'] .n-menu,
:root[data-custom-theme='true'] .n-menu-item,
:root[data-custom-theme='true'] .n-menu-item-content,
:root[data-custom-theme='true'] .n-submenu,
:root[data-custom-theme='true'] .n-submenu-children {
  --n-color: var(--sc-bg-surface) !important;
  background-color: var(--sc-bg-surface) !important;
}

/* Tree */
:root[data-custom-theme='true'] .n-tree,
:root[data-custom-theme='true'] .n-tree-node,
:root[data-custom-theme='true'] .n-tree-node-content {
  background-color: transparent !important;
}

/* Tooltip and popover content */
:root[data-custom-theme='true'] .n-tooltip .n-tooltip__content,
:root[data-custom-theme='true'] .n-popover .n-popover__content,
:root[data-custom-theme='true'] .n-popover__content {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* --------------------------------------------------------------------------
   INPUTS AND FORM CONTROLS
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] .n-input,
:root[data-custom-theme='true'] .n-input__input-el,
:root[data-custom-theme='true'] .n-input__textarea-el,
:root[data-custom-theme='true'] .n-input-wrapper,
:root[data-custom-theme='true'] .n-base-selection,
:root[data-custom-theme='true'] .n-select,
:root[data-custom-theme='true'] textarea {
  --n-color: var(--sc-bg-input) !important;
  background-color: var(--sc-bg-input) !important;
}

/* --------------------------------------------------------------------------
   BUTTONS
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] .n-button--default-type:not(.n-button--disabled) {
  --n-color: var(--sc-bg-surface) !important;
  background-color: var(--sc-bg-surface) !important;
  border-color: var(--sc-border-mute) !important;
}

/* --------------------------------------------------------------------------
   BORDERS
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] .n-card,
:root[data-custom-theme='true'] .n-input,
:root[data-custom-theme='true'] .n-select,
:root[data-custom-theme='true'] .n-collapse-item,
:root[data-custom-theme='true'] .n-divider,
:root[data-custom-theme='true'] .sc-header {
  border-color: var(--sc-border-mute) !important;
}

/* --------------------------------------------------------------------------
   TEXT COLORS
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] .n-text,
:root[data-custom-theme='true'] .n-h1,
:root[data-custom-theme='true'] .n-h2,
:root[data-custom-theme='true'] .n-h3,
:root[data-custom-theme='true'] .n-h4,
:root[data-custom-theme='true'] .n-h5,
:root[data-custom-theme='true'] .n-h6,
:root[data-custom-theme='true'] .n-p {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-text--secondary {
  color: var(--sc-text-secondary) !important;
}

/* --------------------------------------------------------------------------
   CHAT MESSAGE SPECIFIC
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] .message-row__surface--tone-ic {
  background-color: var(--custom-chat-ic-bg, var(--chat-ic-bg)) !important;
}

:root[data-custom-theme='true'] .message-row__surface--tone-ooc {
  background-color: var(--custom-chat-ooc-bg, var(--chat-ooc-bg)) !important;
}

/* --------------------------------------------------------------------------
   SCROLLBAR (optional visual enhancement)
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] ::-webkit-scrollbar-track {
  background: var(--sc-bg-surface);
}

:root[data-custom-theme='true'] ::-webkit-scrollbar-thumb {
  background: var(--sc-border-strong);
}

/* --------------------------------------------------------------------------
   CSS VARIABLE FALLBACK OVERRIDE
   Force CSS variables to use :root values even when fallbacks are specified
   -------------------------------------------------------------------------- */

:root[data-custom-theme='true'] {
  --sc-bg-surface: var(--sc-bg-surface);
  --sc-bg-elevated: var(--sc-bg-elevated);
  --sc-bg-input: var(--sc-bg-input);
  --sc-bg-header: var(--sc-bg-header);
  --sc-text-primary: var(--sc-text-primary);
  --sc-text-secondary: var(--sc-text-secondary);
  --sc-border-mute: var(--sc-border-mute);
  --sc-border-strong: var(--sc-border-strong);
}

/* --------------------------------------------------------------------------
   DEEP NAIVE UI INTERNAL VARIABLE OVERRIDES
   These target Naive UI's inline CSS variable system
   -------------------------------------------------------------------------- */

/* Modal deep overrides */
:root[data-custom-theme='true'] .n-modal .n-card,
:root[data-custom-theme='true'] .n-modal .n-card__content,
:root[data-custom-theme='true'] .n-modal .n-card-header,
:root[data-custom-theme='true'] .n-modal .n-card-header__main,
:root[data-custom-theme='true'] .n-card__content,
:root[data-custom-theme='true'] .n-card-header {
  --n-color: var(--sc-bg-elevated) !important;
  --n-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Dialog deep overrides */
:root[data-custom-theme='true'] .n-dialog,
:root[data-custom-theme='true'] .n-dialog__content,
:root[data-custom-theme='true'] .n-dialog .n-dialog__title {
  --n-color: var(--sc-bg-elevated) !important;
  --n-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Popover deep overrides */
:root[data-custom-theme='true'] .n-popover-shared,
:root[data-custom-theme='true'] .n-popover-shared .n-popover-arrow-wrapper,
:root[data-custom-theme='true'] [class*="n-popover"] {
  --n-color: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Button comprehensive overrides */
:root[data-custom-theme='true'] .n-button {
  --n-border: 1px solid var(--sc-border-mute) !important;
  --n-text-color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-button--default-type {
  --n-color: var(--sc-bg-surface) !important;
  --n-color-hover: var(--sc-bg-elevated) !important;
  --n-color-pressed: var(--sc-bg-elevated) !important;
  --n-color-focus: var(--sc-bg-elevated) !important;
  --n-text-color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-button--tertiary-type,
:root[data-custom-theme='true'] .n-button--quaternary-type {
  --n-color: transparent !important;
  --n-color-hover: var(--sc-bg-elevated) !important;
  --n-text-color: var(--sc-text-primary) !important;
}

/* Input comprehensive overrides */
:root[data-custom-theme='true'] .n-input {
  --n-color: var(--sc-bg-input) !important;
  --n-color-focus: var(--sc-bg-input) !important;
  --n-text-color: var(--sc-text-primary) !important;
  --n-placeholder-color: var(--sc-text-secondary) !important;
  --n-border: 1px solid var(--sc-border-mute) !important;
  --n-border-hover: 1px solid var(--sc-border-strong) !important;
}

/* Select comprehensive overrides */
:root[data-custom-theme='true'] .n-base-selection,
:root[data-custom-theme='true'] .n-base-selection .n-base-selection-label {
  --n-color: var(--sc-bg-input) !important;
  --n-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-input) !important;
}

:root[data-custom-theme='true'] .n-base-select-menu {
  --n-color: var(--sc-bg-elevated) !important;
  --n-option-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Dropdown comprehensive overrides */
:root[data-custom-theme='true'] .n-dropdown-menu {
  --n-color: var(--sc-bg-elevated) !important;
  --n-option-color-hover: rgba(0, 0, 0, 0.05) !important;
  --n-option-text-color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-dropdown-option-body {
  --n-option-text-color: var(--sc-text-primary) !important;
  color: var(--sc-text-primary) !important;
}

/* Tag overrides */
:root[data-custom-theme='true'] .n-tag--default-type {
  --n-color: var(--sc-bg-elevated) !important;
  --n-text-color: var(--sc-text-primary) !important;
  --n-border: 1px solid var(--sc-border-mute) !important;
}

/* Switch overrides */
:root[data-custom-theme='true'] .n-switch {
  --n-rail-color: var(--sc-border-mute) !important;
}

/* Radio/Checkbox overrides */
:root[data-custom-theme='true'] .n-radio,
:root[data-custom-theme='true'] .n-checkbox {
  --n-text-color: var(--sc-text-primary) !important;
}

/* Slider overrides */
:root[data-custom-theme='true'] .n-slider {
  --n-rail-color: var(--sc-border-mute) !important;
}

/* Divider overrides */
:root[data-custom-theme='true'] .n-divider {
  --n-color: var(--sc-border-mute) !important;
}

/* Data table overrides */
:root[data-custom-theme='true'] .n-data-table,
:root[data-custom-theme='true'] .n-data-table-th,
:root[data-custom-theme='true'] .n-data-table-td {
  --n-th-color: var(--sc-bg-elevated) !important;
  --n-td-color: var(--sc-bg-surface) !important;
  --n-text-color: var(--sc-text-primary) !important;
  --n-border-color: var(--sc-border-mute) !important;
}

/* Drawer deep overrides */
:root[data-custom-theme='true'] .n-drawer,
:root[data-custom-theme='true'] .n-drawer-content,
:root[data-custom-theme='true'] .n-drawer-body-content-wrapper {
  --n-color: var(--sc-bg-elevated) !important;
  --n-body-color: var(--sc-bg-elevated) !important;
  --n-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Form item overrides */
:root[data-custom-theme='true'] .n-form-item-label {
  --n-label-text-color: var(--sc-text-primary) !important;
  color: var(--sc-text-primary) !important;
}

/* Empty state overrides */
:root[data-custom-theme='true'] .n-empty {
  --n-text-color: var(--sc-text-secondary) !important;
}

/* Badge overrides */
:root[data-custom-theme='true'] .n-badge {
  --n-color: var(--primary-color, #3388de) !important;
}

/* Pagination overrides */
:root[data-custom-theme='true'] .n-pagination {
  --n-item-color: var(--sc-bg-surface) !important;
  --n-item-text-color: var(--sc-text-primary) !important;
  --n-button-color: var(--sc-bg-surface) !important;
}

/* Loading / Spin overrides */
:root[data-custom-theme='true'] .n-spin-container {
  --n-color: var(--sc-bg-surface) !important;
}

/* Tooltip arrow fix */
:root[data-custom-theme='true'] .n-tooltip .n-tooltip__arrow {
  background-color: var(--sc-bg-elevated) !important;
}

/* Scrollbar for Naive UI scrollable areas */
:root[data-custom-theme='true'] .n-scrollbar-rail,
:root[data-custom-theme='true'] .n-scrollbar-content {
  --n-scrollbar-color: var(--sc-border-strong) !important;
}

/* Ultimate fallback: any element with background-color white in custom theme mode */
:root[data-custom-theme='true'] [style*="background-color: rgb(255, 255, 255)"],
:root[data-custom-theme='true'] [style*="background-color:#fff"],
:root[data-custom-theme='true'] [style*="background-color: #fff"],
:root[data-custom-theme='true'] [style*="background-color:#ffffff"],
:root[data-custom-theme='true'] [style*="background-color: #ffffff"] {
  background-color: var(--sc-bg-elevated) !important;
}

/* --------------------------------------------------------------------------
   ADDITIONAL MISSING COMPONENTS
   -------------------------------------------------------------------------- */

/* HTML and Body backgrounds */
:root[data-custom-theme='true'],
:root[data-custom-theme='true'] html,
:root[data-custom-theme='true'] body,
:root[data-custom-theme='true'] #app {
  background-color: var(--sc-bg-surface) !important;
}

/* Radio button groups (used for tabs like 频道/私聊 and layout/theme selectors) */
:root[data-custom-theme='true'] .n-radio-group,
:root[data-custom-theme='true'] .n-radio-button-group {
  --n-button-color: var(--sc-bg-surface) !important;
  --n-button-color-active: var(--sc-bg-elevated) !important;
  --n-button-text-color: var(--sc-text-primary) !important;
  --n-button-border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .n-radio-button,
:root[data-custom-theme='true'] .n-radio__label {
  --n-color: var(--sc-bg-surface) !important;
  --n-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-radio-button--checked {
  --n-color: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Button groups */
:root[data-custom-theme='true'] .n-button-group .n-button {
  --n-color: var(--sc-bg-surface) !important;
  background-color: var(--sc-bg-surface) !important;
  border-color: var(--sc-border-mute) !important;
}

/* Tabs bar backgrounds - more specific */
:root[data-custom-theme='true'] .n-tabs-bar,
:root[data-custom-theme='true'] .n-tabs-rail {
  background-color: transparent !important;
}

:root[data-custom-theme='true'] .n-tabs-tab-pad,
:root[data-custom-theme='true'] .n-tabs-scroll-padding {
  background-color: var(--sc-bg-surface) !important;
}

/* Segmented control / button style tabs */
:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-rail,
:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-tab {
  background-color: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-tab--active {
  background-color: var(--sc-bg-elevated) !important;
}

/* Card style tabs */
:root[data-custom-theme='true'] .n-tabs--card-type .n-tabs-tab {
  background-color: var(--sc-bg-surface) !important;
  border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .n-tabs--card-type .n-tabs-tab--active {
  background-color: var(--sc-bg-elevated) !important;
}

/* Channel favorites area */
:root[data-custom-theme='true'] .favorite-channels,
:root[data-custom-theme='true'] .channel-favorites,
:root[data-custom-theme='true'] .sc-favorites {
  background-color: var(--sc-bg-surface) !important;
}

/* Any Naive UI component with --n-color css variable */
:root[data-custom-theme='true'] [class*="n-"][style*="--n-color"] {
  --n-color: var(--sc-bg-elevated) !important;
}

/* Ensure all popconfirm dialogs use custom theme */
:root[data-custom-theme='true'] .n-popconfirm,
:root[data-custom-theme='true'] .n-popconfirm__body {
  --n-color: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* Alert component */
:root[data-custom-theme='true'] .n-alert {
  --n-color: var(--sc-bg-elevated) !important;
}

/* Steps component */
:root[data-custom-theme='true'] .n-steps,
:root[data-custom-theme='true'] .n-step {
  --n-indicator-color: var(--sc-bg-surface) !important;
}

/* Timeline component */
:root[data-custom-theme='true'] .n-timeline,
:root[data-custom-theme='true'] .n-timeline-item {
  --n-color: var(--sc-bg-surface) !important;
}

/* Upload component */
:root[data-custom-theme='true'] .n-upload,
:root[data-custom-theme='true'] .n-upload-trigger {
  --n-color: var(--sc-bg-surface) !important;
  background-color: var(--sc-bg-surface) !important;
}

/* Avatar component background */
:root[data-custom-theme='true'] .n-avatar {
  --n-color: var(--sc-bg-elevated) !important;
}

/* Result/Empty states */
:root[data-custom-theme='true'] .n-result {
  --n-color: var(--sc-bg-surface) !important;
}

/* Affix component */
:root[data-custom-theme='true'] .n-affix {
  background-color: var(--sc-bg-surface) !important;
}

/* Back to top */
:root[data-custom-theme='true'] .n-back-top {
  --n-color: var(--sc-bg-elevated) !important;
}

/* Breadcrumb */
:root[data-custom-theme='true'] .n-breadcrumb {
  --n-item-text-color: var(--sc-text-primary) !important;
}

/* Calendar */
:root[data-custom-theme='true'] .n-calendar {
  --n-color: var(--sc-bg-surface) !important;
}

/* Carousel */
:root[data-custom-theme='true'] .n-carousel {
  --n-color: var(--sc-bg-surface) !important;
}

/* Countdown */
:root[data-custom-theme='true'] .n-countdown {
  --n-text-color: var(--sc-text-primary) !important;
}

/* Image preview */
:root[data-custom-theme='true'] .n-image-preview-toolbar {
  background-color: var(--sc-bg-elevated) !important;
}

/* Transfer component */
:root[data-custom-theme='true'] .n-transfer {
  --n-color: var(--sc-bg-surface) !important;
}

/* Watermark - transparent */
:root[data-custom-theme='true'] .n-watermark {
  background-color: transparent !important;
}

/* Mega fallback: force ALL Naive components to respect custom bg */
:root[data-custom-theme='true'] .n-config-provider {
  --n-body-color: var(--sc-bg-surface) !important;
}

/* --------------------------------------------------------------------------
   USER-IDENTIFIED SPECIFIC CLASSES
   -------------------------------------------------------------------------- */

/* Chat search panel */
:root[data-custom-theme='true'] .chat-search-panel {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Tabs tab wrapper - inner wrapper */
:root[data-custom-theme='true'] .n-tabs-tab-wrapper {
  background-color: var(--sc-bg-surface) !important;
}

/* Export entry */
:root[data-custom-theme='true'] .export-entry {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
}

/* N-table component */
:root[data-custom-theme='true'] .n-table,
:root[data-custom-theme='true'] .n-table--bordered,
:root[data-custom-theme='true'] .n-table--bottom-bordered,
:root[data-custom-theme='true'] .n-table th,
:root[data-custom-theme='true'] .n-table td,
:root[data-custom-theme='true'] .n-table thead,
:root[data-custom-theme='true'] .n-table tbody {
  --n-th-color: var(--sc-bg-elevated) !important;
  --n-td-color: var(--sc-bg-surface) !important;
  --n-border-color: var(--sc-border-mute) !important;
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .n-table th {
  background-color: var(--sc-bg-elevated) !important;
}

/* Online badge */
:root[data-custom-theme='true'] .online-badge {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Sider item (sidebar navigation) */
:root[data-custom-theme='true'] .sider-item {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .sider-item.active,
:root[data-custom-theme='true'] .sider-item:hover {
  background-color: var(--sc-bg-elevated) !important;
}

/* Chat search panel results */
:root[data-custom-theme='true'] .chat-search-panel__results {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* History mode hint */
:root[data-custom-theme='true'] .history-mode-hint {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Audio drawer and player */
:root[data-custom-theme='true'] .audio-drawer,
:root[data-custom-theme='true'] .audio-drawer__player,
:root[data-custom-theme='true'] .audio-drawer__player * {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .audio-drawer__player input,
:root[data-custom-theme='true'] .audio-drawer__player select,
:root[data-custom-theme='true'] .audio-drawer__player .n-input,
:root[data-custom-theme='true'] .audio-drawer__player .n-select {
  background-color: var(--sc-bg-input) !important;
}

/* Active tabs - comprehensive */
:root[data-custom-theme='true'] .n-tabs-tab--active,
:root[data-custom-theme='true'] .n-tabs-tab.n-tabs-tab--active,
:root[data-custom-theme='true'] .n-tabs-tab--active.sc-sidebar-fill,
:root[data-custom-theme='true'] .sc-sidebar-fill.n-tabs-tab--active {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Inactive tabs */
:root[data-custom-theme='true'] .n-tabs-tab:not(.n-tabs-tab--active) {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-secondary) !important;
}

/* SC sidebar fill class */
:root[data-custom-theme='true'] .sc-sidebar-fill {
  background-color: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .sc-sidebar-fill.active,
:root[data-custom-theme='true'] .sc-sidebar-fill:hover {
  background-color: var(--sc-bg-elevated) !important;
}

/* N-Card action area */
:root[data-custom-theme='true'] .n-card__action {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-mute) !important;
}

/* Dice tray columns */
:root[data-custom-theme='true'] .dice-tray,
:root[data-custom-theme='true'] .dice-tray__column,
:root[data-custom-theme='true'] .dice-tray__column--quick,
:root[data-custom-theme='true'] .dice-tray__column--form {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .dice-tray input,
:root[data-custom-theme='true'] .dice-tray .n-input,
:root[data-custom-theme='true'] .dice-tray .n-select,
:root[data-custom-theme='true'] .dice-tray__column input {
  background-color: var(--sc-bg-input) !important;
}

/* TipTap rich text editor - comprehensive */
:root[data-custom-theme='true'] .tiptap-wrapper,
:root[data-custom-theme='true'] .tiptap-wrapper *,
:root[data-custom-theme='true'] .tiptap-editor,
:root[data-custom-theme='true'] .tiptap-editor-wrapper,
:root[data-custom-theme='true'] .tiptap-content,
:root[data-custom-theme='true'] .ProseMirror,
:root[data-custom-theme='true'] .tiptap {
  background-color: var(--sc-bg-input) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .tiptap-toolbar,
:root[data-custom-theme='true'] .tiptap-menubar,
:root[data-custom-theme='true'] .tiptap-wrapper .toolbar,
:root[data-custom-theme='true'] .editor-toolbar {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .tiptap-toolbar button,
:root[data-custom-theme='true'] .tiptap-menubar button,
:root[data-custom-theme='true'] .editor-toolbar button {
  background-color: transparent !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .tiptap-toolbar button:hover,
:root[data-custom-theme='true'] .tiptap-toolbar button.is-active {
  background-color: var(--sc-bg-surface) !important;
}

/* TipTap placeholder */
:root[data-custom-theme='true'] .tiptap p.is-editor-empty:first-child::before {
  color: var(--sc-text-secondary) !important;
}

/* TipTap selection and focus */
:root[data-custom-theme='true'] .ProseMirror-focused {
  border-color: var(--sc-border-strong) !important;
}

/* TipTap bubble menu */
:root[data-custom-theme='true'] .tippy-box,
:root[data-custom-theme='true'] .tippy-content {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Upload dragger */
:root[data-custom-theme='true'] .n-upload-dragger {
  background-color: var(--sc-bg-surface) !important;
  border-color: var(--sc-border-mute) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-upload-dragger:hover {
  border-color: var(--sc-border-strong) !important;
}

/* Dice tray quick buttons */
:root[data-custom-theme='true'] .dice-tray__quick-btn {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .dice-tray__quick-btn:hover {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-strong) !important;
}

/* Dice tray history card */
:root[data-custom-theme='true'] .dice-tray__history-card {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

/* Keyword tooltip */
:root[data-custom-theme='true'] .keyword-tooltip,
:root[data-custom-theme='true'] .keyword-tooltip--hover {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

/* Keyword tooltip scrollbar - minimal/invisible design for custom theme */
/* Firefox */
:root[data-custom-theme='true'] .keyword-tooltip:hover {
  scrollbar-color: rgba(128, 128, 128, 0.25) transparent !important;
}

/* WebKit */
:root[data-custom-theme='true'] .keyword-tooltip:hover::-webkit-scrollbar-thumb {
  background: rgba(128, 128, 128, 0.25) !important;
}

/* Dice tray macro key buttons */
:root[data-custom-theme='true'] .dice-tray__macro-key {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .dice-tray__macro-key:hover {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-strong) !important;
}

/* --------------------------------------------------------------------------
   KEYWORD HIGHLIGHT CUSTOM THEME OVERRIDES
   Uses custom CSS variables for full theming support
   -------------------------------------------------------------------------- */

/* Define custom keyword highlight variables when custom theme is active */
:root[data-custom-theme='true'] {
  --keyword-bg: var(--custom-keyword-bg, rgba(180, 140, 60, 0.35));
  --keyword-bg-hover: var(--custom-keyword-bg-hover, rgba(180, 140, 60, 0.5));
  --keyword-border-color: var(--custom-keyword-border, rgba(220, 180, 80, 0.7));
  --keyword-text-color: var(--custom-keyword-text, var(--sc-text-primary));
  --keyword-underline-bg-hover: var(--custom-keyword-underline-bg-hover, rgba(180, 140, 60, 0.25));
}

/* Override keyword highlight styles in custom theme mode */
:root[data-custom-theme='true'] .keyword-highlight:not(.keyword-highlight--underline) {
  background: var(--keyword-bg) !important;
  border-bottom-color: var(--keyword-border-color) !important;
  color: var(--keyword-text-color) !important;
}

:root[data-custom-theme='true'] .keyword-highlight:not(.keyword-highlight--underline):hover {
  background: var(--keyword-bg-hover) !important;
}

:root[data-custom-theme='true'] .keyword-highlight.keyword-highlight--underline {
  background: transparent !important;
  border-bottom-color: var(--keyword-border-color) !important;
  color: inherit !important;
}

:root[data-custom-theme='true'] .keyword-highlight.keyword-highlight--underline:hover {
  background: var(--keyword-underline-bg-hover) !important;
}

/* Keyword tooltip body in custom theme */
:root[data-custom-theme='true'] .keyword-tooltip__body {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

:root[data-custom-theme='true'] .keyword-tooltip__body .keyword-highlight:not(.keyword-highlight--underline) {
  background: var(--keyword-bg) !important;
  border-bottom-color: var(--keyword-border-color) !important;
}

:root[data-custom-theme='true'] .keyword-tooltip__body .keyword-highlight.keyword-highlight--underline {
  background: transparent !important;
  border-bottom-color: var(--keyword-border-color) !important;
}

/* 术语气泡多段首行缩进样式 - 全局 */
.keyword-tooltip__body--indented .keyword-tooltip__paragraph {
  text-indent: var(--keyword-tooltip-text-indent, 0);
  margin: 0;
  padding: 0;
}

.keyword-tooltip__body--indented .keyword-tooltip__paragraph + .keyword-tooltip__paragraph {
  margin-top: 0.5em;
}

/* Keyword Tooltip Image Styles */
.keyword-tooltip__image {
  max-width: 120px;
  max-height: 80px;
  object-fit: contain;
  border-radius: 4px;
  cursor: pointer;
  transition: opacity 0.15s ease, transform 0.15s ease;
  display: inline-block;
  vertical-align: middle;
  margin: 4px 2px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.12);
}

.keyword-tooltip__image:hover {
  opacity: 0.85;
  transform: scale(1.02);
}

/* Night mode image styles */
[data-display-palette='night'] .keyword-tooltip__image,
:root[data-display-palette='night'] .keyword-tooltip__image {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.35);
}

/* Custom theme image styles */
:root[data-custom-theme='true'] .keyword-tooltip__image {
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.15);
}

/* Active tabs with sc-sidebar-fill - more specific selectors */
:root[data-custom-theme='true'] .n-tabs-tab.n-tabs-tab--active.sc-sidebar-fill,
:root[data-custom-theme='true'] .n-tabs-tab--active.sc-sidebar-fill,
:root[data-custom-theme='true'] .sc-sidebar-fill.n-tabs-tab--active,
:root[data-custom-theme='true'] .n-tabs .n-tabs-tab--active {
  --n-tab-color-active: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Export entry warning */
:root[data-custom-theme='true'] .export-entry__warning {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Chat search panel filter bar */
:root[data-custom-theme='true'] .chat-search-panel__filter-bar {
  background-color: var(--sc-bg-surface) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}

/* N-base-selection-tags (multi-select tags area) */
:root[data-custom-theme='true'] .n-base-selection-tags {
  background-color: var(--sc-bg-input) !important;
}

/* All dropdown/select menus - comprehensive */
:root[data-custom-theme='true'] .n-base-select-menu,
:root[data-custom-theme='true'] .n-base-select-option,
:root[data-custom-theme='true'] .n-base-select-group-header,
:root[data-custom-theme='true'] .v-binder-follower-content,
:root[data-custom-theme='true'] .n-select-menu {
  --n-color: var(--sc-bg-elevated) !important;
  --n-option-color-pending: rgba(0, 0, 0, 0.05) !important;
  --n-option-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-base-select-option--selected,
:root[data-custom-theme='true'] .n-base-select-option--pending {
  background-color: rgba(0, 0, 0, 0.08) !important;
}

/* Auto-complete dropdown */
:root[data-custom-theme='true'] .n-auto-complete-menu {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Cascader dropdown */
:root[data-custom-theme='true'] .n-cascader-menu {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Time/Date picker dropdowns */
:root[data-custom-theme='true'] .n-date-panel,
:root[data-custom-theme='true'] .n-time-picker-panel {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* Color picker popup */
:root[data-custom-theme='true'] .n-color-picker-panel {
  background-color: var(--sc-bg-elevated) !important;
}

/* --------------------------------------------------------------------------
   NAIVEUI SEGMENT TABS - AGGRESSIVE INLINE STYLE OVERRIDE
   Naive UI sets these as inline styles, so we need to override with CSS vars
   -------------------------------------------------------------------------- */

/* Target segment tabs specifically */
:root[data-custom-theme='true'] .n-tabs--segment-type {
  --n-color-segment: var(--sc-bg-surface) !important;
  --n-tab-color: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-rail {
  background-color: var(--sc-bg-surface) !important;
  --n-color-segment: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-tab {
  --n-tab-text-color: var(--sc-text-secondary) !important;
  --n-tab-text-color-active: var(--sc-text-primary) !important;
  --n-tab-text-color-hover: var(--sc-text-primary) !important;
  color: var(--sc-text-secondary) !important;
}

:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-tab--active {
  --n-tab-color: var(--sc-bg-elevated) !important;
  --n-tab-text-color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

/* The segment tab capsule/background */
:root[data-custom-theme='true'] .n-tabs--segment-type .n-tabs-capsule {
  background-color: var(--sc-bg-elevated) !important;
}

/* --------------------------------------------------------------------------
   NAIVEUI SELECT OPTIONS - INLINE STYLE OVERRIDE
   -------------------------------------------------------------------------- */

/* Target select option backgrounds */
:root[data-custom-theme='true'] .n-base-select-option {
  --n-option-color-active: var(--sc-bg-elevated) !important;
  --n-option-color-pending: rgba(128, 128, 128, 0.15) !important;
  --n-option-text-color: var(--sc-text-primary) !important;
  --n-option-text-color-active: var(--sc-text-primary) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-base-select-option--selected {
  --n-option-color-active: var(--sc-bg-elevated) !important;
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .n-base-select-option--pending {
  background-color: rgba(128, 128, 128, 0.12) !important;
}

/* The check icon in selected options */
:root[data-custom-theme='true'] .n-base-select-option__check {
  color: var(--primary-color, #3388de) !important;
}

/* Force all inline backgrounds in Naive components */
:root[data-custom-theme='true'] [class*="n-"][style*="--n-color"] {
  --n-color: var(--sc-bg-elevated) !important;
}

:root[data-custom-theme='true'] [class*="n-tabs"][style*="--n-color-segment"] {
  --n-color-segment: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] [class*="n-tabs"][style*="--n-tab-color"] {
  --n-tab-color: var(--sc-bg-elevated) !important;
}

/* ==========================================================================
   CHAT INPUT TOOLBAR - 输入栏工具区
   ========================================================================== */

:root[data-custom-theme='true'] .chat-input-actions,
:root[data-custom-theme='true'] .input-floating-toolbar {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .input-floating-toolbar .n-button:not([disabled]) .n-icon,
:root[data-custom-theme='true'] .input-floating-toolbar .n-button:not([disabled]) .n-button__icon > svg,
:root[data-custom-theme='true'] .input-floating-toolbar .n-button:not([disabled]) .n-button__icon,
:root[data-custom-theme='true'] .chat-input-actions .n-button:not([disabled]) .n-icon,
:root[data-custom-theme='true'] .chat-input-actions .n-button:not([disabled]) .n-button__icon > svg,
:root[data-custom-theme='true'] .chat-input-actions .n-button:not([disabled]) .n-button__icon {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .chat-input-actions__icon {
  color: var(--sc-text-primary) !important;
}

/* Dice settings trigger */
:root[data-custom-theme='true'] .dice-tray-settings-trigger {
  color: var(--sc-text-secondary) !important;
}

:root[data-custom-theme='true'] .dice-tray-settings-trigger--active {
  color: var(--primary-color, #3388de) !important;
  border-color: var(--sc-border-strong) !important;
  background-color: var(--sc-bg-elevated) !important;
}

/* ==========================================================================
   HISTORY ENTRY - 历史记录条目
   ========================================================================== */

:root[data-custom-theme='true'] .history-entry {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-mute) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .history-entry:hover {
  border-color: var(--primary-color-hover, var(--sc-border-strong)) !important;
  background-color: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .history-entry__preview {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .history-entry__meta,
:root[data-custom-theme='true'] .history-entry__time {
  color: var(--sc-text-secondary) !important;
}

/* ==========================================================================
   MESSAGE ACTION BAR - 消息操作栏
   ========================================================================== */

:root[data-custom-theme='true'] .message-action-bar__btn {
  color: var(--sc-text-secondary) !important;
  background-color: var(--sc-bg-elevated) !important;
}

:root[data-custom-theme='true'] .message-action-bar__btn:hover {
  color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-surface) !important;
}

/* ==========================================================================
   EDITING PREVIEW BUBBLE - 编辑预览气泡
   ========================================================================== */

:root[data-custom-theme='true'] .editing-preview__bubble {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .editing-preview__bubble[data-tone='ic'] {
  background-color: var(--custom-chat-ic-bg, var(--sc-bg-elevated)) !important;
}

:root[data-custom-theme='true'] .editing-preview__bubble[data-tone='ooc'] {
  background-color: var(--custom-chat-ooc-bg, var(--sc-bg-elevated)) !important;
}

/* ==========================================================================
   TYPING PREVIEW BUBBLE - 输入预览气泡
   ========================================================================== */

:root[data-custom-theme='true'] .typing-preview-bubble {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .typing-preview-bubble[data-tone='ic'] {
  background-color: var(--custom-chat-ic-bg, var(--sc-bg-elevated)) !important;
}

:root[data-custom-theme='true'] .typing-preview-bubble[data-tone='ooc'] {
  background-color: var(--custom-chat-ooc-bg, var(--sc-bg-elevated)) !important;
}

/* ==========================================================================
   SELECTION FLOATING BAR - 选择浮动栏
   ========================================================================== */

:root[data-custom-theme='true'] .selection-floating-bar {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-strong) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .selection-floating-bar__button {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .selection-floating-bar__button:hover {
  background-color: var(--sc-bg-surface) !important;
}

/* ==========================================================================
   DICE CHIP - 骰子芯片
   ========================================================================== */

:root[data-custom-theme='true'] .dice-chip {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-mute) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .dice-chip--preview {
  background-color: var(--sc-bg-surface) !important;
}

:root[data-custom-theme='true'] .dice-chip--tone-ic:not(.dice-chip--preview),
:root[data-custom-theme='true'] [data-dice-tone='ic']:not(.dice-chip--preview) {
  background-color: var(--custom-chat-ic-bg, var(--sc-bg-elevated)) !important;
}

:root[data-custom-theme='true'] .dice-chip--tone-ooc:not(.dice-chip--preview),
:root[data-custom-theme='true'] [data-dice-tone='ooc']:not(.dice-chip--preview) {
  background-color: var(--custom-chat-ooc-bg, var(--sc-bg-elevated)) !important;
}

/* ==========================================================================
   USER PRESENCE POPOVER - 用户在线状态
   ========================================================================== */

:root[data-custom-theme='true'] .presence-popover {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .presence-name {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .presence-meta,
:root[data-custom-theme='true'] .presence-empty {
  color: var(--sc-text-secondary) !important;
}

/* ==========================================================================
   CHANNEL SETTINGS - 频道设置
   ========================================================================== */

:root[data-custom-theme='true'] .role-title {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .role-desc {
  color: var(--sc-text-secondary) !important;
}

/* ==========================================================================
   KEYWORD MOBILE ROW - 术语管理移动端行
   ========================================================================== */

:root[data-custom-theme='true'] .keyword-mobile-simple-row {
  color: var(--sc-text-primary) !important;
  background-color: var(--sc-bg-surface) !important;
}

/* ==========================================================================
   CHAT DICE BUTTON - 聊天骰子按钮
   ========================================================================== */

:root[data-custom-theme='true'] .chat-dice-button {
  color: var(--sc-text-primary) !important;
}

/* ==========================================================================
   HISTORY PANEL - 历史记录面板
   ========================================================================== */

:root[data-custom-theme='true'] .history-panel {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .history-panel__title {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .history-panel__empty {
  color: var(--sc-text-secondary) !important;
  background-color: var(--sc-bg-surface) !important;
}

/* ==========================================================================
   SCROLL BOTTOM BUTTON - 滚动到底部按钮
   ========================================================================== */

:root[data-custom-theme='true'] .scroll-bottom-button {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-strong) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .scroll-bottom-button:hover {
  background-color: var(--sc-bg-surface) !important;
}

/* ==========================================================================
   DICE SETTINGS PANEL - 骰子设置面板
   ========================================================================== */

:root[data-custom-theme='true'] .dice-settings-panel {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .dice-settings-panel__section {
  background-color: var(--sc-bg-elevated) !important;
  border-color: var(--sc-border-strong) !important;
}

:root[data-custom-theme='true'] .dice-settings-panel__title {
  color: var(--sc-text-primary) !important;
}

:root[data-custom-theme='true'] .dice-settings-panel__desc,
:root[data-custom-theme='true'] .dice-settings-panel__hint {
  color: var(--sc-text-secondary) !important;
}

/* ==========================================================================
   HISTORY POPOVER - 历史记录弹出框
   ========================================================================== */

:root[data-custom-theme='true'] .history-popover .n-popover__content {
  background-color: var(--sc-bg-elevated) !important;
  color: var(--sc-text-primary) !important;
  border-color: var(--sc-border-mute) !important;
}
</style>
