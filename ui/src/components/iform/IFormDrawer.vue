<template>
  <n-drawer
    :show="iform.drawerVisible"
    placement="right"
    :width="drawerWidth"
    :mask-closable="true"
    :close-on-esc="true"
    @update:show="iform.toggleDrawer"
    class="iform-drawer"
  >
    <n-drawer-content>
      <template #header>
        <div class="iform-drawer__title">
          <n-button v-if="isMobileLayout" size="tiny" quaternary @click="iform.closeDrawer()">
            返回
          </n-button>
          <span>频道嵌入窗</span>
        </div>
      </template>
      <div class="iform-drawer__header">
        <div>
          <p class="iform-drawer__subtitle">可嵌入网页/工具并同步给频道成员</p>
          <div class="iform-drawer__badges">
            <n-tag size="small" type="info">{{ forms.length }} 个控件</n-tag>
            <n-tag size="small" v-if="!iform.canManage" type="warning">只读模式</n-tag>
          </div>
        </div>
        <n-button quaternary size="small" @click="refresh">刷新</n-button>
      </div>

      <n-space vertical size="medium">
        <div class="iform-toolbar">
          <n-button type="primary" size="small" :disabled="!iform.canManage" @click="openFormModal()">
            新增控件
          </n-button>
          <n-button size="small" :disabled="!iform.canBroadcast || !iform.selectedFormIds.length" @click="pushSelected">
            推送选中
          </n-button>
          <n-button size="small" tertiary :disabled="!iform.canManage || !forms.length" @click="migrationModalVisible = true">
            迁移/复制
          </n-button>
        </div>

        <n-alert v-if="!iform.canManage" type="info" closable>
          你当前没有管理权限，仅可查看与打开控件。
        </n-alert>

        <n-spin :show="iform.loading">
          <template v-if="forms.length">
            <div class="iform-card" v-for="form in forms" :key="form.id">
              <div class="iform-card__header">
                <div class="iform-card__title">
                  <n-checkbox
                    :disabled="!iform.canBroadcast"
                    :checked="iform.selectedFormIds.includes(form.id)"
                    @update:checked="iform.toggleSelection(form.id)"
                  />
                  <div>
                    <strong>{{ form.name || '未命名控件' }}</strong>
                    <p class="iform-card__meta">
                      默认 {{ form.defaultWidth }} × {{ form.defaultHeight }} · {{ form.defaultCollapsed ? '折叠' : '展开' }} ·
                      {{ form.defaultFloating ? '弹出' : '面板' }}
                    </p>
                  </div>
                </div>
                <div class="iform-card__actions">
                  <n-button quaternary size="tiny" @click="iform.openPanel(form.id)">面板</n-button>
                  <n-button quaternary size="tiny" @click="openFloating(form.id)">弹出</n-button>
                  <n-button quaternary size="tiny" :disabled="!iform.canBroadcast" @click="pushSingle(form)">推送</n-button>
                  <n-button quaternary size="tiny" :disabled="!iform.canManage" @click="openFormModal(form)">编辑</n-button>
                  <n-button quaternary size="tiny" :disabled="!iform.canManage" @click="confirmDelete(form)">
                    <template #icon>
                      <n-icon :component="TrashOutline" />
                    </template>
                  </n-button>
                </div>
              </div>
              <div class="iform-card__body">
                <div class="iform-card__field">
                  <span>访问方式：</span>
                  <n-tag size="small" type="info">{{ form.url ? 'URL' : '嵌入代码' }}</n-tag>
                  <n-tag v-if="form.mediaOptions?.autoPlay" size="small" type="success">自动播放</n-tag>
                  <n-tag v-if="form.mediaOptions?.autoUnmute" size="small" type="success">自动解除静音</n-tag>
                </div>
                <div class="iform-card__field">
                  <span>默认行为：</span>
                  <n-switch
                    size="small"
                    :disabled="!iform.canManage"
                    :value="form.defaultCollapsed"
                    @update:value="updateForm(form.id, { defaultCollapsed: $event })"
                  >
                    <template #checked>折叠</template>
                    <template #unchecked>展开</template>
                  </n-switch>
                  <n-switch
                    size="small"
                    :disabled="!iform.canManage"
                    :value="form.defaultFloating"
                    @update:value="updateForm(form.id, { defaultFloating: $event })"
                  >
                    <template #checked>弹出</template>
                    <template #unchecked>面板</template>
                  </n-switch>
                </div>
              </div>
            </div>
          </template>
          <n-empty v-else description="当前频道暂无嵌入控件" />
        </n-spin>
      </n-space>

      <n-modal v-model:show="formModalVisible" preset="dialog" :title="editingForm ? '编辑控件' : '新增控件'" :positive-text="editingForm ? '保存' : '创建'" negative-text="'取消'" @positive-click="handleSubmit" @negative-click="handleCancel">
        <n-form label-placement="left" label-width="72">
          <n-form-item label="名称" required>
            <n-input v-model:value="formModel.name" placeholder="示例：战斗地图" maxlength="64" />
          </n-form-item>
          <n-form-item label="URL">
            <n-input v-model:value="formModel.url" placeholder="https://example.com" />
          </n-form-item>
          <n-form-item label="嵌入代码">
            <n-input type="textarea" v-model:value="formModel.embedCode" placeholder="支持粘贴 <iframe> 代码" :rows="3" />
          </n-form-item>
          <n-form-item label="默认尺寸">
            <div class="iform-form__size">
              <n-input-number v-model:value="formModel.defaultWidth" :min="240" :max="1920" placeholder="宽" />
              <span>×</span>
              <n-input-number v-model:value="formModel.defaultHeight" :min="160" :max="1200" placeholder="高" />
            </div>
          </n-form-item>
          <n-form-item label="默认状态">
            <n-switch v-model:value="formModel.defaultCollapsed">
              <template #checked>折叠</template>
              <template #unchecked>展开</template>
            </n-switch>
            <n-switch v-model:value="formModel.defaultFloating">
              <template #checked>弹出</template>
              <template #unchecked>面板</template>
            </n-switch>
          </n-form-item>
          <n-form-item label="媒体优化">
            <n-switch v-model:value="formModel.mediaOptions.autoPlay">
              <template #checked>自动播放</template>
              <template #unchecked>手动播放</template>
            </n-switch>
            <n-switch v-model:value="formModel.mediaOptions.autoUnmute">
              <template #checked>自动解除静音</template>
              <template #unchecked>保持静音</template>
            </n-switch>
          </n-form-item>
        </n-form>
      </n-modal>

      <n-modal v-model:show="migrationModalVisible" preset="dialog" title="迁移到其他频道" positive-text="执行" negative-text="取消" @positive-click="handleMigration" @negative-click="() => (migrationModalVisible = false)">
        <n-form label-placement="left" label-width="72">
          <n-form-item label="目标频道" required>
            <n-select v-model:value="migrationTargets" multiple filterable :options="channelOptions" placeholder="选择一个或多个频道" />
          </n-form-item>
          <n-form-item label="模式" required>
            <n-radio-group v-model:value="migrationMode">
              <n-radio value="copy">复制</n-radio>
              <n-radio value="move">迁移</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item label="控件">
            <n-checkbox-group v-model:value="migrationFormIds">
              <n-space vertical>
                <n-checkbox value="@all">全部</n-checkbox>
                <n-checkbox v-for="form in forms" :key="form.id" :value="form.id">{{ form.name || form.id }}</n-checkbox>
              </n-space>
            </n-checkbox-group>
          </n-form-item>
        </n-form>
      </n-modal>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useWindowSize } from '@vueuse/core';
