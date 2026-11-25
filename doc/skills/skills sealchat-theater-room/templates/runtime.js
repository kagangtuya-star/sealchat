/* <sealstage-runtime v1>
 * SealStage —— 小剧场网页组件的统一运行时（粘贴进每个需要同步的组件）。
 * · 在 SealChat iForm 内：通过 Channel Embed SDK 使用 Storage（持久状态）与 Events（瞬时特效）。
 * · 独立预览时：回落到 BroadcastChannel + localStorage，本机多个标签/iframe 之间同步。
 * · Storage / Events 的作用域是 (频道, iForm)：同一个 iForm 被多次放上舞台时共享状态。
 * API：
 *   Stage.ready            Promise，完成连接或回落后 resolve
 *   Stage.mode             'sealchat' | 'local' | 'unsynced'
 *   Stage.gm               当前用户是否为 World Owner/Admin（本地预览恒为 true，?gm=0 可模拟玩家）
 *   Stage.get(key)         读取已同步的状态
 *   Stage.set(key, value)  写入并广播（乐观更新）
 *   Stage.onState(fn)      fn(key, value)，任何状态变化（含首次拉取）
 *   Stage.emit(topic, p)   广播瞬时事件（本地立即触发一次，远端回声自动去重）
 *   Stage.on(topic, fn)    订阅瞬时事件
 *   Stage.onGM(fn)         GM 身份变化
 */
