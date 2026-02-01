import { ref } from 'vue'
import type { WorldKeywordItem } from '@/models/worldGlossary'

// pinyin-pro 类型定义
interface PinyinProModule {
  pinyin: (text: string, options?: { toneType?: 'none' | 'symbol' | 'num'; type?: 'string' | 'array' }) => string | string[]
}

// 懒加载状态
let pinyinModule: PinyinProModule | null = null
let loadPromise: Promise<boolean> | null = null
const pinyinReadyVersion = ref(0)

const markPinyinReady = () => {
  pinyinReadyVersion.value += 1
}

// CDN 地址（UMD，全局暴露 window.pinyinPro）
const CDN_URL = 'https://npm.elemecdn.com/pinyin-pro/dist/index.js'
const CDN_SCRIPT_ID = 'pinyin-pro-cdn'

const getGlobalPinyinModule = (): PinyinProModule | null => {
  const globalModule = (window as any).pinyinPro || (window as any).PinyinPro
  if (globalModule?.pinyin) {
    return globalModule as PinyinProModule
  }
  return null
}

// 从 CDN 加载 pinyin-pro
async function loadFromCDN(): Promise<PinyinProModule | null> {
  return new Promise((resolve) => {
    const existing = document.getElementById(CDN_SCRIPT_ID) as HTMLScriptElement | null
    const cachedModule = getGlobalPinyinModule()
    if (cachedModule) {
      resolve(cachedModule)
      return
    }

    let script = existing
    let timer: number | undefined

    const cleanup = () => {
      if (!script) return
      script.removeEventListener('load', handleLoad)
      script.removeEventListener('error', handleError)
      if (timer) {
        window.clearTimeout(timer)
      }
    }

    const finish = (module: PinyinProModule | null) => {
      cleanup()
      resolve(module)
    }

    const handleLoad = () => {
      finish(getGlobalPinyinModule())
    }

    const handleError = () => {
      finish(null)
    }

    if (!script) {
      script = document.createElement('script')
      script.id = CDN_SCRIPT_ID
      script.src = CDN_URL
      script.async = true
      document.head.appendChild(script)
    }

    script.addEventListener('load', handleLoad)
    script.addEventListener('error', handleError)

    // 超时处理
    timer = window.setTimeout(() => {
      finish(null)
    }, 5000)
  })
}

// 从本地模块加载 pinyin-pro
async function loadFromLocal(): Promise<PinyinProModule | null> {
  try {
    const module = await import('pinyin-pro')
    return module as unknown as PinyinProModule
  } catch (error) {
    console.warn('[pinyinMatch] Failed to load pinyin-pro from local:', error)
    return null
  }
}

// 确保拼音库已加载（懒加载入口）
export async function ensurePinyinLoaded(): Promise<boolean> {
  if (pinyinModule) {
    return true
  }

  if (loadPromise) {
    return loadPromise
  }

  loadPromise = (async () => {

    // 优先尝试 CDN
    pinyinModule = await loadFromCDN()
    if (pinyinModule) {
      console.info('[pinyinMatch] Loaded pinyin-pro from CDN')
      markPinyinReady()
      return true
    }

    // CDN 失败则回退本地模块
    pinyinModule = await loadFromLocal()
    if (pinyinModule) {
      console.info('[pinyinMatch] Loaded pinyin-pro from local module')
      markPinyinReady()
      return true
    }

    console.warn('[pinyinMatch] Failed to load pinyin-pro from both local and CDN')
    return false
  })()

  return loadPromise
}

// 获取拼音首字母
export function getPinyinInitials(text: string): string {
  if (!pinyinModule || !text) {
    return ''
  }

  try {
    const result = pinyinModule.pinyin(text, { toneType: 'none', type: 'array' })
    if (Array.isArray(result)) {
      return result.map((p) => p.charAt(0).toUpperCase()).join('')
    }
    return ''
  } catch {
    return ''
  }
}

// 获取拼音全拼（无音调，空格分隔）
export function getPinyinFull(text: string): string {
  if (!pinyinModule || !text) {
    return ''
  }

  try {
    const result = pinyinModule.pinyin(text, { toneType: 'none', type: 'string' })
    if (typeof result === 'string') {
      return result.toLowerCase().replace(/\s+/g, '')
    }
    return ''
  } catch {
    return ''
  }
}

// 匹配类型
export type MatchType =
  | 'keywordExact'
  | 'keywordStartsWith'
  | 'keywordContains'
  | 'aliasExact'
  | 'aliasStartsWith'
  | 'aliasContains'
  | 'pinyinInitialExact'
  | 'pinyinInitialStartsWith'
  | 'pinyinInitialContains'
  | 'pinyinFullStartsWith'
  | 'pinyinFullContains'

// 匹配结果
export interface KeywordMatchResult {
  keyword: WorldKeywordItem
  score: number
  matchType: MatchType
}

// 匹配评分
const SCORE_MAP: Record<MatchType, number> = {
  keywordExact: 100,
  keywordStartsWith: 90,
  keywordContains: 80,
  aliasExact: 70,
  aliasStartsWith: 65,
  aliasContains: 60,
  pinyinInitialExact: 50,
  pinyinInitialStartsWith: 40,
  pinyinInitialContains: 30,
  pinyinFullStartsWith: 25,
  pinyinFullContains: 20,
}

