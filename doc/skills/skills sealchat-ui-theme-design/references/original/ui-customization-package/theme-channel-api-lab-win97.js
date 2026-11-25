const doc = document
const ALL_CAPABILITIES = [
  'context.read',
  'user.read',
  'members.read',
  'world.admins.read',
  'characters.read',
  'characterCard.read',
  'characterCard.write',
  'permissions.read',
  'storage.read',
  'storage.write',
  'events.subscribe',
  'events.publish',
  'messages.send',
  'attachments.upload',
  'theater.dialogue.subscribe',
  'theater.character.read',
]
const THEATER_CAPABILITIES = new Set(['theater.dialogue.subscribe', 'theater.character.read'])
const STORAGE_KEY = 'channel-api-lab.win97'
const EVENT_TOPIC = 'channel-api-lab.ping'
const rootId = 'sc-channel-api-lab-' + sc.packageId
doc.getElementById(rootId)?.remove()

const root = doc.createElement('section')
root.id = rootId
root.className = 'sc-channel-api-lab'
root.setAttribute('data-sc-api-lab', 'win97')
root.innerHTML = `
  <div class="sc-api-lab__titlebar">
    <span aria-hidden="true">▣</span>
    <span class="sc-api-lab__titlebar-text">CHANNEL API LAB — Windows 97</span>
    <button type="button" data-action="minimize" title="最小化">_</button>
  </div>
  <div class="sc-api-lab__body">
    <div class="sc-api-lab__summary">
      <div class="sc-api-lab__summary-box"><span class="sc-api-lab__summary-label">Surface</span><span class="sc-api-lab__summary-value" data-field="surface">-</span></div>
      <div class="sc-api-lab__summary-box"><span class="sc-api-lab__summary-label">World</span><span class="sc-api-lab__summary-value" data-field="world">-</span></div>
      <div class="sc-api-lab__summary-box"><span class="sc-api-lab__summary-label">Channel</span><span class="sc-api-lab__summary-value" data-field="channel">-</span></div>
      <div class="sc-api-lab__summary-box"><span class="sc-api-lab__summary-label">API</span><span class="sc-api-lab__summary-value">sc.apiVersion = ${sc.apiVersion}</span></div>
    </div>

    <div class="sc-api-lab__toolbar">
      <button class="sc-api-lab__button" type="button" data-action="refresh">刷新状态</button>
      <button class="sc-api-lab__button" type="button" data-action="read-all">检测全部只读 API</button>
      <button class="sc-api-lab__button" type="button" data-action="clear-log">清空日志</button>
    </div>

    <fieldset class="sc-api-lab__group">
      <legend>Effective Capabilities</legend>
      <div class="sc-api-lab__caps" data-field="capabilities"></div>
      <p class="sc-api-lab__hint">绿色表示当前可用。黄色 Theater 项在普通世界脚本没有 provider 时预期不可用。红色表示能力声明、成员身份、只读状态或频道权限未满足。</p>
    </fieldset>

    <fieldset class="sc-api-lab__group">
      <legend>Storage / Events</legend>
      <div class="sc-api-lab__row">
        <button class="sc-api-lab__button" type="button" data-action="storage-set">写入测试值</button>
        <button class="sc-api-lab__button" type="button" data-action="storage-get">读取测试值</button>
        <button class="sc-api-lab__button" type="button" data-action="storage-delete">删除测试值</button>
        <button class="sc-api-lab__button" type="button" data-action="event-ping">发布事件 Ping</button>
      </div>
      <p class="sc-api-lab__hint">Storage key：<code>${STORAGE_KEY}</code>；Event topic：<code>${EVENT_TOPIC}</code>。</p>
    </fieldset>

    <fieldset class="sc-api-lab__group">
      <legend>Message / Character Card / Attachment</legend>
      <div class="sc-api-lab__row">
        <input class="sc-api-lab__input" data-field="message" value="[Channel API Lab] messages.send 测试" aria-label="测试消息">
        <button class="sc-api-lab__button" type="button" data-action="message-send">发送 OOC 测试消息</button>
      </div>
      <p class="sc-api-lab__hint">发送消息会真实写入当前频道。</p>

      <textarea class="sc-api-lab__textarea" data-field="card-patch" placeholder='输入要写入当前人物卡的 JSON 对象，例如 {"hp": 8}'></textarea>
      <div class="sc-api-lab__row">
        <button class="sc-api-lab__button" type="button" data-action="card-write">执行人物卡写入</button>
        <span class="sc-api-lab__danger">此操作会修改当前活动人物卡；空对象不会执行。</span>
      </div>

      <div class="sc-api-lab__row" style="margin-top: 6px">
        <input class="sc-api-lab__file" data-field="image" type="file" accept="image/*">
        <button class="sc-api-lab__button" type="button" data-action="image-upload">上传图片</button>
      </div>
    </fieldset>

    <fieldset class="sc-api-lab__group">
      <legend>Result Log</legend>
      <pre class="sc-api-lab__output" data-field="output">CHANNEL API LAB ready.</pre>
    </fieldset>
  </div>
  <div class="sc-api-lab__statusbar">
    <span class="sc-api-lab__status" data-field="status">Initializing...</span>
    <span class="sc-api-lab__status" data-field="effective-count">0 / ${ALL_CAPABILITIES.length}</span>
    <span class="sc-api-lab__status">WIN97</span>
  </div>
`
doc.body.append(root)
sc.cleanup(() => root.remove())

