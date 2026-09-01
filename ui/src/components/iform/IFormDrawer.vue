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
          <n-button size="small" :disabled="!iform.canManage" @click="openTemplateModal">
            安装内置工具
          </n-button>
          <n-button size="small" :disabled="!iform.canBroadcast || !iform.selectedFormIds.length" @click="pushSelected">
            推送选中
          </n-button>
          <n-button
            size="small"
            tertiary
            :disabled="!iform.canManageWorldShared || !iform.selectedWorldShareEligibleForms.length"
            @click="toggleWorldShareSelected"
          >
            {{ iform.selectedWorldShareAllShared ? '取消世界共享' : '推送到世界' }}
          </n-button>
          <n-button size="small" tertiary :disabled="!iform.canManage || !forms.length" @click="migrationModalVisible = true">
            迁移/复制
          </n-button>
          <n-button size="small" tertiary :disabled="!forms.length" @click="exportForms">导出</n-button>
          <n-button size="small" tertiary :disabled="!iform.canManage" @click="openImport">导入</n-button>
          <input ref="importInput" type="file" accept="application/json,.json" hidden @change="handleImportFile" />
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
                    <div class="iform-card__meta-row">
                      <p class="iform-card__meta">
                        默认 {{ form.defaultWidth }} × {{ form.defaultHeight }} · {{ form.defaultCollapsed ? '折叠' : '展开' }} ·
                        {{ form.defaultFloating ? '弹出' : '面板' }}
                      </p>
                      <div class="iform-card__tags">
                        <n-tag v-if="form.templateMissing" size="small" type="error">模板不可用</n-tag>
                        <n-tag v-else-if="form.templateArchived" size="small" type="warning">模板已归档</n-tag>
                        <n-tag v-else-if="form.templateOrigin === 'builtin'" size="small" type="info">内置模板</n-tag>
                        <n-tag v-else-if="form.templateOrigin === 'platform'" size="small" type="info">平台模板</n-tag>
                        <n-tag v-else size="small">独立控件</n-tag>
                        <n-tag v-if="!form.sharedRef" size="small">本频道</n-tag>
                        <n-tag v-if="form.worldShared && !form.sharedRef" size="small" type="success">世界共享</n-tag>
                        <n-tag v-if="form.sharedRef" size="small" type="warning">世界引用</n-tag>
                      </div>
                    </div>
                  </div>
                </div>
                <div class="iform-card__actions">
                  <n-button quaternary size="tiny" @click="iform.openPanel(form.id)">面板</n-button>
                  <n-button
                    quaternary
                    size="tiny"
                    :disabled="form.allowPopout === false"
                    @click="popoutInternalLink(form)"
                  >
                    <template #icon><n-icon :component="OpenOutline" /></template>
                  </n-button>
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-button quaternary size="tiny" @click="copyInternalLink(form)">
                        <template #icon><n-icon :component="CopyOutline" /></template>
                      </n-button>
                    </template>
                    <span>复制外部链接</span>
                  </n-tooltip>
                  <n-button quaternary size="tiny" @click="copyEmbedLink(form)">复制嵌入</n-button>
                  <n-button quaternary size="tiny" :disabled="!iform.canBroadcast" @click="pushSingle(form)">推送</n-button>
                  <n-button quaternary size="tiny" :disabled="!iform.canManage || !canEditForm(form)" @click="openFormModal(form)">编辑</n-button>
                  <n-button quaternary size="tiny" :disabled="!iform.canManage || !canDeleteForm(form)" @click="confirmDelete(form)">
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
                    :disabled="!iform.canManage || !canEditForm(form)"
                    :value="form.defaultCollapsed"
                    @update:value="updateForm(form, { defaultCollapsed: $event })"
                  >
                    <template #checked>折叠</template>
                    <template #unchecked>展开</template>
                  </n-switch>
                  <n-switch
                    size="small"
                    :disabled="!iform.canManage || !canEditForm(form)"
                    :value="form.defaultFloating"
                    @update:value="updateForm(form, { defaultFloating: $event })"
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

      <n-modal v-model:show="formModalVisible" preset="dialog" :title="editingForm ? '编辑控件' : '新增控件'" :positive-text="editingForm ? '保存' : '创建'" negative-text="取消" @positive-click="handleSubmit" @negative-click="handleCancel">
        <n-form label-placement="left" label-width="72">
          <n-form-item label="名称" required>
            <n-input v-model:value="formModel.name" placeholder="示例：战斗地图" maxlength="64" />
          </n-form-item>
          <n-form-item label="URL">
            <n-input v-model:value="formModel.url" placeholder="https://example.com" :disabled="!!editingForm?.templateRef" />
          </n-form-item>
          <n-form-item label="嵌入代码">
            <n-input type="textarea" v-model:value="formModel.embedCode" placeholder="支持粘贴 HTML / iframe 代码（可含 script）" :rows="3" :disabled="!!editingForm?.templateRef" />
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
            <n-switch v-model:value="formModel.allowPopout">
              <template #checked>允许弹出</template>
              <template #unchecked>禁止弹出</template>
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
            <n-switch v-model:value="formModel.mediaOptions.allowAudio">
              <template #checked>允许音频</template>
              <template #unchecked>禁用音频</template>
            </n-switch>
            <n-switch v-model:value="formModel.mediaOptions.allowVideo">
              <template #checked>允许视频</template>
              <template #unchecked>禁用视频</template>
            </n-switch>
          </n-form-item>
          <n-form-item label="Embed API">
            <n-space vertical size="small">
              <n-switch v-model:value="formModel.bridgePolicy.enabled">
                <template #checked>启用</template>
                <template #unchecked>关闭</template>
              </n-switch>
              <n-input v-model:value="formModel.bridgePolicy.allowedOrigins" placeholder="允许来源，逗号分隔（可选）" :disabled="!formModel.bridgePolicy.enabled" />
              <n-input v-model:value="formModel.bridgePolicy.capabilities" placeholder="能力，逗号分隔（storage.read 等）" :disabled="!formModel.bridgePolicy.enabled" />
            </n-space>
          </n-form-item>
          <n-button v-if="editingForm?.templateRef" size="small" tertiary @click="resetTemplateOverrides">恢复模板默认</n-button>
        </n-form>
      </n-modal>

      <n-modal v-model:show="templateModalVisible" preset="card" title="安装内置工具" style="width: min(620px, 92vw);">
        <n-space vertical>
          <n-input v-model:value="templateSearch" placeholder="搜索模板" clearable @keyup.enter="loadTemplateCatalog(1)" />
          <n-spin :show="templateLoading">
            <n-list bordered>
              <n-list-item v-for="item in templateCatalog" :key="item.ref">
                <n-thing :title="item.name" :description="item.description || ''">
                  <template #header-extra><n-tag size="small" :type="item.origin === 'builtin' ? 'info' : 'success'">{{ item.origin === 'builtin' ? '内置' : '平台' }}</n-tag></template>
                  <template #footer><n-button size="small" type="primary" :disabled="!item.installable" @click="installTemplate(item.ref)">安装</n-button></template>
                </n-thing>
              </n-list-item>
            </n-list>
            <n-pagination
              v-if="templateTotal > templatePageSize"
              v-model:page="templatePage"
              :page-size="templatePageSize"
              :item-count="templateTotal"
              @update:page="loadTemplateCatalog"
            />
          </n-spin>
        </n-space>
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
import { useUtilsStore } from '@/stores/utils';
import { useMessage, useDialog } from 'naive-ui';
import { CopyOutline, OpenOutline, TrashOutline } from '@vicons/ionicons5';
import type { ChannelIForm } from '@/types/iform';
import { copyTextWithFallback } from '@/utils/clipboard';
import { generateIFormEmbedLink } from '@/utils/iformEmbedLink';
import {
  generateInternalSurfaceLink,
  openInternalSurfaceLink,
  resolveInternalSurfaceLinkBase,
} from '@/utils/internalSurfaceLink';
import { api } from '@/stores/_config';
import type { ChannelIFormTemplateCatalogItem } from '@/types/iform';