// 对单个术语进行匹配
function matchSingleKeyword(query: string, item: WorldKeywordItem): KeywordMatchResult | null {
  if (!item.isEnabled) {
    return null
  }

  const queryLower = query.toLowerCase()
  const keywordLower = item.keyword.toLowerCase()

  // 关键字匹配
  if (keywordLower === queryLower) {
    return { keyword: item, score: SCORE_MAP.keywordExact, matchType: 'keywordExact' }
  }
  if (keywordLower.startsWith(queryLower)) {
    return { keyword: item, score: SCORE_MAP.keywordStartsWith, matchType: 'keywordStartsWith' }
  }
  if (keywordLower.includes(queryLower)) {
    return { keyword: item, score: SCORE_MAP.keywordContains, matchType: 'keywordContains' }
  }

  // 别名匹配
  const aliases = item.aliases || []
  for (const alias of aliases) {
    const aliasLower = alias.toLowerCase()
    if (aliasLower === queryLower) {
      return { keyword: item, score: SCORE_MAP.aliasExact, matchType: 'aliasExact' }
    }
    if (aliasLower.startsWith(queryLower)) {
      return { keyword: item, score: SCORE_MAP.aliasStartsWith, matchType: 'aliasStartsWith' }
    }
    if (aliasLower.includes(queryLower)) {
      return { keyword: item, score: SCORE_MAP.aliasContains, matchType: 'aliasContains' }
    }
  }

  // 拼音匹配（需要拼音库已加载）
  if (pinyinModule) {
    const queryUpper = query.toUpperCase()

    // 关键字拼音首字母
    const keywordInitials = getPinyinInitials(item.keyword)
    if (keywordInitials) {
      if (keywordInitials === queryUpper) {
        return { keyword: item, score: SCORE_MAP.pinyinInitialExact, matchType: 'pinyinInitialExact' }
      }
      if (keywordInitials.startsWith(queryUpper)) {
        return { keyword: item, score: SCORE_MAP.pinyinInitialStartsWith, matchType: 'pinyinInitialStartsWith' }
      }
      if (keywordInitials.includes(queryUpper)) {
        return { keyword: item, score: SCORE_MAP.pinyinInitialContains, matchType: 'pinyinInitialContains' }
      }
    }

    // 关键字拼音全拼
    const keywordPinyin = getPinyinFull(item.keyword)
    if (keywordPinyin) {
      if (keywordPinyin.startsWith(queryLower)) {
        return { keyword: item, score: SCORE_MAP.pinyinFullStartsWith, matchType: 'pinyinFullStartsWith' }
      }
      if (keywordPinyin.includes(queryLower)) {
        return { keyword: item, score: SCORE_MAP.pinyinFullContains, matchType: 'pinyinFullContains' }
      }
    }

    // 别名拼音匹配
    for (const alias of aliases) {
      const aliasInitials = getPinyinInitials(alias)
      if (aliasInitials) {
        if (aliasInitials === queryUpper) {
          return { keyword: item, score: SCORE_MAP.pinyinInitialExact - 5, matchType: 'pinyinInitialExact' }
        }
        if (aliasInitials.startsWith(queryUpper)) {
          return { keyword: item, score: SCORE_MAP.pinyinInitialStartsWith - 5, matchType: 'pinyinInitialStartsWith' }
        }
        if (aliasInitials.includes(queryUpper)) {
          return { keyword: item, score: SCORE_MAP.pinyinInitialContains - 5, matchType: 'pinyinInitialContains' }
        }
      }

      const aliasPinyin = getPinyinFull(alias)
      if (aliasPinyin) {
        if (aliasPinyin.startsWith(queryLower)) {
          return { keyword: item, score: SCORE_MAP.pinyinFullStartsWith - 5, matchType: 'pinyinFullStartsWith' }
        }
        if (aliasPinyin.includes(queryLower)) {
          return { keyword: item, score: SCORE_MAP.pinyinFullContains - 5, matchType: 'pinyinFullContains' }
        }
      }
    }
  }

  return null
}

// 清理查询字符串中的标点符号（处理输入法自动补全引号等情况）
function sanitizeQuery(query: string): string {
  // 移除中英文标点符号，保留字母、数字、中文及空格
  return query.replace(/["""'''\(\)（）\[\]【】\{\}《》<>,.，。!！?？;；:：、·`~@#$%^&*+=|\\\/\-_]/g, '')
}

// 执行匹配
export function matchKeywords(
  query: string,
  keywords: WorldKeywordItem[],
  limit: number = 5
): KeywordMatchResult[] {
  if (!query || !keywords?.length) {
    return []
  }

  // 清理标点符号
  query = sanitizeQuery(query)
  if (!query) {
    return []
  }

  const results: KeywordMatchResult[] = []

  for (const item of keywords) {
    const result = matchSingleKeyword(query, item)
    if (result) {
      results.push(result)
    }
  }

  // 按分数降序排序，取前 limit 个
  results.sort((a, b) => b.score - a.score)

  return results.slice(0, limit)
}

// 通用文本拼音匹配（用于成员搜索等场景）
export function matchText(query: string, text: string): boolean {
  void pinyinReadyVersion.value
  if (!pinyinModule) {
    void ensurePinyinLoaded()
  }
  if (!query || !text) {
    return !query // 空查询匹配所有
  }

  const queryLower = query.toLowerCase()
  const textLower = text.toLowerCase()

  // 直接文本匹配
  if (textLower.includes(queryLower)) {
    return true
  }

  // 拼音匹配
  if (pinyinModule) {
    const queryUpper = query.toUpperCase()
    const initials = getPinyinInitials(text)
    if (initials && initials.includes(queryUpper)) {
      return true
    }
    const pinyin = getPinyinFull(text)
    if (pinyin && pinyin.includes(queryLower)) {
      return true
    }
  }

  return false
}

// 检查拼音库是否已加载
export function isPinyinLoaded(): boolean {
  return pinyinModule !== null
}

export function usePinyinReadyVersion() {
  return pinyinReadyVersion
}
