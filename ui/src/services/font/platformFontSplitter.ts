import type {
  PlatformFontSplitCapability,
  PlatformFontSubsetPackagePayload,
} from './platformFontTypes'
import { urlBase } from '@/stores/_config'

const resolveRuntimeAssetUrl = (name: string): string =>
  `${urlBase}/api/v1/admin/platform-fonts/split-runtime/${name.split('/').map(encodeURIComponent).join('/')}`

type SplitWorkerSuccess = {
  id: string
  ok: true
  manifest: PlatformFontSubsetPackagePayload['manifest']
  files: Array<{
    name: string
    contentType: string
    data: Uint8Array
  }>
}

type SplitWorkerFailure = {
  id: string
  ok: false
  error: string
}

type SplitWorkerResponse = SplitWorkerSuccess | SplitWorkerFailure

const FONT_SPLIT_WASM_NAME = 'libffi-wasm32-wasip1.wasm'
const FONT_SPLIT_VERSION_NAME = 'version'

const parseVersionInfo = (raw: string): { version?: string; hasWasmVersion: boolean } => {
  const lines = String(raw || '')
    .split(/\r?\n/u)
    .map(line => line.trim())
    .filter(Boolean)
  const wasmLine = lines.find(line => line.startsWith('wasm32-wasip1'))
  const version = wasmLine?.split('@')[1]?.trim()
  return {
    version: version || undefined,
    hasWasmVersion: !!version,
  }
}

const readVersionInfo = async (): Promise<{ version?: string; hasWasmVersion: boolean }> => {
  try {
    const resp = await fetch(resolveRuntimeAssetUrl(FONT_SPLIT_VERSION_NAME), {
      credentials: 'include',
    })
    if (!resp.ok) {
      return { hasWasmVersion: false }
    }
    return parseVersionInfo(await resp.text())
  } catch {
    return { hasWasmVersion: false }
  }
}

const resolveWasmAssetName = async (): Promise<string | null> => {
  const target = resolveRuntimeAssetUrl(FONT_SPLIT_WASM_NAME)
  try {
    const resp = await fetch(target, {
      method: 'HEAD',
      credentials: 'include',
    })
    if (!resp.ok) return null
    return target
  } catch {
    return null
  }
}

export const getPlatformFontSplitCapability = async (): Promise<PlatformFontSplitCapability> => {
  if (typeof window === 'undefined' || typeof Worker === 'undefined') {
    return { available: false, reason: '当前环境不支持 Worker 分割' }
  }
  const versionInfo = await readVersionInfo()
  if (!versionInfo.hasWasmVersion) {
    return {
      available: false,
      version: versionInfo.version,
      reason: '未检测到分割运行时版本文件，请把产物放到主程序同目录的 bin/cn-font-split/',
    }
  }
  const wasmAssetName = await resolveWasmAssetName()
  if (!wasmAssetName) {
    return {
      available: false,
      version: versionInfo.version,
      reason: '未找到分割运行时 wasm：bin/cn-font-split/libffi-wasm32-wasip1.wasm',
    }
  }
  return {
    available: true,
    version: versionInfo.version,
    wasmAssetName,
  }
}

const createWorker = () => {
  return new Worker(new URL('./platformFontSplitWorker.ts', import.meta.url), {
    type: 'module',
  })
}

export const splitPlatformFontFile = async (input: {
  file: File
  family: string
  weight: string
  style: string
}): Promise<PlatformFontSubsetPackagePayload> => {
  const capability = await getPlatformFontSplitCapability()
  if (!capability.available || !capability.wasmAssetName) {
    throw new Error(capability.reason || '字体分割运行时不可用')
  }
  const buffer = await input.file.arrayBuffer()
  const worker = createWorker()
  try {
    const result = await new Promise<SplitWorkerResponse>((resolve, reject) => {
      const id = `${Date.now()}_${Math.random().toString(36).slice(2, 8)}`
      worker.onmessage = (event: MessageEvent<SplitWorkerResponse>) => {
        resolve(event.data)
      }
      worker.onerror = (event) => {
        reject(new Error(event.message || '字体分割 Worker 运行失败'))
      }
      worker.postMessage({
        id,
        type: 'split',
        payload: {
          fileName: input.file.name,
          buffer,
          family: input.family,
          weight: input.weight,
          style: input.style,
          wasmUrl: capability.wasmAssetName,
        },
      }, [buffer])
    })
    if (!result.ok) {
      throw new Error(result.error || '字体分割失败')
    }
    return {
      manifest: result.manifest,
      files: result.files.map((item) => ({
        name: item.name,
        blob: new Blob([item.data], { type: item.contentType }),
        contentType: item.contentType,
      })),
    }
  } finally {
    worker.terminate()
  }
}
