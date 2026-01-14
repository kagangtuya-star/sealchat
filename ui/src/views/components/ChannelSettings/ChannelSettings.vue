<script lang="tsx" setup>
import { ChannelType, type SChannel } from '@/types';
import { clone } from 'lodash-es';
import { useDialog, useMessage } from 'naive-ui';
import { computed, onMounted, ref, watch, type PropType } from 'vue';
import TabMember from './TabMember.vue'
import useRequest from 'vue-hooks-plus/es/useRequest';
import { useChatStore } from '@/stores/chat';
import { useI18n } from 'vue-i18n';
import TabRoles from './TabRoles.vue'
import TabAppearance from './TabAppearance.vue'

const message = useMessage();
const dialog = useDialog();

const chat = useChatStore();
const { t } = useI18n();

const modelShow = defineModel({
  default: false,
  set(value: any): any {
    if (value) {
      if (props.channel) {
        console.log(2222, props.channel);
        model.value = clone(props.channel);
      }
    }
    return value;
  }
});

// watch(modelShow[1], () => {
//   console.log(111111);
//   alert(1111)
// })

const props = defineProps({
  channel: {
    type: Object as PropType<SChannel>,
  }
})

const model = ref<SChannel>({
  id: '',
  type: 0, // 0 text
  sortOrder: 0,
  typingIndicatorSetting: false,
})

const { data, run: doReload } = useRequest(async () => {
  if (props.channel) {
    model.value = clone(props.channel);
    const resp = await chat.channelInfoGet(props.channel.id);
    model.value = clone(resp.item);
  }
}, {});

watch(() => props.channel, () => {
  if (props.channel) {
    doReload();
  }
}, { immediate: true });

const channelEdit = async (): Promise<void> => {
  if (!model.value.name?.trim()) {
    message.error('频道名称不能为空');
    return;
  }
  try {
    await chat.channelInfoEdit(model.value.id, model.value);
    message.success('更新成功');
    await chat.channelList(chat.currentWorldId, true); // 强制刷新
    modelShow.value = false;
  } catch (err) {
    message.error('更新失败');
  }
}

const confirmBtnText = computed(() => {
  const curTab = tabRef.value;
  
  return curTab === 'basic' ? t('dialoChannelgNew.positiveText') : '';
});

const tabRef = ref('members');


</script>

<template>
  <n-modal v-model:show="modelShow" preset="dialog" :title="'频道设置'" :positive-text="confirmBtnText"
    :negative-text="$t('dialoChannelgNew.negativeText')" @positive-click="channelEdit" style="min-height: 70vh;"
    class="modalX">

    <n-tabs type="line" animated v-model:value="tabRef">
      <n-tab-pane name="members" tab="成员管理">
        <TabMember :channel="channel" />
      </n-tab-pane>

      <n-tab-pane name="basic" tab="基础设置">
        <n-form label-placement="top" label-width="auto" class="pt-4">
          <n-form-item label="频道名称">
            <n-input v-model:value="model.name" />
          </n-form-item>
          <!-- 可以在这里添加更多基础设置项 -->
          <n-form-item label="频道简介">
            <n-input v-model:value="model.note" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" />
          </n-form-item>
          <n-form-item label="频道类型">
            <n-radio-group v-model:value="model.permType">
              <n-radio :value="ChannelType.Public">公开</n-radio>
              <n-radio :value="ChannelType.NonPublic">非公开</n-radio>
            </n-radio-group>
          </n-form-item>

          <n-form-item label="置顶优先级" class="mb-8">
            <n-input-number v-model:value="model.sortOrder" />
            <template #feedback>数字越大排到越前</template>
          </n-form-item>

          <n-form-item label="成员'正在输入'提示" v-if="false">
            <n-radio-group v-model:value="model.typingIndicatorSetting">
              <n-radio :value="true">启用</n-radio>
              <n-radio :value="false">禁用</n-radio>
            </n-radio-group>
          </n-form-item>
        </n-form>
      </n-tab-pane>

      <n-tab-pane name="members2" tab="权限配置">
        <TabRoles :channel="channel" />
      </n-tab-pane>

      <n-tab-pane name="appearance" tab="外观设置">
        <TabAppearance :channel="model" @update="doReload" />
      </n-tab-pane>
    </n-tabs>

  </n-modal>
</template>

<style lang="scss">
.modalX {
  min-width: 83.333333%; // 5/6
}

@media (min-width: 768px) {
  .modalX {
    min-width: 66.666667%; // 2/3
  }
}
</style>
