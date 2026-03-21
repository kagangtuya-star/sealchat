<script lang="tsx" setup>
import { urlBase } from '@/stores/_config';
import { computed, onMounted, ref } from 'vue';
import FileUpload from 'vue-upload-component'
import { useUserStore } from '@/stores/user';
import { filesize } from "filesize";
import { useMessage } from 'naive-ui';
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';

const user = useUserStore();
const chat = useChatStore();
const message = useMessage()

const files = ref<any[]>([])

const uploadRef = ref<any>(null)
const dragAreaRef = ref<any>(null);
const postAction = urlBase + '/api/v1/upload'

const headers = computed(() => {
  return {
    ChannelId: `${chat.curChannel?.id}`,
    Authorization: `${user.token}`
  }
})

const inputFile = async (newFile: any, oldFile: any) => {
  if (newFile && oldFile && !newFile.active && oldFile.active) {
    // 获得相应数据
    console.log('response', newFile.response)
    dialogVisible.value = false;
    files.value = [];

    let fileType = 'image'
    if (newFile.file.type.startsWith('audio/')) {
      fileType = 'audio'
    }
    if (newFile.file.type.startsWith('image/')) {
      fileType = 'image'
    }

    const attachmentId = newFile.response?.ids?.[0] as string | undefined;

    if (newFile.xhr) {
      if (newFile.xhr.status === 200) {
        // 上传成功
        if (fileType === 'image' && attachmentId) {
          const resp = await chat.messageCreate(`<img src="id:${attachmentId}" />`)
          if (!resp) {
            message.error('发送失败,您可能没有权限在此频道发送消息');
            return;
          }
        }
        if (fileType === 'audio' && attachmentId) {
          const resp = await chat.messageCreate(`<audio src="id:${attachmentId}" />`)
          if (!resp) {
            message.error('发送失败,您可能没有权限在此频道发送消息');
            return;
          }
        }
        if (!attachmentId) {
          message.error('上传结果缺少附件ID，消息未发送');
          return;
        }
        if (fileType !== 'image' && fileType !== 'audio') {
          message.error('无法处理此格式的文件');
        }
        console.log('success')
      } else {
        // 上传失败
        message.error('上传失败')
      }
    }
  }
}

const inputFilter = function (newFile: any, oldFile: any, prevent: any) {
  if (newFile && !oldFile) {
    // 过滤不是图片后缀的文件
    if (/\.(jpeg|jpe|jpg|gif|png|webp)$/i.test(newFile.name)) {
      // 创建 blob 字段 用于图片预览
      newFile.blob = ''
      let URL = window.URL || window.webkitURL
      if (URL && URL.createObjectURL) {
        newFile.blob = URL.createObjectURL(newFile.file)
      }

      newFile.thumb = ''
      if (newFile.blob && newFile.type.substr(0, 6) === 'image/') {
        newFile.thumb = newFile.blob
      }
      dialogVisible.value = true;
    } else if (/\.(mp3|ogg|aac|wav)$/i.test(newFile.name)) {
      // 创建 blob 字段 音频预览
      newFile.blob = ''
      let URL = window.URL || window.webkitURL
      if (URL && URL.createObjectURL) {
        newFile.blob = URL.createObjectURL(newFile.file)
      }

      newFile.audio = ''
      if (newFile.blob && newFile.type.substr(0, 6) === 'audio/') {
        newFile.audio = newFile.blob
      }
      dialogVisible.value = true;
    } else {
      message.error('无法处理此格式的文件');
      return prevent()
    }
  }
}

document.addEventListener('paste', function (event) {
  // 获取粘贴事件中的剪贴板数据
  const items = (event.clipboardData || (event as any).originalEvent.clipboardData).items;

  // 遍历剪贴板中的每一项
  for (let i = 0; i < items.length; i++) {
    // 如果是文件类型
    if (items[i].kind === 'file') {
      const file = items[i].getAsFile();

      // 模拟文件上传
      uploadRef.value.add(file)
      break;
    }
  }
});

const dialogVisible = ref(false)

const isDropShow = computed(() => {
  return Boolean(uploadRef.value && uploadRef.value.dropActive)
});


function preventDefaults(e: any) {
  e.preventDefault();
  e.stopPropagation();
}

['drop', 'dragover'].forEach(eventName => {
  document.body.addEventListener(eventName, preventDefaults, false);
});

onMounted(() => {
  dragAreaRef.value.addEventListener('drop', (e: any) => {
    // 有开着的就替换掉，但是不用做其余操作
    uploadRef.value.dropActive = false; // 反应太慢，这里加速一下
    uploadRef.value.add(e.dataTransfer.files[0])
    preventDefaults(e);
  }, false);
})

defineExpose({
  openUpload: () => {
    const el = uploadRef.value.$el;
    el.querySelector('input').click();
  }
})

const utils = useUtilsStore()
const fileSizeLimit = computed(() => {
  return utils.fileSizeLimit;
})
</script>

<template>
  <div class=" absolute">
    <file-upload ref="uploadRef" v-model="files" :post-action="postAction" @input-file="inputFile"
      @input-filter="inputFilter" :headers="headers" :size="fileSizeLimit" :drop="true" :drop-directory="false">
    </file-upload>
  </div>

  <div class="drop-active" v-show="isDropShow" ref="dragAreaRef">
    拖拽到这里上传
  </div>

  <n-modal v-model:show="dialogVisible" preset="dialog" title="上传文件" :auto-focus="false">
    <div class="p-4">
      <ul class="mb-4">
        <li v-for="file in files" :key="file.name" class="text-gray-600">
          <template v-if="file.thumb">
            <img class="td-image-thumb" v-if="file.thumb" :src="file.thumb" />
            <div class="text-center">
              <span>{{ file.name }}</span>
              <span> - </span>
              <span :class="file.size > fileSizeLimit ? 'text-red-500' : ''">{{ filesize(file.size) }}</span>
            </div>
          </template>
          <template v-else>
            <!-- <img class="td-image-thumb" v-if="file.thumb" :src="file.thumb" /> -->
            <div class="text-center">
              <span>{{ file.name }}</span>
              <span> - </span>
              <span :class="file.size > fileSizeLimit ? 'text-red-500' : ''">{{ filesize(file.size) }}</span>
            </div>
          </template>
          <!-- {{ file.name }} - Error: {{ file.error }}, Success: {{ file.success }} -->
        </li>
      </ul>
    </div>

    <template #action>
      <n-button autofocus v-show="!uploadRef || !uploadRef.active" @click.prevent="uploadRef.active = true" type="primary"
        :disabled="Boolean(files.length) && files[0].size > fileSizeLimit">开始上传</n-button>
      <n-button v-show="uploadRef && uploadRef.active" @click.prevent="uploadRef.active = false"
        type="button">停止上传</n-button>
      <n-button @click="dialogVisible = false">取消</n-button>
    </template>
  </n-modal>
</template>

<style lang="scss">
.drop-active {
  top: 0;
  bottom: 0;
  right: 0;
  left: 0;
  position: fixed;
  z-index: 9999;
  opacity: .6;
  text-align: center;
  background: #000;
}
</style>
