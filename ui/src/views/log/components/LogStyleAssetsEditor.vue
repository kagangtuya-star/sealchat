<script setup lang="ts">
// Style Studio · 资源：样式包自己的图片与字体（不是 LOG 工程素材）。上传、替换、删除只改工作副本；
// 字节随保存进入样式包版本，由服务端按内容校验 MIME 与哈希。
// 引用统计：当前 LOG 的结构化引用是准确的；CSS 只做 url(...) 文本匹配，作为保守提示。
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { Copy, Replace, Trash, Typography, Upload } from '@vicons/tabler';
import type { LogStyleAsset } from '@/types/log';
import { isFontMime } from '../logStyleWorkingCopy';

const props = defineProps<{
  assets: LogStyleAsset[];
  readonly: boolean;
  busy: boolean;
  /** 资源 id → 可显示的图片地址（Blob URL）；由 Studio 按本地字节或服务端版本读取。 */
  thumb: (asset: LogStyleAsset) => Promise<string | null>;
  /** 资源字节数（只有本地字节可知）。 */
  sizeOf: (id: string) => number | null;
  cssRefs: Map<string, number>;
  /** 当前 LOG 绑定的是同一样式包时，草稿对各资源 id 的结构化引用数；否则为 null。 */
  draftRefs: Map<string, number> | null;
}>();
const emit = defineEmits<{
  (e: 'upload', files: File[]): void;
  (e: 'replace', id: string, file: File): void;
  (e: 'remove', id: string): void;
}>();

const ACCEPT = '.png,.jpg,.jpeg,.webp,.ttf,.otf,image/png,image/jpeg,image/webp,font/ttf,font/otf';
const uploadInput = ref<HTMLInputElement | null>(null);
const replaceInput = ref<HTMLInputElement | null>(null);
const replacing = ref('');

const urls = ref<Record<string, string | null>>({});
let alive = true;
watch(() => props.assets.map((a) => a.id).join('|'), () => {
  for (const a of props.assets) {
    if (a.id in urls.value || isFontMime(a.mime)) continue;
    urls.value = { ...urls.value, [a.id]: null };
    void props.thumb(a).then((url) => {
      if (alive) urls.value = { ...urls.value, [a.id]: url };
    });
  }
}, { immediate: true });
onBeforeUnmount(() => {
  alive = false;
});

