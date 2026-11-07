<script setup lang="ts">
import router from '@/router';
import { defineComponent, ref, reactive, onMounted } from 'vue';
import type { FormRules, FormItemRule, FormItemInst, FormInst } from 'naive-ui';
import { useMessage } from 'naive-ui';
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';
import type { ServerConfig } from '@/types';

const message = useMessage()

const formRef = ref<FormInst | null>(null)

interface ModelType {
  password: string | null
  passwordNew: string | null
  passwordNew2: string | null
}

const model = ref<ModelType>({
  password: null,
  passwordNew: null,
  passwordNew2: null
})

function validatePasswordStartWith(
  rule: FormItemRule,
  value: string
): boolean {
  return (
    !!model.value.passwordNew &&
    model.value.passwordNew.startsWith(value) &&
    model.value.passwordNew.length >= value.length
  )
}

function validatePasswordSame(rule: FormItemRule, value: string): boolean {
  return value === model.value.passwordNew
}

const handleValidateButtonClick = async (e: MouseEvent) => {
  e.preventDefault()
  formRef.value?.validate(async (errors) => {
    if (!errors) {
      try {
        const resp = await userStore.changePassword({
          password: model.value.password || '',
          passwordNew: model.value.passwordNew || '',
        });
        const ret = resp.data;
        message.success('验证成功，返回首页。所有旧设备登录将失效')
        if (ret.token) {
          // setTimeout(() => {
          router.replace({ name: 'home' })
          // }, 3000);
        }
      } catch (err) {
        console.log(err)
        message.error('修改密码失败: ' + ((err as any).response?.data?.message || '未知错误'))
      }
    } else {
      console.log(errors)
      message.error('验证失败')
    }
  })
}


const rules: FormRules = {
  password: [
    {
      required: true,
      message: '请输入密码'
    }
  ],
  passwordNew: [
    {
      required: true,
      message: '请输入密码'
    }
  ],
  passwordNew2: [
    {
      required: true,
      message: '请输入新密码',
      trigger: ['input', 'blur']
    },
    {
      validator: validatePasswordSame,
      message: '两次密码输入不一致',
      trigger: ['blur', 'password-input']
    }
  ]
}

const rPasswordFormItemRef = ref<FormItemInst | null>(null)

const handlePasswordInput = () => {
  if (model.value.passwordNew) {
    rPasswordFormItemRef.value?.validate({ trigger: 'password-input' })
  }
}

const userStore = useUserStore();

const utils = useUtilsStore();
const config = ref<ServerConfig | null>(utils.config)

const back = async () => {
  router.back();
}

onMounted(async () => {
  const resp = await utils.configGet();
  config.value = resp.data;
})
</script>

<template>
  <div class="flex h-full w-full justify-center items-center">
    <div class="w-[50%] flex items-center justify-center flex-col" style="min-width: 20rem;">
      <h2 class="font-bold text-xl mb-8">修改密码</h2>

      <n-form ref="formRef" :model="model" :rules="rules" class="w-full px-8 max-w-md">
        <n-form-item path="password" label="当前密码">
          <n-input v-model:value="model.password" type="password" @input="handlePasswordInput" @keydown.enter.prevent />
        </n-form-item>

        <n-form-item path="passwordNew" label="新密码">
          <n-input v-model:value="model.passwordNew" type="password" @input="handlePasswordInput"
            @keydown.enter.prevent />
        </n-form-item>

        <n-form-item path="passwordNew2" label="重复新密码">
          <n-input v-model:value="model.passwordNew2" type="password" @input="handlePasswordInput"
            @keydown.enter.prevent />
        </n-form-item>

        <!-- <n-form-item ref="rPasswordFormItemRef" first path="reenteredPassword" label="重复密码">
          <n-input v-model:value="model.reenteredPassword" :disabled="!model.password" type="password"
            @keydown.enter.prevent />
        </n-form-item> -->

        <n-row :gutter="[0, 24]">
          <n-col :span="24">
            <div class=" flex justify-between">
              <router-link :to="{ name: 'user-signup' }">
                <n-button text v-if="config?.registerOpen">注册</n-button>
              </router-link>

              <div class="space-x-2">
                <n-button :disabled="model.password === ''" round @click="back">
                  返回
                </n-button>

                <n-button :disabled="model.password === ''" round type="primary" @click="handleValidateButtonClick">
                  修改密码
                </n-button>
              </div>
            </div>
          </n-col>
        </n-row>
      </n-form>

    </div>
  </div>
</template>
  
<style>
.sign-bg {
  background-size: cover;
  background-position: center;
}
</style>
