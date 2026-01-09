<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useMessage, type FormInst, type FormRules, type UploadFileInfo } from 'naive-ui';
import { useUtilsStore } from '@/stores/utils';
import { useUserStore } from '@/stores/user';

const props = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
  (e: 'success'): void;
}>();

const message = useMessage();
const utils = useUtilsStore();
const user = useUserStore();

const activeTab = ref<'single' | 'batch'>('single');
const loading = ref(false);

// Single user form
const formRef = ref<FormInst | null>(null);
const formData = ref({
  username: '',
  nickname: '',
  password: '',
  confirmPassword: '',
  roleIds: ['sys-user'] as string[],
  disabled: false,
});

// Username validation
const usernameChecking = ref(false);
const usernameAvailable = ref<boolean | null>(null);
let usernameCheckTimer: any = null;

const checkUsername = async (username: string) => {
  if (!username || username.length < 2) {
    usernameAvailable.value = null;
    return;
  }
  usernameChecking.value = true;
  try {
    const result = await utils.adminCheckUsername(username);
    usernameAvailable.value = result.available;
  } catch {
    usernameAvailable.value = null;
  } finally {
    usernameChecking.value = false;
  }
};

watch(() => formData.value.username, (val) => {
  usernameAvailable.value = null;
  if (usernameCheckTimer) clearTimeout(usernameCheckTimer);
  usernameCheckTimer = setTimeout(() => checkUsername(val), 500);
});

