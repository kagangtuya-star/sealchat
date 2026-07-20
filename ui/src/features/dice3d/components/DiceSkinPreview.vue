<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as THREE from 'three'

import type { Dice3DSkin } from '@/types'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { createDiceAtlasTexture, DICE_GEOMETRY_RESOURCES, type DiceResourceKey } from '../engine/DiceGeometryRegistry'

const props = defineProps<{ skin: Dice3DSkin; label?: string }>()
const canvasRef = ref<HTMLCanvasElement | null>(null)
const hostRef = ref<HTMLElement | null>(null)
const failed = ref(false)
const diceItems: Array<{ label: string; key: DiceResourceKey }> = [
  { label: 'd2', key: 'd2' }, { label: 'd4', key: 'd4' }, { label: 'd6', key: 'd6' }, { label: 'd8', key: 'd8' },
  { label: 'd10', key: 'd10' }, { label: 'd12', key: 'd12' }, { label: 'd20', key: 'd20' }, { label: 'd100', key: 'd100tens' },
]

let renderer: THREE.WebGLRenderer | null = null
let frame = 0
let observer: ResizeObserver | null = null
const scene = new THREE.Scene()
const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 100)
const diceGroups: THREE.Group[] = []
const disposableTextures: THREE.Texture[] = []
const disposableMaterials: THREE.Material[] = []

const clearDice = () => {
  diceGroups.splice(0).forEach(group => scene.remove(group))
  disposableTextures.splice(0).forEach(texture => texture.dispose())
  disposableMaterials.splice(0).forEach(material => material.dispose())
}

const layoutDice = () => {
  const width = camera.right - camera.left
  const height = camera.top - camera.bottom
  diceGroups.forEach((group, index) => {
    const column = index % 4
    const row = Math.floor(index / 4)
    group.position.set(
      camera.left + (column + 0.5) * width / 4,
      camera.top - (row + 0.5) * height / 2,
      0,
    )
  })
}

const textureFor = (key: DiceResourceKey) => {
  const resource = DICE_GEOMETRY_RESOURCES[key]
  const source = props.skin.textures?.[resource.atlasType]
  const texture = source
    ? new THREE.TextureLoader().load(resolveAttachmentUrl(source))
    : createDiceAtlasTexture(resource.atlasType, props.skin)
  texture.colorSpace = THREE.SRGBColorSpace
  disposableTextures.push(texture)
  return texture
}

const rebuild = () => {
  if (!renderer) return
  clearDice()
  diceItems.forEach((item, index) => {
    const resource = DICE_GEOMETRY_RESOURCES[item.key]
    const faceMaterial = new THREE.MeshStandardMaterial({
      color: 0xffffff,
      map: textureFor(item.key),
      roughness: props.skin.roughness ?? 0.72,
      metalness: props.skin.metalness ?? 0.05,
      flatShading: true,
    })
    const edgeMaterial = new THREE.LineBasicMaterial({ color: props.skin.edgeColor || '#d1d5db', transparent: true, opacity: 0.82 })
    disposableMaterials.push(faceMaterial, edgeMaterial)
    const mesh = new THREE.Mesh(resource.geometry, faceMaterial)
    const edges = new THREE.LineSegments(resource.edgeGeometry, edgeMaterial)
    const group = new THREE.Group()
    const scale = (0.82 / resource.radius) * Math.max(0.72, Math.min(1.28, props.skin.scale || 1))
    mesh.scale.setScalar(scale)
    edges.scale.setScalar(scale * 1.004)
    group.add(mesh, edges)
    group.rotation.set(-0.42 + index * 0.07, 0.48 + index * 0.19, 0.08)
    scene.add(group)
    diceGroups.push(group)
  })
  layoutDice()
}

const resize = () => {
  if (!renderer || !hostRef.value) return
  const { width, height } = hostRef.value.getBoundingClientRect()
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
  renderer.setSize(Math.max(1, width), Math.max(1, height), false)
  const halfHeight = 2.9
  const halfWidth = halfHeight * width / Math.max(1, height)
  camera.left = -halfWidth
  camera.right = halfWidth
  camera.top = halfHeight
  camera.bottom = -halfHeight
  camera.updateProjectionMatrix()
  layoutDice()
}

const tick = () => {
  diceGroups.forEach((group, index) => {
    group.rotation.x += 0.0017 + index * 0.00008
    group.rotation.y += 0.0024 + index * 0.00011
  })
  renderer?.render(scene, camera)
  frame = requestAnimationFrame(tick)
}

watch(() => props.skin, () => nextTick(rebuild), { deep: true })

onMounted(() => {
  if (!canvasRef.value || !hostRef.value) return
  try {
    renderer = new THREE.WebGLRenderer({ canvas: canvasRef.value, alpha: true, antialias: true })
    renderer.setClearColor(0x000000, 0)
    camera.position.set(0, 0, 10.8)
    scene.add(new THREE.HemisphereLight(0xffffff, 0x243247, 2.8))
    const light = new THREE.DirectionalLight(0xffffff, 3.6)
    light.position.set(-3, 6, 7)
    scene.add(light)
    observer = new ResizeObserver(resize)
    observer.observe(hostRef.value)
    resize()
    rebuild()
    tick()
  } catch {
    failed.value = true
  }
})

onBeforeUnmount(() => {
  cancelAnimationFrame(frame)
  observer?.disconnect()
  clearDice()
  renderer?.dispose()
  renderer = null
})
</script>

<template>
  <section class="dice-preview" aria-label="全部 3D 骰型样式预览">
    <header class="dice-preview__header">
      <strong>{{ label || '全部骰型预览' }}</strong>
    </header>
    <div ref="hostRef" class="dice-preview__stage">
      <canvas v-if="!failed" ref="canvasRef" />
      <div v-else class="dice-preview__fallback">当前设备无法创建 WebGL 预览</div>
      <div class="dice-preview__labels"><span v-for="item in diceItems" :key="item.label">{{ item.label }}</span></div>
    </div>
  </section>
</template>

<style scoped>
.dice-preview { overflow: hidden; border: 1px solid var(--sc-border-muted, rgba(148,163,184,.24)); border-radius: 14px; background: radial-gradient(circle at 50% 44%, rgba(54,173,146,.12), transparent 48%), color-mix(in srgb, var(--sc-bg-surface, #111318) 96%, #000); }
.dice-preview__header { padding: 13px 16px 0; }.dice-preview__header strong { font-size: 15px; }
.dice-preview__stage { position: relative; width: 100%; height: 260px; }.dice-preview__stage canvas { display: block; width: 100%; height: 100%; }
.dice-preview__labels { position: absolute; inset: 0; display: grid; grid-template-columns: repeat(4, 1fr); grid-template-rows: repeat(2, 1fr); pointer-events: none; }.dice-preview__labels span { align-self: end; justify-self: center; margin-bottom: 9px; padding: 2px 6px; border-radius: 999px; color: rgba(226,232,240,.72); background: rgba(15,23,42,.54); font-size: 10px; }
.dice-preview__fallback { height: 100%; display: grid; place-items: center; color: var(--sc-text-secondary); font-size: 12px; }
@media (max-width: 560px) { .dice-preview__stage { height: 220px; } }
@media (prefers-reduced-motion: reduce) { .dice-preview__stage canvas { opacity: .92; } }
</style>
