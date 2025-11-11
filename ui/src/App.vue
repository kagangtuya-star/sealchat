<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { zhCN, dateZhCN, jaJP, dateJaJP } from 'naive-ui'
import { darkTheme } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import type { GlobalTheme, GlobalThemeOverrides } from 'naive-ui'
import { i18n } from './lang'
import { ref, watch } from 'vue'
import dayjs from 'dayjs'

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#3388de',
    primaryColorHover: '#3388de',
    primaryColorPressed: '#3859b3',
  },
  Button: {
    // textColor: '#FF0000'
  }
}

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
</script>

<template>
  <div class="app-shell">
    <n-config-provider :locale="locale" :date-locale="dateLocale" :theme-overrides="themeOverrides" style="height: 100%;">
      <n-message-provider>
        <n-dialog-provider>
          <RouterView />
        </n-dialog-provider>
      </n-message-provider>
    </n-config-provider>
  </div>
</template>

<style scoped>
.app-shell {
  position: relative;
  height: 100%;
}

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