import { useIFormStore } from '@/stores/iform';
import { useChatStore } from '@/stores/chat';
import { useMessage, useDialog } from 'naive-ui';
import { TrashOutline } from '@vicons/ionicons5';
import type { ChannelIForm } from '@/types/iform';

const iform = useIFormStore();
const chat = useChatStore();
iform.bootstrap();

const message = useMessage();
const dialog = useDialog();

const forms = computed(() => [...iform.currentForms]);

const { width: viewportWidth } = useWindowSize();
const drawerWidth = computed(() => {
  if (!viewportWidth.value) return 420;
  return Math.min(480, viewportWidth.value < 640 ? viewportWidth.value : 420);
});
const isMobileLayout = computed(() => viewportWidth.value > 0 && viewportWidth.value < 640);

const formModalVisible = ref(false);
const editingForm = ref<ChannelIForm | null>(null);
const formModel = reactive({
  name: '',
  url: '',
  embedCode: '',
  defaultWidth: 640,
  defaultHeight: 360,
  defaultCollapsed: false,
  defaultFloating: false,
  mediaOptions: {
    autoPlay: false,
    autoUnmute: false,
  },
});

const migrationModalVisible = ref(false);
const migrationTargets = ref<string[]>([]);
const migrationMode = ref<'copy' | 'move'>('copy');
const migrationFormIds = ref<string[]>([]);

