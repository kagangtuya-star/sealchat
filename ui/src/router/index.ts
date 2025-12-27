import { createRouter, createWebHashHistory, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'
import UserSigninVue from '@/views/user/sign-in-view.vue'
import UserSignupVue from '@/views/user/sign-up-view.vue'
import UserPasswordResetView from '@/views/user/password-reset-view.vue'
import WorldLobby from '@/views/world/WorldLobby.vue'
import WorldDetail from '@/views/world/WorldDetail.vue'
import InviteConsume from '@/views/invite/InviteConsume.vue'
import StatusDashboard from '@/views/status/StatusDashboard.vue'


const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView
    },
    {
      path: '/split',
      name: 'split',
      component: () => import('@/views/split/SplitView.vue'),
    },
    {
      path: '/embed',
      name: 'embed',
      component: () => import('@/views/embed/EmbedChatView.vue'),
    },
    {
      path: '/user/signin',
      name: 'user-signin',
      component: UserSigninVue
    },
    {
      path: '/user/signup',
      name: 'user-signup',
      component: UserSignupVue
    },
    {
      path: '/user/password-reset',
      name: 'user-password-reset',
      component: UserPasswordResetView
    },
    {
      path: '/about',
      name: 'about',
      // route level code-splitting
      // this generates a separate chunk (About.[hash].js) for this route
      // which is lazy-loaded when the route is visited.
      component: () => import('../views/AboutView.vue')
    },
    {
      path: '/worlds',
      name: 'world-lobby',
      component: WorldLobby,
    },
    {
      path: '/worlds/:worldId',
      name: 'world-detail',
      component: WorldDetail,
    },
    {
      path: '/invite/:slug',
      name: 'invite-consume',
      component: InviteConsume,
    },
    {
      path: '/status',
      name: 'status',
      component: StatusDashboard,
    },
    {
      path: '/:worldId/:channelId?',
      name: 'world-channel',
      component: HomeView,
    }
  ]
})

export default router
