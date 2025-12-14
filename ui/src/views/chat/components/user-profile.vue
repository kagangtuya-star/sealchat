<script lang="tsx" setup>
import { useUserStore } from '@/stores/user';
import { useUtilsStore } from '@/stores/utils';
import { computed, onMounted, ref, watch } from 'vue';
import Avatar from '@/components/avatar.vue'
import imgAvatar from '@/assets/head2.png'
import { clamp, debounce } from 'lodash-es';
import { dataURItoBlob } from '@/utils/tools';
import { api } from '@/stores/_config';
import { useMessage } from 'naive-ui';
import { AxiosError } from 'axios';
import { useI18n } from 'vue-i18n'
import router from '@/router';

const { t } = useI18n()

const user = useUserStore();
const utils = useUtilsStore();
const message = useMessage()

const model = ref({
  nickname: '',
  brief: ''
})

const imageInfo = ref({
  image: '',
  scale: 0,
  tooSmall: false,

  offsetX: 0,
  offsetY: 0,
  shadeWidth: 30,

  imgBox: {
    w: 260,
    h: 200
  },

  imgMax: {
    w: 0,
    h: 0
  },

  imgMin: {
    w: 0,
    h: 0
  },

  camera: {
    top: 0,
    left: 0,
    w: 0,
    h: 0,

    moving: false,
    movePoint: { x: 0, y: 0 }
  },
})

const imgChanged = function (e: any) {
  imageInfo.value.scale = 0
  if (e.target.naturalWidth < 200 || e.target.naturalHeight < 200) {
    imageInfo.value.image = ''
    imageInfo.value.tooSmall = true
    message.error(t('userProfile.avatarUpload.tooSmall'))
    return
  }
  imageInfo.value.tooSmall = false
  imageInfo.value.imgMax.w = e.target.naturalWidth
  imageInfo.value.imgMax.h = e.target.naturalHeight

  let ratioImg = imageInfo.value.imgMax.w / imageInfo.value.imgMax.h
  let ratioStd = imageInfo.value.imgBox.w / imageInfo.value.imgBox.h

  if (ratioImg > ratioStd) {
    // 横向宽于标准尺寸
    imageInfo.value.imgMin.w = imageInfo.value.imgBox.h * ratioImg
    imageInfo.value.imgMin.h = imageInfo.value.imgBox.h
    imageInfo.value.offsetX = (imageInfo.value.imgMin.w - imageInfo.value.imgBox.w) / 2
    imageInfo.value.offsetY = 0
  } else if (ratioImg < ratioStd) {
    // 横向窄于标准尺寸
    if (ratioImg === 1) {
      // 正方形
      imageInfo.value.imgMin.w = imageInfo.value.imgBox.h
      imageInfo.value.imgMin.h = imageInfo.value.imgBox.h
      imageInfo.value.offsetX = (imageInfo.value.imgMin.w - imageInfo.value.imgBox.w) / 2
      imageInfo.value.offsetY = (imageInfo.value.imgMin.h - imageInfo.value.imgBox.h) / 2
    } else {
      // 长方形
      imageInfo.value.imgMin.w = imageInfo.value.imgBox.w
      imageInfo.value.imgMin.h = imageInfo.value.imgBox.w / ratioImg
      imageInfo.value.offsetX = 0
      imageInfo.value.offsetY = (imageInfo.value.imgMin.h - imageInfo.value.imgBox.h) / 2
    }
  } else {
    // 正好标准比例
    imageInfo.value.imgMin.w = imageInfo.value.imgBox.w
    imageInfo.value.imgMin.h = imageInfo.value.imgBox.h
    imageInfo.value.offsetX = 0
    imageInfo.value.offsetY = 0
  }

  imageInfo.value.camera.w = imageInfo.value.imgMin.w
  imageInfo.value.camera.h = imageInfo.value.imgMin.h
  refreshResult()
}

onMounted(async () => {
  await user.infoUpdate();
  model.value.nickname = user.info.nick;
  model.value.brief = user.info.brief;
})

