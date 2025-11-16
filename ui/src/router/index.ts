import { createRouter, createWebHashHistory } from 'vue-router';
import HomeView from '@/views/HomeView.vue';
import UserSigninVue from '@/views/user/sign-in-view.vue';
import UserSignupVue from '@/views/user/sign-up-view.vue';
import UserPasswordResetView from '@/views/user/password-reset-view.vue';
import WorldHall from '@/views/world/WorldHall.vue';
import WorldDetail from '@/views/world/WorldDetail.vue';
import WorldCreate from '@/views/world/WorldCreate.vue';
import WorldManage from '@/views/world/WorldManage.vue';
import WorldInvite from '@/views/world/WorldInvite.vue';

const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView,
    },
    {
      path: '/user/signin',
      name: 'user-signin',
      component: UserSigninVue,
    },
    {
      path: '/user/signup',
      name: 'user-signup',
      component: UserSignupVue,
    },
    {
      path: '/user/password-reset',
      name: 'user-password-reset',
      component: UserPasswordResetView,
    },
    {
      path: '/worlds',
      name: 'world-hall',
      component: WorldHall,
    },
    {
      path: '/worlds/new',
      name: 'world-create',
      component: WorldCreate,
    },
    {
      path: '/worlds/:slug',
      name: 'world-detail',
      component: WorldDetail,
    },
    {
      path: '/worlds/:slug/manage',
      name: 'world-manage',
      component: WorldManage,
    },
    {
      path: '/invite/:code',
      name: 'world-invite',
      component: WorldInvite,
    },
    {
      path: '/about',
      name: 'about',
      component: () => import('@/views/AboutView.vue'),
    },
  ],
});

export default router;