const $ = selector => root.querySelector(selector)
const output = $('[data-field="output"]')
const statusField = $('[data-field="status"]')
const capField = $('[data-field="capabilities"]')
const effectiveCountField = $('[data-field="effective-count"]')

const stringify = value => {
  if (value === undefined) return 'undefined'
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

const errorInfo = error => ({
  code: error?.code || error?.name || 'ERROR',
  message: error?.message || String(error),
  details: error?.details,
})

const appendLog = (label, value, kind = 'OK') => {
  const time = new Date().toLocaleTimeString()
  const text = `[${time}] [${kind}] ${label}\n${stringify(value)}\n\n`
  output.textContent = text + output.textContent
}

const setStatus = text => {
  statusField.textContent = text
}

const channel = sc.channel()
if (!channel) {
  setStatus('No active Channel Extension Context')
  appendLog('sc.channel()', '当前 Surface 没有有效频道。请在正常频道聊天页启用此包。', 'SKIP')
  return
}

const currentCapabilities = () => new Set(channel.capabilities || [])
const renderCapabilities = () => {
  const effective = currentCapabilities()
  capField.replaceChildren()
  for (const capability of ALL_CAPABILITIES) {
    const row = doc.createElement('div')
    row.className = 'sc-api-lab__cap'
    const name = doc.createElement('code')
    name.textContent = capability
    const badge = doc.createElement('span')
    badge.className = 'sc-api-lab__badge'
    if (effective.has(capability)) {
      badge.classList.add('is-ok')
      badge.textContent = 'AVAILABLE'
    } else if (THEATER_CAPABILITIES.has(capability)) {
      badge.classList.add('is-optional')
      badge.textContent = 'NO PROVIDER'
    } else {
      badge.classList.add('is-denied')
      badge.textContent = 'DENIED'
    }
    row.append(name, badge)
    capField.append(row)
  }
  effectiveCountField.textContent = `${effective.size} / ${ALL_CAPABILITIES.length}`
}

const renderContextSummary = async () => {
  const local = sc.context()
  $('[data-field="surface"]').textContent = local.surface || '-'
  $('[data-field="world"]').textContent = local.worldId || '-'
  $('[data-field="channel"]').textContent = local.channelId || '-'
  renderCapabilities()
  if (!currentCapabilities().has('context.read')) {
    setStatus('context.read unavailable')
    return
  }
  try {
    const context = await channel.context.get()
    $('[data-field="world"]').textContent = context?.world?.name || context?.world?.id || local.worldId || '-'
    $('[data-field="channel"]').textContent = context?.channel?.name || context?.channel?.id || local.channelId || '-'
    setStatus(`${context?.connection?.state || 'unknown'} · contextVersion ${context?.contextVersion ?? '-'}`)
  } catch (error) {
    setStatus(`${error?.code || 'ERROR'} · ${error?.message || error}`)
  }
}

const test = async (capability, label, operation) => {
  if (!currentCapabilities().has(capability)) {
    appendLog(label, `${capability} 当前不可用`, THEATER_CAPABILITIES.has(capability) ? 'N/A' : 'SKIP')
    return null
  }
  try {
    const result = await operation()
    appendLog(label, result)
    return result
  } catch (error) {
    appendLog(label, errorInfo(error), 'FAIL')
    return null
  }
}

const runReadChecks = async () => {
  setStatus('Running read-only checks...')
  await test('context.read', 'context.get()', () => channel.context.get())
  await test('context.read', 'member.getCurrent()', () => channel.member.getCurrent())
  await test('context.read', 'channel.getState()', () => channel.channel.getState())
  await test('context.read', 'connection.getState()', () => channel.connection.getState())
  await test('user.read', 'user.getCurrent()', () => channel.user.getCurrent())
  await test('members.read', 'members.list(online)', () => channel.members.list({ scope: 'online' }))
  await test('members.read', 'members.list(guild)', () => channel.members.list({ scope: 'guild' }))
  if (currentCapabilities().has('members.read')) {
    await test('world.admins.read', 'members.list(world-admins)', () => channel.members.list({ scope: 'world-admins' }))
  }
  const characters = await test('characters.read', 'characters.list()', () => channel.characters.list())
  await test('characters.read', 'characters.getCurrent()', () => channel.characters.getCurrent())
  await test('characterCard.read', 'characterCard.getStatus()', () => channel.characterCard.getStatus())
  await test('characterCard.read', 'characterCard.getCurrent()', () => channel.characterCard.getCurrent())
  const snapshots = await test('characterCard.read', 'characterCard.listSnapshots()', () => channel.characterCard.listSnapshots())
  const identityId = Array.isArray(snapshots) && snapshots[0]?.identityId
    ? snapshots[0].identityId
    : Array.isArray(characters) && characters[0]?.id
      ? characters[0].id
      : ''
  if (identityId) {
    await test('characterCard.read', 'characterCard.getSnapshot()', () => channel.characterCard.getSnapshot({ identityId }))
  }
  await test('permissions.read', 'permissions.getCurrent()', () => channel.permissions.getCurrent())
  await test('storage.read', 'storage.get(test-key)', () => channel.storage.get(STORAGE_KEY))
  await test('storage.read', 'storage.list(prefix)', () => channel.storage.list({ prefix: 'channel-api-lab' }))
  await test('storage.read', 'storage.snapshot()', () => channel.storage.snapshot())
  await test('events.subscribe', 'events.subscribe(test-topic)', () => channel.request('events.subscribe', { topic: EVENT_TOPIC }))
  renderCapabilities()
  setStatus('Read-only checks finished')
}

const register = off => {
  if (typeof off === 'function') sc.cleanup(off)
}

register(channel.on('capabilities.changed', capabilities => {
  appendLog('capabilities.changed', capabilities, 'EVENT')
  renderCapabilities()
  void renderContextSummary()
}))
register(channel.context.onChanged(value => {
  appendLog('context.changed', value, 'EVENT')
  void renderContextSummary()
}))
register(channel.connection.onChanged(value => {
  appendLog('connection.changed', value, 'EVENT')
  setStatus(value?.state || 'connection changed')
}))
register(channel.members.onChanged(value => appendLog('members.changed', value, 'EVENT')))
register(channel.characters.onChanged(value => appendLog('characters.changed', value, 'EVENT')))
register(channel.permissions.onChanged(value => appendLog('permissions.changed', value, 'EVENT')))
register(channel.storage.onChanged(value => appendLog('storage.changed', value, 'EVENT')))
register(channel.events.on(EVENT_TOPIC, value => appendLog(`event:${EVENT_TOPIC}`, value, 'EVENT')))
register(channel.session.onClosed(() => {
  setStatus('Session closed')
  appendLog('session.closed', 'Channel Extension Context 已关闭。', 'EVENT')
}))

sc.listen(root, 'click', async event => {
  const button = event.target.closest('button[data-action]')
  if (!button || !root.contains(button)) return
  const action = button.dataset.action
  if (action === 'minimize') {
    root.classList.toggle('is-minimized')
    button.textContent = root.classList.contains('is-minimized') ? '□' : '_'
    return
  }
  if (action === 'clear-log') {
    output.textContent = ''
    return
  }
  if (action === 'refresh') {
    await renderContextSummary()
    appendLog('client.capabilities', [...channel.capabilities])
    return
  }
  if (action === 'read-all') {
    await runReadChecks()
    return
  }
  if (action === 'storage-set') {
    await test('storage.write', 'storage.set(test-key)', () => channel.storage.set(STORAGE_KEY, {
      packageId: sc.packageId,
      at: new Date().toISOString(),
      note: 'CHANNEL API LAB Windows 97',
    }))
    return
  }
  if (action === 'storage-get') {
    await test('storage.read', 'storage.get(test-key)', () => channel.storage.get(STORAGE_KEY))
    return
  }
  if (action === 'storage-delete') {
    await test('storage.write', 'storage.delete(test-key)', () => channel.storage.delete(STORAGE_KEY))
    return
  }
  if (action === 'event-ping') {
    await test('events.publish', 'events.publish(test-topic)', () => channel.events.publish(EVENT_TOPIC, {
      packageId: sc.packageId,
      at: Date.now(),
    }))
    return
  }
  if (action === 'message-send') {
    const text = $('[data-field="message"]').value.trim()
    if (!text) {
      appendLog('messages.send()', '测试消息为空。', 'SKIP')
      return
    }
    await test('messages.send', 'messages.send(OOC)', () => channel.messages.send({ text, icMode: 'ooc' }))
    return
  }
  if (action === 'card-write') {
    const raw = $('[data-field="card-patch"]').value.trim()
    if (!raw) {
      appendLog('characterCard.updateAttrs()', '未填写 JSON；为避免误改人物卡，本次不执行。', 'SKIP')
      return
    }
    try {
      const patch = JSON.parse(raw)
      if (!patch || typeof patch !== 'object' || Array.isArray(patch) || Object.keys(patch).length === 0) {
        appendLog('characterCard.updateAttrs()', '请输入至少包含 1 个字段的 JSON 对象。', 'SKIP')
        return
      }
      await test('characterCard.write', 'characterCard.updateAttrs()', () => channel.characterCard.updateAttrs(patch))
    } catch (error) {
      appendLog('characterCard.updateAttrs()', { code: 'INVALID_JSON', message: error.message }, 'FAIL')
    }
    return
  }
  if (action === 'image-upload') {
    const file = $('[data-field="image"]').files?.[0]
    if (!file) {
      appendLog('attachments.uploadImage()', '请先选择一张图片。', 'SKIP')
      return
    }
    await test('attachments.upload', 'attachments.uploadImage()', () => channel.attachments.uploadImage(file, { filename: file.name }))
  }
})

void renderContextSummary()
sc.timeout(() => {
  renderCapabilities()
  void renderContextSummary()
}, 300)