const images = computed(() => props.assets.filter((a) => !isFontMime(a.mime)));
const fonts = computed(() => props.assets.filter((a) => isFontMime(a.mime)));
const sizeText = (id: string) => {
  const n = props.sizeOf(id);
  if (n === null) return '';
  return n >= 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`;
};
const cssCount = (a: LogStyleAsset) => props.cssRefs.get(a.path) || 0;
const logCount = (a: LogStyleAsset) => props.draftRefs?.get(a.id) || 0;

const onUpload = (event: Event) => {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  input.value = '';
  if (files.length) emit('upload', files);
};
const startReplace = (id: string) => {
  replacing.value = id;
  replaceInput.value?.click();
};
const onReplace = (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (file && replacing.value) emit('replace', replacing.value, file);
  replacing.value = '';
};

const snippet = (a: LogStyleAsset) => (isFontMime(a.mime)
  ? `@font-face { font-family: "${a.path.replace(/^assets\//, '').replace(/\.[^.]+$/, '')}"; src: url(${a.path}) format("${a.mime === 'font/otf' ? 'opentype' : 'truetype'}"); }`
  : `url(${a.path})`);
const copied = ref('');
const copySnippet = async (a: LogStyleAsset) => {
  try {
    await navigator.clipboard.writeText(snippet(a));
    copied.value = a.id;
    setTimeout(() => {
      if (copied.value === a.id) copied.value = '';
    }, 1500);
  } catch {
    // 剪贴板不可用：用户可从卡片上的路径手动复制
  }
};
</script>

<template>
  <div class="lsa">
    <div class="lsa__toolbar">
      <n-button size="small" type="primary" secondary :loading="busy" :disabled="readonly" @click="uploadInput?.click()">
        <template #icon><n-icon :component="Upload" /></template>上传图片 / 字体
      </n-button>
      <span class="lsa__hint">PNG / JPEG / WebP 与 TTF / OTF；资源随样式版本保存、导出与发布，与 LOG 工程素材无关。</span>
      <input ref="uploadInput" type="file" multiple :accept="ACCEPT" hidden @change="onUpload">
      <input ref="replaceInput" type="file" :accept="ACCEPT" hidden @change="onReplace">
    </div>

    <p v-if="!assets.length" class="lsa__hint">这个样式还没有资源。上传后在 CSS 中用 <code>url(assets/文件名)</code> 引用（不加引号）。</p>

    <section v-if="images.length" aria-label="图片资源">
      <h4 class="lsa__heading">图片 <span>{{ images.length }}</span></h4>
      <div class="lsa__grid">
        <article v-for="a in images" :key="a.id" class="lsa__card">
          <div class="lsa__thumb">
            <img v-if="urls[a.id]" :src="urls[a.id]!" :alt="a.path">
            <span v-else class="lsa__hint">{{ a.id in urls ? '无法预览' : '载入中…' }}</span>
          </div>
          <code class="lsa__path" :title="a.path">{{ a.path }}</code>
          <span class="lsa__meta">{{ a.mime }}<template v-if="sizeText(a.id)"> · {{ sizeText(a.id) }}</template></span>
          <span class="lsa__refs" :class="{ 'is-unused': !cssCount(a) && !logCount(a) }">
            CSS {{ cssCount(a) ? `${cssCount(a)} 处` : '未发现' }}<template v-if="draftRefs"> · 当前 LOG {{ logCount(a) ? `${logCount(a)} 处` : '未引用' }}</template>
          </span>
          <div class="lsa__ops">
            <n-button size="tiny" quaternary :title="snippet(a)" @click="copySnippet(a)"><template #icon><n-icon :component="Copy" /></template>{{ copied === a.id ? '已复制' : 'url()' }}</n-button>
            <n-button size="tiny" quaternary :disabled="readonly || busy" title="用新文件替换，保留路径" @click="startReplace(a.id)"><template #icon><n-icon :component="Replace" /></template></n-button>
            <n-popconfirm v-if="!readonly" @positive-click="emit('remove', a.id)">
              <template #trigger><n-button size="tiny" quaternary type="error" aria-label="删除资源"><template #icon><n-icon :component="Trash" /></template></n-button></template>
              删除 {{ a.path }}？
              <template v-if="cssCount(a)">CSS 中仍有 {{ cssCount(a) }} 处 url() 引用，删除后保存会被校验拒绝。</template>
              <template v-if="logCount(a)">当前 LOG 有 {{ logCount(a) }} 处引用，应用新版本时会被移除或替换。</template>
            </n-popconfirm>
          </div>
        </article>
      </div>
    </section>

    <section v-if="fonts.length" aria-label="字体资源">
      <h4 class="lsa__heading">字体 <span>{{ fonts.length }}</span></h4>
      <div class="lsa__fonts">
        <article v-for="a in fonts" :key="a.id" class="lsa__font">
          <n-icon :component="Typography" size="22" class="lsa__font-icon" />
          <div class="lsa__font-body">
            <code class="lsa__path">{{ a.path }}</code>
            <span class="lsa__meta">{{ a.mime }}<template v-if="sizeText(a.id)"> · {{ sizeText(a.id) }}</template></span>
            <span class="lsa__refs" :class="{ 'is-unused': !cssCount(a) && !logCount(a) }">
              CSS {{ cssCount(a) ? `${cssCount(a)} 处` : '未发现' }}<template v-if="draftRefs"> · 当前 LOG 页面字体{{ logCount(a) ? '使用中' : '未使用' }}</template>
            </span>
          </div>
          <div class="lsa__ops">
            <n-button size="tiny" quaternary :title="snippet(a)" @click="copySnippet(a)"><template #icon><n-icon :component="Copy" /></template>{{ copied === a.id ? '已复制' : '@font-face' }}</n-button>
            <n-button size="tiny" quaternary :disabled="readonly || busy" title="用新文件替换，保留路径" @click="startReplace(a.id)"><template #icon><n-icon :component="Replace" /></template></n-button>
            <n-popconfirm v-if="!readonly" @positive-click="emit('remove', a.id)">
              <template #trigger><n-button size="tiny" quaternary type="error" aria-label="删除资源"><template #icon><n-icon :component="Trash" /></template></n-button></template>
              删除 {{ a.path }}？<template v-if="cssCount(a)">CSS 中仍有 {{ cssCount(a) }} 处引用。</template>
            </n-popconfirm>
          </div>
        </article>
      </div>
    </section>
    <p v-if="assets.length" class="lsa__hint">“CSS 未发现”只是文本匹配结果，不代表一定未被使用。替换会保留路径（CSS 无需修改），但资源 id 会变化：当前 LOG 若直接引用了旧资源，应用新版本时会提示调整。</p>
  </div>
</template>

<style scoped>
.lsa {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.lsa__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.lsa__heading {
  margin: 0 0 8px;
  font-size: 13px;
}

.lsa__heading span {
  font-weight: 400;
  color: var(--sc-text-secondary, #666);
}

.lsa__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 10px;
}

.lsa__card {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  padding: 8px;
  border: 1px solid var(--sc-border-mute, #e5e5e5);
  border-radius: 10px;
}

.lsa__thumb {
  display: grid;
  place-items: center;
  height: 104px;
  margin-bottom: 4px;
  overflow: hidden;
  border-radius: 6px;
  background:
    repeating-conic-gradient(rgba(127, 127, 127, 0.14) 0% 25%, transparent 0% 50%) 0 0 / 14px 14px;
}

.lsa__thumb img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.lsa__path {
  overflow: hidden;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lsa__meta,
.lsa__refs {
  font-size: 11px;
  color: var(--sc-text-secondary, #666);
}

.lsa__refs.is-unused {
  color: #9a5b00;
}

.lsa__ops {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
}

.lsa__fonts {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.lsa__font {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--sc-border-mute, #e5e5e5);
  border-radius: 10px;
}

.lsa__font-icon {
  color: var(--sc-text-secondary, #666);
}

.lsa__font-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.lsa__hint {
  margin: 0;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}
</style>