const channelOptions = computed(() => flattenChannels(chat.channelTree || [], chat.curChannel?.id));

function flattenChannels(tree: any[], excludeId?: string, depth = 0): Array<{ label: string; value: string }> {
  const result: Array<{ label: string; value: string }> = [];
  tree.forEach((node) => {
    if (!node?.id || node.id === excludeId) {
      return;
    }
    const indent = depth ? `${'· '.repeat(depth)}` : '';
    result.push({ label: `${indent}${node.name || node.id}`, value: node.id });
    if (node.children?.length) {
      result.push(...flattenChannels(node.children, excludeId, depth + 1));
    }
  });
  return result;
}

const resetFormModel = () => {
  editingForm.value = null;
  Object.assign(formModel, {
    name: '',
    url: '',
    embedCode: '',
    defaultWidth: 640,
    defaultHeight: 360,
    defaultCollapsed: false,
    defaultFloating: false,
    mediaOptions: {
      autoPlay: false,
      autoUnmute: false,
    },
  });
};

const openFormModal = (form?: ChannelIForm) => {
  if (!iform.canManage) {
    return;
  }
  if (form) {
    editingForm.value = form;
    Object.assign(formModel, {
      name: form.name,
      url: form.url || '',
      embedCode: form.embedCode || '',
      defaultWidth: form.defaultWidth || 640,
      defaultHeight: form.defaultHeight || 360,
      defaultCollapsed: !!form.defaultCollapsed,
      defaultFloating: !!form.defaultFloating,
      mediaOptions: {
        autoPlay: !!form.mediaOptions?.autoPlay,
        autoUnmute: !!form.mediaOptions?.autoUnmute,
      },
    });
  } else {
    resetFormModel();
  }
  formModalVisible.value = true;
};

const handleSubmit = async () => {
  if (!formModel.name.trim()) {
    message.warning('名称不能为空');
    return false;
  }
  if (!formModel.url.trim() && !formModel.embedCode.trim()) {
    message.warning('请至少填写 URL 或嵌入代码');
    return false;
  }
  try {
    if (editingForm.value) {
      await iform.updateForm(editingForm.value.id, {
        name: formModel.name.trim(),
        url: formModel.url.trim(),
        embedCode: formModel.embedCode.trim(),
        defaultWidth: formModel.defaultWidth,
        defaultHeight: formModel.defaultHeight,
        defaultCollapsed: formModel.defaultCollapsed,
        defaultFloating: formModel.defaultFloating,
        mediaOptions: formModel.mediaOptions,
      });
      message.success('控件已更新');
    } else {
      await iform.createForm({
        name: formModel.name.trim(),
        url: formModel.url.trim(),
        embedCode: formModel.embedCode.trim(),
        defaultWidth: formModel.defaultWidth,
        defaultHeight: formModel.defaultHeight,
        defaultCollapsed: formModel.defaultCollapsed,
        defaultFloating: formModel.defaultFloating,
        mediaOptions: formModel.mediaOptions,
      });
      message.success('控件已创建');
    }
    formModalVisible.value = false;
    resetFormModel();
    return true;
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存失败');
    return false;
  }
};

const handleCancel = () => {
  resetFormModel();
  return true;
};

