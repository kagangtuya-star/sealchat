<template>
  <div class="scene-playlist">
    <AudioSearchBar v-model="keyword" placeholder="搜索场景 / 标签" @search="handleSearch" />
    <div class="scene-playlist__list" v-if="filteredScenes.length">
      <n-scrollbar style="height: 360px;">
        <div
          v-for="scene in filteredScenes"
          :key="scene.id"
          class="scene-card"
          :class="{ 'scene-card--active': scene.id === audio.currentSceneId }"
          @click="selectScene(scene.id)"
        >
          <header>
            <h4>{{ scene.name }}</h4>
            <n-tag v-for="tag in scene.tags" :key="tag" size="small">{{ tag }}</n-tag>
          </header>
          <p class="scene-card__desc">{{ scene.description || '暂无描述' }}</p>
          <div class="scene-card__tracks">
            <span v-for="track in scene.tracks" :key="track.type">
              {{ trackLabel(track.type) }} · {{ findAssetName(track.assetId) }}
            </span>
          </div>
        </div>
      </n-scrollbar>
    </div>
    <n-empty v-else description="暂未创建场景" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useAudioStudioStore } from '@/stores/audioStudio';
import AudioSearchBar from './AudioSearchBar.vue';

const audio = useAudioStudioStore();
const keyword = ref('');

const filteredScenes = computed(() => {
  if (!keyword.value) return audio.scenes;
  const lower = keyword.value.toLowerCase();
  return audio.scenes.filter((scene) => {
    const haystack = `${scene.name} ${scene.tags.join(' ')} ${scene.description ?? ''}`.toLowerCase();
    return haystack.includes(lower);
  });
});

function selectScene(id: string) {
  audio.applyScene(id);
}

function handleSearch(value: string) {
  keyword.value = value;
}

function trackLabel(type: string) {
  return { music: '音乐', ambience: '环境', sfx: '音效' }[type] || type;
}

function findAssetName(assetId: string | null) {
  if (!assetId) return '未绑定';
  return audio.assets.find((asset) => asset.id === assetId)?.name || '未加载';
}
</script>

<style scoped lang="scss">
.scene-playlist {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.scene-playlist__list {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  padding: 0.5rem;
}

.scene-card {
  border-radius: 10px;
  padding: 0.75rem;
  margin-bottom: 0.5rem;
  cursor: pointer;
  border: 1px solid transparent;
  transition: border-color 0.2s ease;
}

.scene-card--active {
  border-color: rgba(99, 179, 237, 0.8);
  background: rgba(99, 179, 237, 0.08);
}

.scene-card header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.scene-card h4 {
  margin: 0;
  font-size: 1rem;
}

.scene-card__desc {
  margin: 0.35rem 0;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.scene-card__tracks {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
}
</style>