// Password strength
const passwordStrength = computed(() => {
  const pwd = formData.value.password;
  if (!pwd) return { level: 0, text: '', color: '' };

  let score = 0;
  if (pwd.length >= 6) score++;
  if (pwd.length >= 10) score++;
  if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) score++;
  if (/[0-9]/.test(pwd)) score++;
  if (/[^A-Za-z0-9]/.test(pwd)) score++;

  if (score <= 2) return { level: 1, text: '弱', color: '#f56c6c' };
  if (score <= 3) return { level: 2, text: '中', color: '#e6a23c' };
  return { level: 3, text: '强', color: '#67c23a' };
});

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 2, max: 32, message: '用户名长度为2-32位', trigger: 'blur' },
    { pattern: /^[A-Za-z0-9_.\-]+$/, message: '只能包含字母、数字、下划线、点或中划线', trigger: 'blur' },
  ],
  nickname: [
    { required: true, message: '请输入昵称', trigger: 'blur' },
    { max: 20, message: '昵称不能超过20个字符', trigger: 'blur' },
    { pattern: /^[^\s]+$/, message: '昵称不能包含空格', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码长度至少6位', trigger: 'blur' },
    { pattern: /[A-Za-z]/, message: '密码必须包含字母', trigger: 'blur' },
    { pattern: /[0-9]/, message: '密码必须包含数字', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请确认密码', trigger: 'blur' },
    {
      validator: (_rule, value) => {
        return value === formData.value.password;
      },
      message: '两次输入密码不一致',
      trigger: 'blur',
    },
  ],
  roleIds: [
    { type: 'array', required: true, message: '请选择角色', trigger: 'change' },
  ],
};

const roleOptions = [
  { label: '管理员', value: 'sys-admin' },
  { label: '普通用户', value: 'sys-user' },
];

const handleSingleSubmit = async () => {
  try {
    await formRef.value?.validate();
    if (usernameAvailable.value === false) {
      message.error('用户名已被占用');
      return;
    }
    loading.value = true;
    await utils.adminUserCreate({
      username: formData.value.username,
      nickname: formData.value.nickname,
      password: formData.value.password,
      roleIds: formData.value.roleIds,
      disabled: formData.value.disabled,
    });
    message.success('用户创建成功');
    resetForm();
    emit('success');
    emit('update:show', false);
  } catch (err: any) {
    const errMsg = err?.response?.data?.message || '创建失败';
    message.error(errMsg);
  } finally {
    loading.value = false;
  }
};

const resetForm = () => {
  formData.value = {
    username: '',
    nickname: '',
    password: '',
    confirmPassword: '',
    roleIds: ['sys-user'],
    disabled: false,
  };
  usernameAvailable.value = null;
  batchFile.value = null;
  batchResult.value = null;
};

// Batch import
const batchFile = ref<File | null>(null);
const batchResult = ref<{
  success: boolean;
  message: string;
  stats: { total: number; created: number; failed: number };
  errors: Array<{ row: number; username: string; error: string }>;
} | null>(null);

const handleFileChange = (options: { file: UploadFileInfo }) => {
  batchFile.value = options.file.file || null;
  batchResult.value = null;
};

const downloadTemplate = () => {
  const token = user.token;
  const url = utils.getImportTemplateUrl();
  const link = document.createElement('a');
  link.href = url + `?token=${encodeURIComponent(token || '')}`;
  link.download = 'user_import_template.csv';
  link.click();
};

const handleBatchSubmit = async () => {
  if (!batchFile.value) {
    message.warning('请先选择CSV文件');
    return;
  }
  loading.value = true;
  try {
    const resp = await utils.adminUserBatchCreate(batchFile.value);
    batchResult.value = resp.data;
    if (resp.data.success) {
      message.success(`成功导入 ${resp.data.stats.created} 个用户`);
      emit('success');
    } else {
      message.warning(`导入完成，${resp.data.stats.failed} 个失败`);
    }
  } catch (err: any) {
    message.error(err?.response?.data?.message || '导入失败');
  } finally {
    loading.value = false;
  }
};

const handleClose = () => {
  resetForm();
  emit('update:show', false);
};
</script>

<template>
  <n-modal
    :show="show"
    @update:show="handleClose"
    preset="card"
    title="新增用户"
    style="width: 540px; max-width: 95vw"
    :mask-closable="false"
  >
    <n-tabs v-model:value="activeTab" type="line">
      <n-tab-pane name="single" tab="单个创建">
        <n-form
          ref="formRef"
          :model="formData"
          :rules="rules"
          label-placement="left"
          label-width="80"
          require-mark-placement="right-hanging"
        >
          <n-form-item label="用户名" path="username">
            <n-input
              v-model:value="formData.username"
              placeholder="2-32位字母、数字、下划线"
              :status="usernameAvailable === false ? 'error' : undefined"
            />
            <template v-if="usernameChecking">
              <n-spin size="small" style="margin-left: 8px" />
            </template>
            <template v-else-if="usernameAvailable === true">
              <n-icon color="#67c23a" style="margin-left: 8px">
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>
                </svg>
              </n-icon>
            </template>
            <template v-else-if="usernameAvailable === false">
              <n-text type="error" style="margin-left: 8px; white-space: nowrap">已被占用</n-text>
            </template>
          </n-form-item>

          <n-form-item label="昵称" path="nickname">
            <n-input v-model:value="formData.nickname" placeholder="最多20个字符，不含空格" />
          </n-form-item>

          <n-form-item label="密码" path="password">
            <div style="width: 100%">
              <n-input
                v-model:value="formData.password"
                type="password"
                show-password-on="click"
                placeholder="至少6位，包含字母和数字"
              />
              <div v-if="formData.password" class="password-strength">
                <span>密码强度：</span>
                <div class="strength-bar">
                  <div
                    v-for="i in 3"
                    :key="i"
                    class="strength-segment"
                    :style="{
                      backgroundColor: i <= passwordStrength.level ? passwordStrength.color : '#e0e0e0'
                    }"
                  />
                </div>
                <span :style="{ color: passwordStrength.color }">{{ passwordStrength.text }}</span>
              </div>
            </div>
          </n-form-item>

          <n-form-item label="确认密码" path="confirmPassword">
            <n-input
              v-model:value="formData.confirmPassword"
              type="password"
              show-password-on="click"
              placeholder="再次输入密码"
            />
          </n-form-item>

          <n-form-item label="角色" path="roleIds">
            <n-select
              v-model:value="formData.roleIds"
              :options="roleOptions"
              multiple
              placeholder="选择用户角色"
            />
          </n-form-item>

          <n-form-item label="状态">
            <n-switch v-model:value="formData.disabled" :unchecked-value="true" :checked-value="false">
              <template #checked>启用</template>
              <template #unchecked>禁用</template>
            </n-switch>
          </n-form-item>
        </n-form>

        <div class="modal-footer">
          <n-button @click="handleClose">取消</n-button>
          <n-button type="primary" :loading="loading" @click="handleSingleSubmit">
            创建用户
          </n-button>
        </div>
      </n-tab-pane>

      <n-tab-pane name="batch" tab="批量导入">
        <div class="batch-import">
          <n-alert type="info" style="margin-bottom: 16px">
            <template #header>导入说明</template>
            <ul style="margin: 0; padding-left: 16px; line-height: 1.8">
              <li>支持CSV格式，最大2MB，最多500行</li>
              <li>必须包含 username, nickname, password 列</li>
              <li>已存在的用户名将被跳过</li>
            </ul>
          </n-alert>

          <n-space vertical :size="12">
            <n-button text type="primary" @click="downloadTemplate">
              下载CSV模板
            </n-button>

            <n-upload
              accept=".csv"
              :max="1"
              :default-upload="false"
              @change="handleFileChange"
            >
              <n-button>选择CSV文件</n-button>
            </n-upload>

            <template v-if="batchResult">
              <n-divider />
              <n-space :size="24">
                <n-statistic label="总计" :value="batchResult.stats.total" />
                <n-statistic label="成功" :value="batchResult.stats.created">
                  <template #suffix>
                    <n-text type="success">个</n-text>
                  </template>
                </n-statistic>
                <n-statistic label="失败" :value="batchResult.stats.failed">
                  <template #suffix>
                    <n-text type="error">个</n-text>
                  </template>
                </n-statistic>
              </n-space>

              <n-collapse v-if="batchResult.errors.length > 0">
                <n-collapse-item title="失败详情" name="errors">
                  <n-data-table
                    :columns="[
                      { title: '行', key: 'row', width: 60 },
                      { title: '用户名', key: 'username', width: 120 },
                      { title: '错误', key: 'error' },
                    ]"
                    :data="batchResult.errors"
                    size="small"
                    :max-height="200"
                  />
                </n-collapse-item>
              </n-collapse>
            </template>
          </n-space>
        </div>

        <div class="modal-footer">
          <n-button @click="handleClose">关闭</n-button>
          <n-button
            type="primary"
            :loading="loading"
            :disabled="!batchFile"
            @click="handleBatchSubmit"
          >
            开始导入
          </n-button>
        </div>
      </n-tab-pane>
    </n-tabs>
  </n-modal>
</template>

<style scoped>
.password-strength {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  font-size: 12px;
}

.strength-bar {
  display: flex;
  gap: 4px;
}

.strength-segment {
  width: 24px;
  height: 4px;
  border-radius: 2px;
  transition: background-color 0.2s;
}

.batch-import {
  min-height: 200px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--n-border-color);
}
</style>