const iform = useIFormStore();
const chat = useChatStore();
const utils = useUtilsStore();
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
  allowPopout: true,
  mediaOptions: {
    autoPlay: false,
    autoUnmute: false,
    autoExpand: false,
    allowAudio: true,
    allowVideo: true,
  },
  bridgePolicy: {
    enabled: false,
    allowedOrigins: '',
    capabilities: 'context.read,user.read,members.read,world.admins.read,characters.read,permissions.read,storage.read,storage.write,events.subscribe,events.publish,messages.send',
  },
});

const migrationModalVisible = ref(false);
const migrationTargets = ref<string[]>([]);
const migrationMode = ref<'copy' | 'move'>('copy');
const migrationFormIds = ref<string[]>([]);
const templateModalVisible = ref(false);
const templateLoading = ref(false);
const templateSearch = ref('');
const templateCatalog = ref<ChannelIFormTemplateCatalogItem[]>([]);
const templatePage = ref(1);
const templatePageSize = 30;
const templateTotal = ref(0);
const importInput = ref<HTMLInputElement | null>(null);

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
    allowPopout: true,
    mediaOptions: {
      autoPlay: false,
      autoUnmute: false,
      autoExpand: false,
      allowAudio: true,
      allowVideo: true,
    },
    bridgePolicy: {
      enabled: false,
      allowedOrigins: '',
      capabilities: 'context.read,user.read,members.read,world.admins.read,characters.read,permissions.read,storage.read,storage.write,events.subscribe,events.publish,messages.send',
    },
  });
};

