<script setup lang="ts">
const roles = [
  {
    name: 'world-owner',
    label: '拥有者',
    permissions: ['管理世界信息', '邀请/移除成员', '管理角色'],
  },
  {
    name: 'world-admin',
    label: '管理员',
    permissions: ['邀请成员', '管理频道', '查看审核'],
  },
  {
    name: 'world-member',
    label: '成员',
    permissions: ['访问公开频道', '参与讨论'],
  },
  {
    name: 'world-guest',
    label: '访客',
    permissions: ['只读频道', '等待审批'],
  },
];
</script>

<template>
  <div class="grid md:grid-cols-2 gap-4">
    <div v-for="role in roles" :key="role.name" class="border rounded-xl p-4 shadow-sm bg-white/80 dark:bg-slate-800/70">
      <div class="text-sm text-gray-500 uppercase tracking-wide">{{ role.name }}</div>
      <div class="text-xl font-semibold mb-2">{{ role.label }}</div>
      <ul class="text-sm text-gray-600 dark:text-gray-300 list-disc pl-5 space-y-1">
        <li v-for="item in role.permissions" :key="item">{{ item }}</li>
      </ul>
    </div>
  </div>
</template>
