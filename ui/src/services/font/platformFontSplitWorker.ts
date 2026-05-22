import type { PlatformFontSubsetManifest } from './platformFontTypes'

type SplitJobInput = {
  fileName: string
  buffer: ArrayBuffer
  family: string
  weight: string
  style: string
  wasmUrl: string
}

type SplitWorkerRequest = {
  id: string
  type: 'split'
  payload: SplitJobInput
}

type SplitWorkerSuccess = {
  id: string
  ok: true
  manifest: PlatformFontSubsetManifest
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

const ctx = self as DedicatedWorkerGlobalScope

const detectContentType = (name: string): string => {
  const lower = name.toLowerCase()
  if (lower.endsWith('.css')) return 'text/css'
  if (lower.endsWith('.woff2')) return 'font/woff2'
  if (lower.endsWith('.woff')) return 'font/woff'
  if (lower.endsWith('.ttf')) return 'font/ttf'
  if (lower.endsWith('.otf')) return 'font/otf'
  if (lower.endsWith('.json')) return 'application/json'
  if (lower.endsWith('.html')) return 'text/html'
  return 'application/octet-stream'
}

const sanitizeRelativeName = (name: string): string => {
  return (name || '')
    .replace(/\\/g, '/')
    .split('/')
    .filter((part) => part && part !== '.' && part !== '..')
    .join('/')
}

const inferManifest = (
  family: string,
  emittedFiles: Array<{ name: string; contentType: string; data: Uint8Array }>,
): PlatformFontSubsetManifest => {
  const cssFile = emittedFiles.find((item) => item.name.toLowerCase().endsWith('.css')) || null
  const chunks = emittedFiles
    .filter((item) => /\.(woff2?|ttf|otf)$/i.test(item.name))
    .map((item) => ({
      name: item.name,
      mimeType: item.contentType,
    }))
  return {
    mode: 'cn-font-split',
    entry: cssFile?.name || chunks[0]?.name || '',
    cssName: cssFile?.name || '',
    cssUrl: cssFile?.name || '',
    fontFiles: chunks.map((item) => item.name),
    fontUrls: chunks.map((item) => item.name),
    chunks,
  }
}

const handleSplit = async (req: SplitWorkerRequest): Promise<SplitWorkerResponse> => {
  try {
    const mod = await import('cn-font-split/dist/wasm/index.mjs')
    const wasm = new mod.StaticWasm(req.payload.wasmUrl)
    const results = await mod.fontSplit(
      {
        input: new Uint8Array(req.payload.buffer),
        outDir: './dist',
        renameOutputFont: '[index].[ext]',
        silent: true,
        autoSubset: true,
        languageAreas: true,
        reduceMins: true,
        css: {
          fontFamily: req.payload.family,
          fontWeight: req.payload.weight || '400',
          fontStyle: req.payload.style || 'normal',
          fontDisplay: 'swap',
          compress: true,
        },
      },
      wasm.WasiHandle,
      {
        logger() {
          // 管理端 worker 内忽略详细日志，避免刷屏。
        },
      },
    )

    const files = (results || [])
      .filter((item): item is { name: string; data: Uint8Array } => !!item?.name && !!item?.data)
      .map((item) => {
        const name = sanitizeRelativeName(item.name)
        return {
          name,
          contentType: detectContentType(name),
          data: item.data,
        }
      })
      .filter((item) => !!item.name)

    const manifest = inferManifest(req.payload.family, files)
    if (!manifest.entry || !manifest.chunks?.length) {
      throw new Error('分割结果缺少可发布的 CSS 或字体分片')
    }

    return {
      id: req.id,
      ok: true,
      manifest,
      files,
    }
  } catch (error: any) {
    return {
      id: req.id,
      ok: false,
      error: error?.message || '字体分割失败',
    }
  }
}

ctx.onmessage = async (event: MessageEvent<SplitWorkerRequest>) => {
  const response = await handleSplit(event.data)
  const transfer = response.ok ? response.files.map((item) => item.data.buffer) : []
  ctx.postMessage(response, transfer)
}