const Stage = (() => {
  const NS = (typeof STAGE_NS === 'string' && STAGE_NS) || 'sealstage'
  const cfg = window.__SEALCHAT_EMBED_CONFIG__ || {}
  const q = new URLSearchParams(location.search)
  const hostOrigin = q.get('hostOrigin') || cfg.hostOrigin || ''
  const sdkUrl = q.get('sdkUrl') || cfg.sdkUrl || ''
  const framed = window.parent !== window
  const state = {}, stateFns = new Set(), gmFns = new Set(), evtFns = {}, seen = new Set(), topics = new Set()
  let client = null, retry = 1000
  const api = { mode: 'local', gm: q.get('gm') !== '0', ready: null }

  const fireState = (k, v) => { if (v === undefined) delete state[k]; else state[k] = v; stateFns.forEach(fn => { try { fn(k, v) } catch (e) { console.error(e) } }) }
  const fireEvt = (topic, p) => {
    if (p && p._id) { if (seen.has(p._id)) return; seen.add(p._id); if (seen.size > 500) seen.delete(seen.values().next().value) }
    (evtFns[topic] || []).forEach(fn => { try { fn(p || {}) } catch (e) { console.error(e) } })
  }
  const setGM = v => { if (api.gm === v) return; api.gm = v; gmFns.forEach(fn => fn(v)) }

  /* ---------- 本地回落：BroadcastChannel + localStorage（均可能被沙箱禁止，全部 try） ---------- */
  let bc = null
  const lsKey = k => `${NS}:${k}`
  const lsRead = () => { try { for (let i = 0; i < localStorage.length; i++) { const k = localStorage.key(i); if (k && k.startsWith(NS + ':')) state[k.slice(NS.length + 1)] = JSON.parse(localStorage.getItem(k)) } } catch {} }
  const lsWrite = (k, v) => { try { v === undefined ? localStorage.removeItem(lsKey(k)) : localStorage.setItem(lsKey(k), JSON.stringify(v)) } catch {} }
  const local = () => {
    try { bc = new BroadcastChannel(NS); bc.onmessage = e => { const m = e.data || {}; if (api.mode === 'sealchat') return; if (m.t === 's') fireState(m.k, m.v); else if (m.t === 'e') fireEvt(m.topic, m.p) } } catch {}
    lsRead(); Object.keys(state).forEach(k => fireState(k, state[k]))
  }
  const bcPost = m => { try { bc && bc.postMessage(m) } catch {} }

  /* ---------- SealChat Embed ---------- */
  const loadSdk = () => window.SealChatEmbed ? Promise.resolve() : new Promise((res, rej) => {
    const s = document.createElement('script'); s.src = sdkUrl; s.async = true
    s.onload = () => window.SealChatEmbed ? res() : rej(new Error('SDK missing')); s.onerror = () => rej(new Error('SDK load failed'))
    document.head.appendChild(s)
  })
  const pullAll = async () => {
    const snap = await client.storage.snapshot()
    const docs = (snap && (snap.documents || snap.items)) || []
    docs.forEach(d => fireState(d.key, d.value))
  }
  const connect = async () => {
    await loadSdk()
    const c = await window.SealChatEmbed.connect({ targetOrigin: hostOrigin || undefined, timeoutMs: 10000 })
    client = c; api.mode = 'sealchat'; retry = 1000
    c.session.onClosed(() => { if (client !== c) return; client = null; setTimeout(() => connect().catch(reconnect), retry) })
    c.storage.onChanged(async ch => {
      if (!ch) return
      if (ch.kind === 'resynced') return pullAll().catch(() => {})
      if (!ch.key) return
      if (ch.kind === 'delete' || ch.op === 'delete') return fireState(ch.key, undefined)
      if ('value' in ch && ch.value !== undefined) return fireState(ch.key, ch.value)
      const doc = await c.storage.get(ch.key).catch(() => null); fireState(ch.key, doc ? doc.value : undefined)
    })
    topics.forEach(t => c.events.on(t, p => fireEvt(t, p)))
    const applyPerm = p => setGM(!!(p && (p.canManageWorld || p.isWorldAdmin || p.isWorldOwner)))
    try { c.permissions.onChanged(applyPerm); applyPerm(await c.permissions.getCurrent()) } catch { setGM(false) }
    await pullAll().catch(() => {})
  }
  const reconnect = err => {
    const code = err && err.code
    if (code && !['HANDSHAKE_FAILED', 'SESSION_EXPIRED', 'CONTEXT_CHANGED', 'TIMEOUT', 'WS_OFFLINE', 'INTERNAL_ERROR'].includes(code)) { console.warn('[SealStage]', code, err.message); return }
    setTimeout(() => connect().catch(reconnect), retry); retry = Math.min(retry * 2, 30000)
  }

  api.ready = (async () => {
    if (framed && hostOrigin && sdkUrl) {
      api.gm = false
      try { await connect() } catch (e) { reconnect(e) }
    } else {
      // 本地预览页与组件同源，可访问 parent；SealChat 里未启用 Embed API 或裸外链时访问会抛错
      let parentSame = false
      try { parentSame = !!window.parent.location.href } catch {}
      if (framed && !parentSame) { api.mode = 'unsynced'; api.gm = false }
      local()
    }
  })()

  api.get = k => state[k]
  api.set = async (k, v) => {
    fireState(k, v)
    if (client) { try { await client.storage.set(k, v) } catch (e) { console.warn('[SealStage] storage.set', e && e.code, e && e.message) } }
    else { lsWrite(k, v); bcPost({ t: 's', k, v }) }
  }
  // 新订阅者立即收到当前全部状态（本地模式的初始状态在组件注册前就已读出）
  api.onState = fn => { stateFns.add(fn); Object.keys(state).forEach(k => fn(k, state[k])); return () => stateFns.delete(fn) }
  api.onGM = fn => { gmFns.add(fn); return () => gmFns.delete(fn) }
  api.on = (topic, fn) => {
    (evtFns[topic] = evtFns[topic] || []).push(fn)
    if (!topics.has(topic)) { topics.add(topic); if (client) client.events.on(topic, p => fireEvt(topic, p)) }
  }
  api.emit = async (topic, payload) => {
    const p = Object.assign({}, payload, { _id: Math.random().toString(36).slice(2) + Date.now().toString(36), _at: Date.now() })
    fireEvt(topic, p)
    if (client) { try { await client.events.publish(topic, p) } catch (e) { console.warn('[SealStage] events.publish', e && e.code) } }
    else bcPost({ t: 'e', topic, p })
  }
  return api
})()
/* </sealstage-runtime> */
