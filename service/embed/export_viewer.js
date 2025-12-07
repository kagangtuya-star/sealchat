(function () {
  const app = document.getElementById('app')

  function formatTime(value) {
    if (!value) return '--'
    try {
      const date = new Date(value)
      if (Number.isNaN(date.getTime())) return value
      return date.toLocaleString()
    } catch (err) {
      return value
    }
  }

  function stripHTML(html) {
    if (!html) return ''
    const tmp = document.createElement('div')
    tmp.innerHTML = html
    return tmp.textContent || tmp.innerText || ''
  }

  function initials(name) {
    if (!name) return '??'
    const clean = name.trim()
    if (!clean) return '??'
    return clean.slice(0, 2).toUpperCase()
  }

  function createChip(text) {
    const span = document.createElement('span')
    span.className = 'viewer-chip'
    span.textContent = text
    return span
  }

  function renderEmpty() {
    const wrapper = document.createElement('div')
    wrapper.className = 'viewer-shell viewer-empty'
    wrapper.innerHTML = '<p>未找到导出数据，请重新生成导出文件。</p>'
    app.appendChild(wrapper)
  }

  function renderIndex(manifest) {
    document.body.dataset.palette = manifest.display_options?.palette || 'night'
    const shell = document.createElement('div')
    shell.className = 'viewer-shell'

    const header = document.createElement('div')
    header.className = 'viewer-header'
    header.innerHTML = `<h1>${manifest.channel_name}</h1>`

    const meta = document.createElement('div')
    meta.className = 'viewer-meta'
    meta.appendChild(createChip(`分片 ${manifest.part_total}`))
    meta.appendChild(createChip(`消息 ${manifest.total_messages}`))
    meta.appendChild(
      createChip(`切片 ${manifest.slice_limit} / 并发 ${manifest.max_concurrency}`),
    )
    header.appendChild(meta)

    shell.appendChild(header)

    const grid = document.createElement('div')
    grid.className = 'parts-grid'
    if (Array.isArray(manifest.parts)) {
      manifest.parts.forEach((part) => {
        const card = document.createElement('div')
        card.className = 'parts-card'
        card.innerHTML = `
          <h3>Part ${part.part_index}/${part.part_total}</h3>
          <p>消息：${part.messages}</p>
          <p>范围：${formatTime(part.slice_start)} → ${formatTime(part.slice_end)}</p>
          ${part.sha256 ? `<p>SHA256：${part.sha256.slice(0, 12)}…</p>` : ''}
          <a href="${part.file}">打开分片</a>
        `
        grid.appendChild(card)
      })
    }
    shell.appendChild(grid)
    app.appendChild(shell)
  }

  function renderPart(payload) {
    const displayOptions = {
      layout: payload.display_options?.layout || 'bubble',
      palette: payload.display_options?.palette || 'night',
      showAvatar: payload.display_options?.showAvatar !== false,
    }
    document.body.dataset.palette = displayOptions.palette
    document.body.dataset.layout = displayOptions.layout
    document.body.dataset.hideAvatar = displayOptions.showAvatar ? 'false' : 'true'

    const shell = document.createElement('div')
    shell.className = 'viewer-shell'

    const header = document.createElement('div')
    header.className = 'viewer-header'
    header.innerHTML = `<h1>${payload.channel_name}</h1>`

    const meta = document.createElement('div')
    meta.className = 'viewer-meta'
    meta.appendChild(
      createChip(`分片 ${payload.part_index || 1} / ${payload.part_total || 1}`),
    )
    meta.appendChild(createChip(`消息 ${payload.messages?.length || 0}`))
    meta.appendChild(
      createChip(
        `时间 ${formatTime(payload.slice_start || payload.start_time)} → ${formatTime(
          payload.slice_end || payload.end_time,
        )}`,
      ),
    )
    header.appendChild(meta)
    shell.appendChild(header)

    const controls = document.createElement('div')
    controls.className = 'viewer-controls'
    controls.innerHTML = `
      <input id="viewer-search-input" placeholder="关键词 / 正则表达式" />
      <label><input type="checkbox" id="viewer-case" />区分大小写</label>
      <label><input type="checkbox" id="viewer-regex" />正则</label>
      <button type="button" id="viewer-prev">上一条</button>
      <button type="button" id="viewer-next">下一条</button>
      <span class="viewer-chip" id="viewer-counter">0 / 0</span>
    `
    shell.appendChild(controls)

    const display = document.createElement('div')
    display.className = 'viewer-display'
    display.innerHTML = `
      <div>
        <button type="button" data-action="layout-bubble">气泡</button>
        <button type="button" data-action="layout-compact">紧凑</button>
      </div>
      <div>
        <button type="button" data-action="palette-day">日间</button>
        <button type="button" data-action="palette-night">夜间</button>
      </div>
      <div>
        <button type="button" data-action="toggle-avatar">头像 ${displayOptions.showAvatar ? '开' : '关'}</button>
      </div>
    `
    shell.appendChild(display)

    const list = document.createElement('div')
    list.className = 'viewer-message-list'
    list.id = 'viewer-message-list'
    if (Array.isArray(payload.messages)) {
      payload.messages.forEach((msg) => {
        list.appendChild(createMessageElement(msg))
      })
    }
    shell.appendChild(list)

    if ((payload.part_total || 1) > 1) {
      const nav = document.createElement('div')
      nav.className = 'viewer-nav'
      const idx = payload.part_index || 1
      const total = payload.part_total || 1
      nav.innerHTML = `
        ${idx > 1
          ? `<a href="part-${String(idx - 1).padStart(3, '0')}.html">上一分片</a>`
          : '<span></span>'
        }
        <a href="../index.html">返回索引</a>
        ${idx < total
          ? `<a href="part-${String(idx + 1).padStart(3, '0')}.html">下一分片</a>`
          : '<span></span>'
        }
      `
      shell.appendChild(nav)
    }

    app.appendChild(shell)
    attachSearch(payload)
    attachDisplayControls(payload, displayOptions)
  }

  function attachDisplayControls(payload, displayOptions) {
    document.body.dataset.layout = displayOptions.layout
    document.body.dataset.palette = displayOptions.palette
    document.body.dataset.hideAvatar = displayOptions.showAvatar ? 'false' : 'true'

    document.querySelectorAll('.viewer-display button').forEach((btn) => {
      btn.addEventListener('click', () => {
        const action = btn.getAttribute('data-action')
        switch (action) {
          case 'layout-bubble':
            document.body.dataset.layout = 'bubble'
            break
          case 'layout-compact':
            document.body.dataset.layout = 'compact'
            break
          case 'palette-day':
            document.body.dataset.palette = 'day'
            break
          case 'palette-night':
            document.body.dataset.palette = 'night'
            break
          case 'toggle-avatar':
            document.body.dataset.hideAvatar =
              document.body.dataset.hideAvatar === 'true' ? 'false' : 'true'
            btn.textContent =
              '头像 ' + (document.body.dataset.hideAvatar === 'true' ? '关' : '开')
            break
        }
      })
    })
  }

  function attachSearch(payload) {
    const input = document.getElementById('viewer-search-input')
    const counter = document.getElementById('viewer-counter')
    const prevBtn = document.getElementById('viewer-prev')
    const nextBtn = document.getElementById('viewer-next')
    const caseBox = document.getElementById('viewer-case')
    const regexBox = document.getElementById('viewer-regex')
    const messages = Array.from(document.querySelectorAll('.viewer-message'))

    let hits = []
    let activeIndex = -1

    function updateHits() {
      const raw = input.value.trim()
      const useRegex = regexBox.checked
      const caseSensitive = caseBox.checked
      hits = []
      activeIndex = -1
      messages.forEach((msg) => msg.classList.remove('search-hit', 'search-hit-active'))
      if (!raw) {
        counter.textContent = '0 / 0'
        return
      }
      let matcher
      try {
        if (useRegex) {
          const reg = new RegExp(raw, caseSensitive ? 'g' : 'gi')
          matcher = (text) => reg.test(text)
        } else {
          const needle = caseSensitive ? raw : raw.toLowerCase()
          matcher = (text) => text.includes(needle)
        }
      } catch (err) {
        counter.textContent = '无效表达式'
        return
      }
      messages.forEach((item, idx) => {
        const haystack = caseSensitive
          ? item.dataset.searchText || ''
          : (item.dataset.searchText || '').toLowerCase()
        if (matcher(haystack)) {
          item.classList.add('search-hit')
          hits.push({ element: item, index: idx })
        }
      })
      counter.textContent = `${hits.length ? 1 : 0} / ${hits.length}`
      if (hits.length) {
        activeIndex = 0
        scrollToHit()
      }
    }

    function scrollToHit() {
      if (activeIndex < 0 || activeIndex >= hits.length) return
      const target = hits[activeIndex].element
      target.scrollIntoView({ behavior: 'smooth', block: 'center' })
      target.classList.add('search-hit-active')
      setTimeout(() => target.classList.remove('search-hit-active'), 1200)
      counter.textContent = `${activeIndex + 1} / ${hits.length}`
    }

    function jump(delta) {
      if (!hits.length) return
      activeIndex = (activeIndex + delta + hits.length) % hits.length
      scrollToHit()
    }

    input.addEventListener('input', updateHits)
    caseBox.addEventListener('change', updateHits)
    regexBox.addEventListener('change', updateHits)
    prevBtn.addEventListener('click', () => jump(-1))
    nextBtn.addEventListener('click', () => jump(1))

    updateHits()
  }

  function createMessageElement(msg) {
    const name = msg.sender_name || '匿名'
    const article = document.createElement('article')
    article.className = 'viewer-message'
    article.dataset.messageId = msg.id
    article.dataset.icMode = (msg.ic_mode || 'ic').toLowerCase()
    // 使用 content_html 进行渲染，fallback 到 content
    const displayContent = msg.content_html || msg.content || ''
    article.dataset.searchText = (stripHTML(displayContent) + ' ' + name).toLowerCase()

    const avatar = document.createElement('div')
    avatar.className = 'viewer-message__avatar'
    avatar.style.background = msg.sender_color || 'rgba(148, 163, 184, 0.35)'
    if (msg.sender_avatar && msg.sender_avatar.startsWith('data:')) {
      const img = document.createElement('img')
      img.src = msg.sender_avatar
      img.alt = name
      avatar.appendChild(img)
    } else {
      avatar.textContent = initials(name)
    }
    article.appendChild(avatar)

    const main = document.createElement('div')
    main.className = 'viewer-message__main'

    const header = document.createElement('div')
    header.className = 'viewer-message__header'
    header.innerHTML = `
      <div class="viewer-message__title">
        <strong>${name}</strong>
        <span class="viewer-message__tag">${(msg.ic_mode || 'ic').toUpperCase()}</span>
      </div>
      <span>${formatTime(msg.created_at)}</span>
    `
    main.appendChild(header)

    const body = document.createElement('div')
    body.className = 'viewer-message__body'
    body.innerHTML = displayContent
    main.appendChild(body)

    article.appendChild(main)
    return article
  }

  if (!app) {
    return
  }
  if (window.__EXPORT_DATA__) {
    renderPart(window.__EXPORT_DATA__)
  } else if (window.__EXPORT_INDEX__) {
    renderIndex(window.__EXPORT_INDEX__)
  } else {
    renderEmpty()
  }
})()