const createImage = (file: File) => {
  let reader = new FileReader()
  reader.onload = (e: any) => {
    imageInfo.value.image = e.target.result
    imageInfo.value.scale = 0
  }
  reader.readAsDataURL(file)
}

const onFileChange = async (e: any) => {
  let files = e.target.files || e.dataTransfer.files
  if (!files.length) return
  const file = files[0]
  // Check file size before processing
  if (file.size > utils.fileSizeLimit) {
    const limitMB = (utils.fileSizeLimit / 1024 / 1024).toFixed(1)
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`)
    return
  }
  createImage(file)
}

const imageResult = ref('')
const canvasRef = ref<HTMLCanvasElement | null>(null);
const imgRef = ref<HTMLCanvasElement | null>(null);
const inputFileRef = ref<HTMLInputElement>()

const refreshResult = function () {
  let width = imageInfo.value.imgBox.h
  let img = imgRef.value;
  let canvas = canvasRef.value;
  if (!canvas || !img) return;
  let ctx = canvas.getContext('2d')
  if (!ctx) return;
  ctx.clearRect(0, 0, width, width)

  let factor = imageInfo.value.imgMax.w / imageInfo.value.camera.w
  ctx.drawImage(img,
    (-imageInfo.value.camera.left + imageInfo.value.offsetX + imageInfo.value.shadeWidth) * factor,
    (-imageInfo.value.camera.top + imageInfo.value.offsetY) * factor,
    width * factor, width * factor, 0, 0, width, width)
  imageResult.value = canvas.toDataURL('image/png')
}

const getXY = function (e: TouchEvent | MouseEvent) {
  let x = 0
  let y = 0
  if (e instanceof TouchEvent) {
    x = e.touches[0].clientX
    y = e.touches[0].clientY
  } else {
    x = e.clientX
    y = e.clientY
  }
  return { x, y }
}

const cameraMoveStart = function (e: TouchEvent | MouseEvent) {
  const info = getXY(e)
  imageInfo.value.camera.movePoint.x = info.x
  imageInfo.value.camera.movePoint.y = info.y
  imageInfo.value.camera.moving = true
}

const cameraMoveEnd = function (e: TouchEvent | MouseEvent) {
  imageInfo.value.camera.moving = false
  refreshResult()
}

const cameraMove = function (e: TouchEvent | MouseEvent) {
  if (imageInfo.value.camera.moving) {
    const info = getXY(e)
    imageInfo.value.camera.left += info.x - imageInfo.value.camera.movePoint.x
    imageInfo.value.camera.top += info.y - imageInfo.value.camera.movePoint.y
    imageInfo.value.camera.movePoint.x = info.x
    imageInfo.value.camera.movePoint.y = info.y
  }
}

const imgStyle = computed(() => {
  return {
    top: `${imageInfo.value.camera.top}px`,
    left: `${imageInfo.value.camera.left}px`,
    width: `${imageInfo.value.camera.w}px`,
    height: `${imageInfo.value.camera.h}px`,
    'min-width': `${imageInfo.value.camera.w}px`,
    'min-height': `${imageInfo.value.camera.h}px`,
    transform: `translate(${-imageInfo.value.offsetX}px, ${-imageInfo.value.offsetY}px)`
  }
})



const selectFile = async function () {
  let input = inputFileRef.value
  if (input) {
    // 清除里面的文件，确保第二次选同一个文件可用
    input.value = ''
  }
  inputFileRef.value?.click()
}

const backToStep1 = () => {
  imageInfo.value.image = ''
}

const saveAvatarImage = async () => {
  // this.loading = true
  let file = dataURItoBlob(imageResult.value)

  const formData = new FormData();
  formData.append('file', file, 'filename'); // 'filename' 是上传到服务器时的文件名

  try {
    const resp = await api.post('/api/v1/upload', formData, {
      // 如果有需要设置的headers，可以在这里添加
      headers: {
        Authorization: `${user.token}`,
        ChannelId: 'user-avatar', // 特殊值
        // 'Content-Type': 'multipart/form-data',
      },
    });

    // 检查上传是否成功
    if (resp.status === 200) {
      const attachmentId = resp.data?.ids?.[0];
      if (!attachmentId) {
        message.error('上传失败，未返回附件ID');
        return;
      }
      message.success('头像修改成功!')
      user.info.avatar = `id:${attachmentId}`
      imageInfo.value.image = '' // 关闭界面
    } else {
      // 处理上传失败的情况
      message.error('上传失败，请重新尝试')
      console.error('上传失败！', resp);
    }
  } catch (error) {
    // 处理异常
    message.error('出错了，请刷新重试或联系管理员: ' + (error as any).toString())
    console.error('上传出错！', error);
  }
}

const debounceRefreshResult = debounce(() => {
  refreshResult()
}, 500)

watch(() => imageInfo.value.scale, (val) => {
  let ncw = (imageInfo.value.imgMax.w - imageInfo.value.imgMin.w) * (val / 100) + imageInfo.value.imgMin.w
  let nch = (imageInfo.value.imgMax.h - imageInfo.value.imgMin.h) * (val / 100) + imageInfo.value.imgMin.h
  imageInfo.value.camera.left -= (ncw - imageInfo.value.camera.w) / 2
  imageInfo.value.camera.top -= (nch - imageInfo.value.camera.h) / 2
  imageInfo.value.camera.w = ncw
  imageInfo.value.camera.h = nch
  debounceRefreshResult()
  // tooSmall.value = imageInfo.value.tooSmall
})

watch(() => imageInfo.value.camera.left, (val) => {
  let ocx = imageInfo.value.camera.w - imageInfo.value.imgMin.w
  imageInfo.value.camera.left = -clamp(-val, -imageInfo.value.offsetX - imageInfo.value.shadeWidth, imageInfo.value.offsetX + imageInfo.value.shadeWidth + ocx)
})

watch(() => imageInfo.value.camera.top, (val) => {
  let ocy = imageInfo.value.camera.h - imageInfo.value.imgMin.h
  imageInfo.value.camera.top = -clamp(-val, -imageInfo.value.offsetY, imageInfo.value.offsetY + ocy)
})

const emit = defineEmits(['close'])

const save = async () => {
  if (imageInfo.value.image) {
    // 先保存头像（如果开着）
    await saveAvatarImage();
  }

  try {
    if (!model.value.nickname.trim()) {
      message.error('昵称不能为空')
      return
    }
    if (/\s/.test(model.value.nickname)) {
      message.error('昵称中间不能存在空格')
      return
    }

    await user.changeInfo({
      nick: model.value.nickname,
      brief: model.value.brief,
    });
    message.success('修改成功')
    user.info.nick = model.value.nickname
    user.info.brief = model.value.brief
    emit('close')
  } catch (error: any) {
    let msg = error.response?.data?.message;
    if (msg) {
      message.error('出错: ' + msg)
      return
    }
    message.error('修改失败: ' + (error as any).toString())
  }
}

const passwordChange = () => {
  router.push({ name: 'user-password-reset' })
}
</script>

<template>
  <div class="pointer-events-auto relative border px-4 py-2 rounded-md" style="min-width: 20rem;">
    <div class=" text-lg text-center mb-8">{{ $t('userProfile.title') }}</div>
    <n-form ref="formRef" :model="model" label-placement="left" label-width="64px" require-mark-placement="right-hanging"
      :style="{
      }">
      <n-form-item :label="$t('userProfile.nickname')" path="inputValue">
        <n-input v-model:value="model.nickname" placeholder="你的名字" />
      </n-form-item>
      <n-form-item :label="$t('userProfile.avatar')" path="inputValue">
        <input type="file" ref="inputFileRef" @change="onFileChange" accept="image/*" class="input-file" />
        <Avatar v-if="!imageInfo.image" :src="user.info.avatar" @click="selectFile"></Avatar>
        <div class="box" v-else>
          <!-- <div v-show="tooSmall" style="color: red; text-align: center">× 图片最低像素为（宽*高）：200*200</div> -->
          <div class="flex">
            <div class="left">
              <div class="img-container">
                <div class="shade left"></div>
                <div class="shade right"></div>
                <div class="h-full w-full" @mousedown.prevent="cameraMoveStart" @touchstart.prevent="cameraMoveStart"
                  @mouseout="cameraMoveEnd" @mouseup="cameraMoveEnd" @touchend="cameraMoveEnd"
                  @touchcancel="cameraMoveEnd" @mousemove="cameraMove" @touchmove="cameraMove">
                  <img @load="imgChanged" ref="imgRef" class="overflow-visible select-none" :style="imgStyle"
                    :src="imageInfo.image" />
                </div>
              </div>
              <div class="range-area mt-2">
                <n-slider v-model:value="imageInfo.scale" :step="1" />
                <!-- <input type="range" step="1" min="0" max="100" v-model="imageInfo.scale" /> -->
                <!-- <i class="icon5"></i>
                  <i class="icon6"></i> -->
              </div>
            </div>
            <div class="right">
              <div class="preview-item rect" style="margin-left: 10px;">
                <img :src="imageResult" />
                <span class="text">{{ $t('userProfile.avatarUpload.preview') }}</span>
              </div>
            </div>
          </div>
          <canvas style="display: none" :width="imageInfo.imgBox.h" :height="imageInfo.imgBox.h" ref="canvasRef" />
          <div class="space-x-2 mt-4">
            <n-button small @click="backToStep1">{{ $t('userProfile.avatarUpload.back') }}</n-button>
            <n-button small @click="saveAvatarImage">{{ $t('userProfile.avatarUpload.confirm') }}</n-button>
          </div>
        </div>

        <!-- <n-input v-model:value="model.inputValue" placeholder="头像" /> -->
      </n-form-item>
      <n-form-item :label="$t('userProfile.brief')" path="textareaValue">
        <n-input v-model:value="model.brief" :placeholder="$t('userProfile.briefPlaceholder')" type="textarea" :autosize="{
          minRows: 3,
          maxRows: 5
        }" />
      </n-form-item>
      <n-form-item :label="'其他'" path="textareaValue">
        <n-button @click="passwordChange">修改密码</n-button>
      </n-form-item>
    </n-form>
    <div class="flex justify-end mb-4 space-x-4">
      <n-button @click="emit('close')">{{ $t('userProfile.cancel') }}</n-button>
      <n-button @click="save" type="primary">{{ $t('userProfile.save') }}</n-button>
    </div>
  </div>
</template>

<style lang="scss">
.img-container {
  width: 260px;
  height: 200px;
  overflow: hidden;
  position: relative;

  img {
    position: absolute;
  }

  .shade {
    z-index: 1;
    position: absolute;
    box-shadow: 0 2px 6px 0 rgba(0, 0, 0, 0.18);
    background-color: rgba(241, 242, 243, 0.8);
    width: 30px; // (260-200)/2
    pointer-events: none;

    &.left {
      height: 100%;
      width: 30px;
    }

    &.right {
      height: 100%;
      width: 30px;
      right: 0;
    }
  }
}

.preview-item {
  width: 100px;
  height: 100px;
  display: flex;
  flex-direction: column;

  img {
    width: 100%;
    height: 100%;
  }

  .text {
    @apply text-gray-600;
    margin-top: 10px;
    width: 100%;
    text-align: center;
  }

  &.rect {
    img {
      @apply rounded-lg;
      padding: 3px;
      border: 1px solid rgba(0, 0, 0, 0.15);
    }
  }

  &.circle {
    img {
      border-radius: 100%;
      padding: 3px;
      border: 1px solid rgba(0, 0, 0, 0.15);
    }
  }
}

.input-file {
  display: none;
}
</style>