const updateForm = async (formId: string, payload: Record<string, unknown>) => {
  try {
    await iform.updateForm(formId, payload);
  } catch (error: any) {
    message.error(error?.response?.data?.message || '更新失败');
  }
};

const confirmDelete = (form: ChannelIForm) => {
  if (!iform.canManage) {
    return;
  }
  dialog.warning({
    title: '删除控件',
    content: `确认删除「${form.name || form.id}」？该操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await iform.deleteForm(form.id);
        message.success('已删除');
      } catch (error: any) {
        message.error(error?.response?.data?.message || '删除失败');
      }
    },
  });
};

const openFloating = (formId: string) => {
  if (!formId) {
    return;
  }
  const windowId = iform.createWindowId(formId);
  iform.openFloating(formId, { windowId });
};

const pushSingle = async (form: ChannelIForm) => {
  if (!iform.canBroadcast) {
    return;
  }
  try {
    await iform.pushStates([
      {
        formId: form.id,
        width: form.defaultWidth,
        height: form.defaultHeight,
        collapsed: !!form.defaultCollapsed,
        floating: !!form.defaultFloating,
      },
    ], { force: true });
    message.success('已推送到频道');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '推送失败');
  }
};

const pushSelected = async () => {
  if (!iform.canBroadcast || !iform.selectedFormIds.length) {
    return;
  }
  const states = forms.value
    .filter((form) => iform.selectedFormIds.includes(form.id))
    .map((form) => ({
      formId: form.id,
      width: form.defaultWidth,
      height: form.defaultHeight,
      collapsed: !!form.defaultCollapsed,
      floating: !!form.defaultFloating,
    }));
  if (!states.length) {
    message.warning('未选择有效控件');
    return;
  }
  try {
    await iform.pushStates(states, { force: true });
    message.success('已推送选中控件');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '推送失败');
  }
};

const refresh = async () => {
  if (!iform.currentChannelId) {
    return;
  }
  await iform.ensureForms(iform.currentChannelId, true);
  message.success('已刷新控件列表');
};

const handleMigration = async () => {
  try {
    const targets = migrationTargets.value.slice();
    const selected = migrationFormIds.value.includes('@all') ? [] : migrationFormIds.value;
    await iform.migrateForms(targets, selected, migrationMode.value);
    message.success('迁移任务已提交');
    migrationModalVisible.value = false;
    migrationTargets.value = [];
    migrationFormIds.value = [];
  } catch (error: any) {
    message.error(error?.response?.data?.message || '迁移失败');
    return false;
  }
  return true;
};
</script>

<style scoped>
.iform-drawer :deep(.n-drawer-body) {
  background: var(--sc-bg-elevated, #0f172a);
  color: var(--sc-text-primary, #e2e8f0);
}

.iform-drawer__title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.iform-drawer__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1rem;
}

.iform-drawer__subtitle {
  margin: 0;
  font-size: 0.9rem;
  color: var(--sc-text-secondary, rgba(226, 232, 240, 0.8));
}

.iform-drawer__badges {
  display: flex;
  gap: 0.35rem;
  margin-top: 0.35rem;
}

.iform-toolbar {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.iform-card {
  border: 1px solid var(--iform-card-border, rgba(148, 163, 184, 0.25));
  border-radius: 16px;
  padding: 0.85rem 1rem;
  background: var(--iform-card-bg, var(--sc-bg-elevated, #f8fafc));
  box-shadow: 0 15px 35px rgba(15, 23, 42, 0.15);
  margin-bottom: 0.75rem;
  color: var(--iform-card-text, var(--sc-text-primary, #0f172a));
}

.iform-card strong {
  color: inherit;
}

.iform-card__header {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: center;
}

.iform-card__title {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.iform-card__meta {
  margin: 0;
  font-size: 0.8rem;
  color: var(--sc-text-secondary, rgba(100, 116, 139, 0.9));
}

.iform-card__actions {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}

.iform-card__body {
  margin-top: 0.75rem;
  font-size: 0.85rem;
  color: var(--sc-text-secondary, rgba(100, 116, 139, 0.95));
}

.iform-card__field {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.35rem;
}

.iform-form__size {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}
</style>