const openFormModal = (form?: ChannelIForm) => {
  if (!iform.canManage) {
    return;
  }
  if (form && !canEditForm(form)) {
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
      allowPopout: form.allowPopout !== false,
      mediaOptions: {
        autoPlay: !!form.mediaOptions?.autoPlay,
        autoUnmute: !!form.mediaOptions?.autoUnmute,
        autoExpand: !!form.mediaOptions?.autoExpand,
        allowAudio: form.mediaOptions?.allowAudio !== false,
        allowVideo: form.mediaOptions?.allowVideo !== false,
      },
      bridgePolicy: {
        enabled: !!form.bridgePolicy?.enabled,
        allowedOrigins: (form.bridgePolicy?.allowedOrigins || []).join(','),
        capabilities: (form.bridgePolicy?.capabilities || []).join(','),
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
  if (!editingForm.value?.templateRef && !formModel.url.trim() && !formModel.embedCode.trim()) {
    message.warning('请至少填写 URL 或嵌入代码');
    return false;
  }
  try {
    if (editingForm.value) {
      const payload: Record<string, unknown> = {};
      if (editingForm.value.templateRef) {
        const original = editingForm.value;
        const mediaOptions = { ...formModel.mediaOptions };
        const originalMediaOptions = {
          autoPlay: !!original.mediaOptions?.autoPlay,
          autoUnmute: !!original.mediaOptions?.autoUnmute,
          autoExpand: !!original.mediaOptions?.autoExpand,
          allowAudio: original.mediaOptions?.allowAudio !== false,
          allowVideo: original.mediaOptions?.allowVideo !== false,
        };
        const bridgePolicy = normalizeBridgePolicyForm();
        const originalBridgePolicy = {
          enabled: !!original.bridgePolicy?.enabled,
          allowedOrigins: original.bridgePolicy?.allowedOrigins || [],
          capabilities: original.bridgePolicy?.capabilities || [],
        };
        if (formModel.name.trim() !== (original.name || '')) payload.name = formModel.name.trim();
        if (formModel.defaultWidth !== original.defaultWidth) payload.defaultWidth = formModel.defaultWidth;
        if (formModel.defaultHeight !== original.defaultHeight) payload.defaultHeight = formModel.defaultHeight;
        if (formModel.defaultCollapsed !== !!original.defaultCollapsed) payload.defaultCollapsed = formModel.defaultCollapsed;
        if (formModel.defaultFloating !== !!original.defaultFloating) payload.defaultFloating = formModel.defaultFloating;
        if (formModel.allowPopout !== original.allowPopout) payload.allowPopout = formModel.allowPopout;
        if (JSON.stringify(mediaOptions) !== JSON.stringify(originalMediaOptions)) payload.mediaOptions = mediaOptions;
        if (JSON.stringify(bridgePolicy) !== JSON.stringify(originalBridgePolicy)) payload.bridgePolicy = bridgePolicy;
      } else {
        payload.name = formModel.name.trim();
        payload.defaultWidth = formModel.defaultWidth;
        payload.defaultHeight = formModel.defaultHeight;
        payload.defaultCollapsed = formModel.defaultCollapsed;
        payload.defaultFloating = formModel.defaultFloating;
        payload.allowPopout = formModel.allowPopout;
        payload.mediaOptions = formModel.mediaOptions;
        payload.bridgePolicy = normalizeBridgePolicyForm();
        payload.url = formModel.url.trim();
        payload.embedCode = formModel.embedCode.trim();
      }
      await iform.updateForm(editingForm.value.id, payload);
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
        allowPopout: formModel.allowPopout,
        mediaOptions: formModel.mediaOptions,
        bridgePolicy: normalizeBridgePolicyForm(),
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

const normalizeBridgePolicyForm = () => ({
  enabled: !!formModel.bridgePolicy.enabled,
  allowedOrigins: formModel.bridgePolicy.allowedOrigins.split(',').map((item) => item.trim()).filter(Boolean),
  capabilities: formModel.bridgePolicy.capabilities.split(',').map((item) => item.trim()).filter(Boolean),
});

const handleCancel = () => {
  resetFormModel();
  return true;
};

const updateForm = async (form: ChannelIForm, payload: Record<string, unknown>) => {
  if (!canEditForm(form)) {
    return;
  }
  try {
    await iform.updateForm(form.id, payload);
  } catch (error: any) {
    message.error(error?.response?.data?.message || '更新失败');
  }
};

const confirmDelete = (form: ChannelIForm) => {
  if (!iform.canManage || !canDeleteForm(form)) {
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

const resolveEmbedLinkBase = () => {
  const domain = utils.config?.domain?.trim() || '';
  if (!domain) {
    return undefined;
  }
  const webUrl = utils.config?.webUrl?.trim() || '';
  let base = domain;
  if (!/^(https?:)?\/\//i.test(base)) {
    base = `${window.location.protocol}//${base}`;
  }
  if (webUrl) {
    base = `${base}${webUrl.startsWith('/') ? '' : '/'}${webUrl}`;
  }
  return base;
};

const copyEmbedLink = async (form: ChannelIForm) => {
  const worldId = chat.currentWorldId;
  const channelId = chat.curChannel?.id;
  if (!worldId || !channelId || !form?.id) {
    message.warning('无法生成嵌入链接');
    return;
  }
  const link = generateIFormEmbedLink(
    {
      worldId,
      channelId: form.sourceChannelId || form.channelId,
      formId: form.id,
      width: form.defaultWidth,
      height: form.defaultHeight,
    },
    { base: resolveEmbedLinkBase() },
  );
  const copied = await copyTextWithFallback(link);
  if (copied) {
    message.success('嵌入链接已复制');
  } else {
    message.error('复制失败');
  }
};

const getInternalLink = (form: ChannelIForm) => {
  const worldId = String(chat.currentWorldId || '').trim();
  const channelId = String(iform.visibleChannelId || chat.curChannel?.id || '').trim();
  if (!worldId || !channelId || !form?.id) {
    message.warning('无法生成外部链接');
    return null;
  }
  return generateInternalSurfaceLink({
    type: 'iform',
    id: form.id,
    worldId,
    channelId,
  }, { base: resolveInternalSurfaceLinkBase(utils.config) });
};

const popoutInternalLink = (form: ChannelIForm) => {
  const link = getInternalLink(form);
  if (!link) return;
  const opened = openInternalSurfaceLink(link, {
    width: form.defaultWidth,
    height: form.defaultHeight,
  });
  if (!opened) {
    message.error('弹出失败，请允许浏览器弹窗');
  }
};

const copyInternalLink = async (form: ChannelIForm) => {
  const link = getInternalLink(form);
  if (!link) return;
  const copied = await copyTextWithFallback(link);
  copied ? message.success('外部链接已复制') : message.error('复制失败');
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

const canEditForm = (form: ChannelIForm) => !iform.isReadonlyForm(form);

const canDeleteForm = (form: ChannelIForm) => !form.sharedRef && !iform.isReadonlyForm(form);

const toggleWorldShareSelected = async () => {
  if (!iform.canManageWorldShared) {
    return;
  }
  const formIds = iform.selectedWorldShareEligibleForms.map((form) => form.id);
  if (!formIds.length) {
    message.warning('未选择可共享控件');
    return;
  }
  const enabled = !iform.selectedWorldShareAllShared;
  try {
    await iform.toggleWorldShare(formIds, enabled);
    message.success(enabled ? '已推送到世界' : '已取消世界共享');
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '世界共享切换失败');
  }
};

const refresh = async () => {
  if (!iform.currentChannelId) {
    return;
  }
  await iform.ensureForms(iform.currentChannelId, true);
  message.success('已刷新控件列表');
};

const loadTemplateCatalog = async (page = templatePage.value) => {
  templatePage.value = page;
  templateLoading.value = true;
  try {
    const { data } = await api.get<{ items: ChannelIFormTemplateCatalogItem[]; total?: number }>('api/v1/channel-embed-tools/catalog', {
      params: { search: templateSearch.value.trim(), page: templatePage.value, pageSize: templatePageSize },
    });
    templateCatalog.value = data?.items || [];
    templateTotal.value = data?.total || 0;
  } catch (error: any) {
    message.error(error?.response?.data?.message || '读取模板目录失败');
  } finally {
    templateLoading.value = false;
  }
};

const openTemplateModal = async () => {
  templateModalVisible.value = true;
  templatePage.value = 1;
  await loadTemplateCatalog(1);
};

const installTemplate = async (templateRef: string) => {
  try {
    await iform.createForm({ name: '', templateRef });
    await iform.ensureForms(iform.visibleChannelId || '', true);
    message.success('模板已安装');
    templateModalVisible.value = false;
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '安装失败');
  }
};

const resetTemplateOverrides = async () => {
  if (!editingForm.value?.templateRef) return;
  try {
    await iform.updateForm(editingForm.value.id, { templateOverrides: {} });
    message.success('已恢复模板默认');
    formModalVisible.value = false;
    resetFormModel();
  } catch (error: any) {
    message.error(error?.response?.data?.message || '恢复失败');
  }
};

const exportForms = () => {
  const items = forms.value.map((form) => form.templateRef
    ? { mode: 'reference', templateRef: form.templateRef, overrides: form.templateOverrides || {}, local: { orderIndex: form.orderIndex } }
    : { mode: 'standalone', config: {
      name: form.name, url: form.url || '', embedCode: form.embedCode || '', defaultWidth: form.defaultWidth,
      defaultHeight: form.defaultHeight, defaultCollapsed: form.defaultCollapsed, defaultFloating: form.defaultFloating,
      allowPopout: form.allowPopout, mediaOptions: form.mediaOptions, bridgePolicy: form.bridgePolicy,
    } });
  const blob = new Blob([JSON.stringify({ schemaVersion: 1, type: 'sealchat-channel-iforms', items }, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = 'sealchat-channel-iforms.json';
  anchor.click();
  URL.revokeObjectURL(url);
};

const openImport = () => importInput.value?.click();

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  try {
    const bundle = JSON.parse(await file.text());
    if (bundle?.schemaVersion !== 1 || bundle?.type !== 'sealchat-channel-iforms' || !Array.isArray(bundle.items)) {
      throw new Error('导入文件格式无效');
    }
    let success = 0;
    const failures: string[] = [];
    for (const [index, item] of bundle.items.entries()) {
      try {
        if (item?.mode === 'reference' && typeof item.templateRef === 'string') {
          await iform.createForm({ templateRef: item.templateRef, templateOverrides: item.overrides || {}, orderIndex: item.local?.orderIndex || 0, name: '' });
        } else if (item?.mode === 'standalone' && item.config) {
          await iform.createForm(item.config);
        } else {
          throw new Error('项目模式无效');
        }
        success += 1;
      } catch (error: any) {
        failures.push(`${index + 1}: ${error?.response?.data?.message || error?.message || '失败'}`);
      }
    }
    await iform.ensureForms(iform.visibleChannelId || '', true);
    message.info(`导入完成：成功 ${success}，失败 ${failures.length}${failures.length ? `（${failures.join('；')}）` : ''}`);
  } catch (error: any) {
    message.error(error?.message || '导入失败');
  }
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

.iform-card__meta-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-top: 0.15rem;
}

.iform-card__tags {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
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
