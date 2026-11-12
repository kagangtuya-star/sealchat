// LOG 隔离预览运行时：只由应用注入（带 nonce），在 sandbox="allow-scripts allow-modals" 的 srcdoc iframe 中执行。
// 不发起网络请求（CSP connect-src 'none'），不访问外层；所有通信经 MessageChannel 端口，且先校验会话令牌。
// 分页引擎：逐块测量（DOM 溢出检测）+ 文本二分切分，章节为分页单元；旧修订的结果一律丢弃。
(function () {
  'use strict';
  var TOKEN = '__LOG_FRAME_TOKEN__';
  var MM = 96 / 25.4;
  var CLOSE = '，。、；：？！）」』】〉》〕”’…—,.;:?!)]}%';
  var OPEN = '（「『【〈《〔“‘([{';
  var IMAGE_MIME = /^image\/(png|jpeg|webp|gif|avif)$/;
  var FONT_MIME = /^(font\/(ttf|otf|sfnt|woff|woff2)|application\/(x-font-ttf|x-font-otf|font-sfnt))$/;
  var port = null;
  var revision = 0;
  var assetUrls = new Map();
  var current = { doc: null, design: false, canResizeImages: false, mode: 'paged', spread: false, selection: new Set(), relocation: null, multi: false, zoom: 1 };
  var root = document.getElementById('log-root');
  var pkgStyle = document.getElementById('log-package');
  var pageStyle = document.getElementById('log-page-rule');
  var scrollTimer = 0;
  var drag = null;
  var imageResize = null;
  var printBusy = false;

  function post(msg) { if (port) port.postMessage(msg); }

  window.addEventListener('message', function onInit(e) {
    var d = e.data;
    if (e.source !== window.parent || !d || d.type !== 'log-frame-init' || d.token !== TOKEN || !e.ports || !e.ports[0]) return;
    window.removeEventListener('message', onInit);
    port = e.ports[0];
    port.onmessage = function (ev) { handle(ev.data); };
    post({ type: 'ready' });
  });

  function handle(msg) {
    if (!msg || typeof msg.type !== 'string') return;
    switch (msg.type) {
      case 'assets': receiveAssets(msg.items || []); break;
      case 'render': render(msg).catch(function (err) { post({ type: 'error', revision: msg.revision, message: String(err && err.message || err) }); }); break;
      case 'select': current.selection = new Set(msg.ids || []); applySelection(); break;
      case 'relocate': current.relocation = msg.state || null; applyRelocation(); break;
      case 'multi': current.multi = msg.active === true; applyMulti(); break;
      case 'scrollTo': scrollToNode(msg.nodeId, msg.flash !== false); break;
      case 'zoom': endImageResize(false); current.zoom = clampZoom(msg.zoom); root.style.zoom = String(current.zoom); break;
      case 'canvas': setCanvas(msg.color, msg.dark, msg.accent); break;
      case 'print': printDocument(msg); break;
      case 'dispose': dispose(); break;
      default: break;
    }
  }

  function clampZoom(z) { z = Number(z) || 1; return Math.min(2, Math.max(0.3, z)); }

  // 工作区底色跟随外层主题；只接受十六进制或 rgb()/rgba() 颜色，其余输入恢复默认。
  var CANVAS_COLOR = /^(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\))$/i;
  function setCanvas(color, dark, accent) {
    var html = document.documentElement;
    if (typeof color === 'string' && CANVAS_COLOR.test(color.trim())) html.style.setProperty('--log-workspace-bg', color.trim());
    else html.style.removeProperty('--log-workspace-bg');
    html.classList.toggle('log-canvas-dark', dark === true);
    if (typeof accent === 'string' && CANVAS_COLOR.test(accent.trim())) html.style.setProperty('--log-selection', accent.trim());
    else html.style.removeProperty('--log-selection');
  }

  async function printDocument(msg) {
    var requestId = Number(msg.requestId) || 0;
    if (printBusy) {
      post({ type: 'printError', requestId: requestId, message: '已有打印任务正在进行' });
      return;
    }
    var host = root.querySelector('.log-pages');
    var pages = host ? Array.prototype.filter.call(host.children, function (el) { return el.classList && el.classList.contains('log-page'); }) : [];
    if (current.mode !== 'paged' || !host || !pages.length) {
      post({ type: 'printError', requestId: requestId, message: '请先完成纸张模式排版' });
      return;
    }
    var total = pages.length;
    var fromPage = msg.fromPage == null ? 1 : Math.floor(Number(msg.fromPage));
    var toPage = msg.toPage == null ? total : Math.floor(Number(msg.toPage));
    if (!Number.isFinite(fromPage) || !Number.isFinite(toPage) || fromPage < 1 || toPage < fromPage || toPage > total) {
      post({ type: 'printError', requestId: requestId, message: '打印页码范围无效' });
      return;
    }
    var limited = fromPage !== 1 || toPage !== total;
    var scrollX = window.scrollX;
    var scrollY = window.scrollY;
    printBusy = true;
    document.body.classList.add('log-printing');
    try {
      // 分段打印时直接把范围外页面从渲染树临时摘下，避免浏览器为 100+ 页建立完整打印布局树。
      // 节点本身仍由 pages 引用持有，打印结束后原样恢复，不重新分页、不复制图片内存。
      if (limited) host.replaceChildren.apply(host, pages.slice(fromPage - 1, toPage));
      await nextFrame();
      await nextFrame();
      window.print();
      post({ type: 'printDone', requestId: requestId });
    } catch (err) {
      post({ type: 'printError', requestId: requestId, message: String(err && err.message || err) });
    } finally {
      if (limited) host.replaceChildren.apply(host, pages);
      document.body.classList.remove('log-printing');
      window.scrollTo(scrollX, scrollY);
      printBusy = false;
    }
  }

  function receiveAssets(items) {
    for (var i = 0; i < items.length; i++) {
      var it = items[i];
      if (!it || typeof it.token !== 'string' || assetUrls.has(it.token)) continue;
      var mime = String(it.mime || '').split(';')[0].trim().toLowerCase();
      if (it.data instanceof ArrayBuffer && (IMAGE_MIME.test(mime) || FONT_MIME.test(mime))) {
        assetUrls.set(it.token, URL.createObjectURL(new Blob([it.data], { type: mime })));
      } else {
        assetUrls.set(it.token, null);
      }
    }
  }

  function dispose() {
    endImageResize(false);
    endDrag(false);
    clearTimeout(scrollTimer);
    assetUrls.forEach(function (url) { if (url) URL.revokeObjectURL(url); });
    assetUrls.clear();
    revision += 1;
    root.replaceChildren();
    if (port) { port.close(); port = null; }
  }

  // ---------- 渲染 ----------

  function cssWithAssets(css) {
    return String(css || '').replace(/__LOGASSET_([A-Za-z0-9:_-]+)__/g, function (_, token) {
      var url = assetUrls.get(token);
      return url || 'about:blank';
    });
  }

  var appliedVars = [];
  var appliedAttrs = [];
  function applyRoot(doc) {
    var html = document.documentElement;
    // 先清除上一次的设计变量，避免切换样式后残留旧变量。
    appliedVars.forEach(function (k) { html.style.removeProperty(k); });
    appliedAttrs.forEach(function (k) { html.removeAttribute(k); });
    appliedVars = [];
    appliedAttrs = [];
    var vars = doc.rootVars || {};
    Object.keys(vars).forEach(function (k) {
      if (/^--[a-zA-Z0-9_-]+$/.test(k)) { html.style.setProperty(k, String(vars[k])); appliedVars.push(k); }
    });
    var attrs = doc.rootAttrs || {};
    Object.keys(attrs).forEach(function (k) {
      if (/^data-log-[a-z0-9-]+$/.test(k)) { html.setAttribute(k, String(attrs[k])); appliedAttrs.push(k); }
    });
    pkgStyle.textContent = cssWithAssets(doc.css);
    var p = doc.page;
    pageStyle.textContent = '@page { size: ' + p.widthMm + 'mm ' + p.heightMm + 'mm; margin: 0; }';
    document.body.classList.toggle('log-design', !!current.design);
  }

  function geometry(p, columns) {
    var m = p.marginsMm;
    return {
      w: p.widthMm * MM, h: p.heightMm * MM,
      top: m.top * MM, right: m.right * MM, bottom: m.bottom * MM, left: m.left * MM,
      gutter: p.gutterMm * MM, columns: columns,
    };
  }

  function setAssetSrc(img) {
    var token = img.getAttribute('data-log-asset');
    var url = token ? assetUrls.get(token) : null;
    if (url) { img.src = url; return; }
    var holder = document.createElement('span');
    holder.className = 'log-img-missing';
    holder.textContent = img.getAttribute('alt') || '图片不可用';
    img.replaceWith(holder);
  }

  function createBlock(b) {
    var el = document.createElement('div');
    el.className = 'log-node log-node--' + b.kind + (b.classes && b.classes.length ? ' ' + b.classes.join(' ') : '');
    el.setAttribute('data-log-node', b.id);
    el.setAttribute('data-log-kind', b.kind);
    el.setAttribute('data-log-fragment', '0');
    var attrs = b.attrs || {};
    Object.keys(attrs).forEach(function (k) { if (/^data-log-[a-z0-9-]+$/.test(k)) el.setAttribute(k, String(attrs[k])); });
    var vars = b.vars || {};
    Object.keys(vars).forEach(function (k) { if (/^--[a-zA-Z0-9_-]+$/.test(k)) el.style.setProperty(k, String(vars[k])); });
    el.innerHTML = b.html;
    var imgs = el.querySelectorAll('img');
    for (var i = 0; i < imgs.length; i++) {
      var img = imgs[i];
      if (img.hasAttribute('data-log-resizable-image')) {
        var box = document.createElement('span');
        box.className = 'log-image-box';
        img.replaceWith(box);
        box.appendChild(img);
      }
      setAssetSrc(img);
    }
    if (b.breakBefore) el.setAttribute('data-break-before', '1');
    if (b.breakAfter) el.setAttribute('data-break-after', '1');
    return el;
  }

  function decodeImages(container) {
    var imgs = Array.prototype.slice.call(container.querySelectorAll('img'));
    return Promise.all(imgs.map(function (img) {
      return new Promise(function (resolve) {
        var timer;
        function done() {
          clearTimeout(timer);
          img.removeEventListener('load', done);
          img.removeEventListener('error', broken);
          resolve();
        }
        function broken() { markBroken(img); done(); }
        timer = setTimeout(done, 4000);
        img.addEventListener('load', done);
        img.addEventListener('error', broken);
        if (img.decode) img.decode().then(done, broken);
        else if (img.complete) { if (img.naturalWidth) done(); else broken(); }
      });
    }));
  }

  function markBroken(img) {
    if (!img.isConnected) return;
    var holder = document.createElement('span');
    holder.className = 'log-img-missing';
    holder.textContent = '图片加载失败';
    img.replaceWith(holder);
  }

  function clampImageRatio(value) { return Math.min(1, Math.max(0.15, value)); }

  function contentWidth(el) {
    var cs = getComputedStyle(el);
    return Math.max(0, el.clientWidth - (parseFloat(cs.paddingLeft) || 0) - (parseFloat(cs.paddingRight) || 0));
  }

  function imageLimits(img) {
    var box = img.parentElement;
    var node = img.closest('.log-node');
    var column = contentWidth(node);
    var available = contentWidth(box.parentElement) || column;
    var heightRule = getComputedStyle(img).maxHeight;
    var maxHeight = /px$/.test(heightRule) ? parseFloat(heightRule) : 0;
    var max = Math.min(column, available);
    if (maxHeight > 0 && img.naturalHeight > 0) max = Math.min(max, maxHeight * img.naturalWidth / img.naturalHeight);
    return { column: column, max: max, min: Math.min(column * 0.15, max) };
  }

  function sizeImages(node) {
    var imgs = node.querySelectorAll('img[data-log-resizable-image]');
    for (var i = 0; i < imgs.length; i++) {
      var img = imgs[i];
      if (!img.naturalWidth || !img.naturalHeight) continue;
      var limits = imageLimits(img);
      var saved = Number(img.getAttribute('data-log-image-width'));
      var explicit = saved > 0 && Number.isFinite(saved);
      var rendered = img.getBoundingClientRect ? img.getBoundingClientRect().width : 0;
      var width = explicit ? limits.column * clampImageRatio(saved) : Math.min(img.naturalWidth, limits.column * 0.75);
      if (!explicit && rendered > 0) width = Math.min(width, rendered);
      img.parentElement.style.width = Math.min(width, limits.max) + 'px';
      img.parentElement.classList.add('log-image-box--sized');
    }
  }

  async function waitFonts() {
    if (!document.fonts || !document.fonts.ready) return Promise.resolve();
    // 当前修订的 Blob 字体已注入；显式加载与 FontFaceSet ready 共用一个总超时，异常字体不阻塞分页。
    var loads = [document.fonts.ready];
    document.fonts.forEach(function (face) {
      if (/^['"]?sc-log-platform-/.test(face.family)) loads.push(face.load().catch(function () {}));
    });
    var timer = 0;
    try {
      await Promise.race([
        Promise.all(loads),
        new Promise(function (r) { timer = setTimeout(r, 3000); }),
      ]);
    } finally {
      if (timer) clearTimeout(timer);
    }
  }

  function nextFrame() { return new Promise(function (r) { setTimeout(r, 0); }); }

  function isPageBreak(v) { return v === 'page' || v === 'left' || v === 'right' || v === 'recto' || v === 'verso' || v === 'always'; }

  function overflows(el) {
    return el.scrollHeight > el.clientHeight + 1 || el.scrollWidth > el.clientWidth + 1;
  }

  function newPage(host, g, doc, index) {
    var page = document.createElement('section');
    page.className = 'log-page';
    page.setAttribute('data-page', String(index + 1));
    page.setAttribute('aria-label', '第 ' + (index + 1) + ' 页');
    page.style.width = g.w + 'px';
    page.style.height = g.h + 'px';
    page.style.containIntrinsicSize = g.w + 'px ' + g.h + 'px';
    if (doc.backgroundToken && assetUrls.get(doc.backgroundToken)) page.style.backgroundImage = 'url("' + assetUrls.get(doc.backgroundToken) + '")';
    var content = document.createElement('div');
    content.className = 'log-page-content' + (g.columns === 2 ? ' log-columns-2' : '');
    content.style.top = g.top + 'px';
    content.style.left = g.left + 'px';
    content.style.width = (g.w - g.left - g.right) + 'px';
    content.style.height = (g.h - g.top - g.bottom) + 'px';
    if (g.columns === 2) content.style.columnGap = g.gutter + 'px';
    if (doc.headerHtml) {
      var header = document.createElement('div');
      header.className = 'log-page-header';
      header.style.top = '0';
      header.style.height = g.top + 'px';
      header.style.padding = '0 ' + g.right + 'px 0 ' + g.left + 'px';
      header.innerHTML = doc.headerHtml;
      page.appendChild(header);
    }
    if (doc.footerHtml) {
      var footer = document.createElement('div');
      footer.className = 'log-page-footer';
      footer.style.bottom = '0';
      footer.style.height = g.bottom + 'px';
      footer.style.padding = '0 ' + g.right + 'px 0 ' + g.left + 'px';
      footer.innerHTML = doc.footerHtml;
      page.appendChild(footer);
    }
    page.appendChild(content);
    host.appendChild(page);
    return { el: page, content: content };
  }

  function splittable(el, ignoreAvoid) {
    var tag = el.tagName;
    if (tag === 'IMG' || tag === 'FIGURE' || tag === 'HR' || tag === 'BR' || tag === 'TR' || tag === 'BUTTON') return false;
    if (el.classList.contains('log-image-box')) return false;
    if (el.classList.contains('log-img-missing')) return false;
    if (!ignoreAvoid) {
      var bi = getComputedStyle(el).breakInside;
      if (bi === 'avoid' || bi === 'avoid-page' || bi === 'avoid-column') return false;
    }
    return true;
  }

  function fitText(t, container) {
    var full = t.data;
    var lo = 0;
    var hi = full.length;
    while (lo < hi) {
      var mid = Math.ceil((lo + hi) / 2);
      t.data = full.slice(0, mid);
      if (overflows(container)) hi = mid - 1; else lo = mid;
    }
    var k = lo;
    if (k > 0 && k < full.length) {
      if (/[A-Za-z0-9]/.test(full.charAt(k)) && /[A-Za-z0-9]/.test(full.charAt(k - 1))) {
        var head = full.slice(0, k);
        var sp = head.search(/\s\S*$/);
        if (sp > 0 && k - sp < 32) k = sp + 1;
      }
      // 简单禁则：行首禁止的标点随前一字移到下一页；页末不留开括号。
      while (k > 1 && CLOSE.indexOf(full.charAt(k)) >= 0) k -= 1;
      while (k > 1 && OPEN.indexOf(full.charAt(k - 1)) >= 0) k -= 1;
    }
    t.data = full;
    return k;
  }

  function cloneSplitShell(node) {
    var shell = node.cloneNode(false);
    // Continuation tables need the same column sizing as the first page.
    if (node.nodeType === 1 && node.tagName === 'TABLE') {
      var columns = node.querySelector('colgroup');
      if (columns) shell.appendChild(columns.cloneNode(true));
    }
    return shell;
  }

  function splitNode(node, container, ignoreAvoid) {
    var kids = Array.prototype.slice.call(node.childNodes);
    var i;
    for (i = 0; i < kids.length; i++) node.removeChild(kids[i]);
    for (i = 0; i < kids.length; i++) {
      var child = kids[i];
      node.appendChild(child);
      if (!overflows(container)) continue;
      var rest = null;
      if (child.nodeType === 3) {
        var k = fitText(child, container);
        if (k > 0 && k < child.data.length) {
          rest = document.createTextNode(child.data.slice(k));
          child.data = child.data.slice(0, k);
        }
      } else if (child.nodeType === 1 && splittable(child, ignoreAvoid)) {
        rest = splitNode(child, container, ignoreAvoid);
      }
      var shell;
      var j;
      if (rest) {
        shell = cloneSplitShell(node);
        shell.appendChild(rest);
        for (j = i + 1; j < kids.length; j++) shell.appendChild(kids[j]);
        return shell;
      }
      node.removeChild(child);
      var onlyTableMetadataBefore = node.nodeType === 1 && node.tagName === 'TABLE' && kids.slice(0, i).every(function (item) {
        return item.nodeType === 1 && item.tagName === 'COLGROUP';
      });
      if (i === 0 || onlyTableMetadataBefore) {
        for (j = 0; j < kids.length; j++) node.appendChild(kids[j]);
        return null;
      }
      shell = cloneSplitShell(node);
      for (j = i; j < kids.length; j++) shell.appendChild(kids[j]);
      return shell;
    }
    return null;
  }

  function place(host, g, doc, state, el, warnings) {
    state.page.content.appendChild(el);
    if (!overflows(state.page.content)) return;
    if (state.page.content.childNodes.length > 1) {
      state.page.content.removeChild(el);
      state.page = newPage(host, g, doc, state.count++);
      state.page.content.appendChild(el);
      if (!overflows(state.page.content)) return;
    }
    var piece = el;
    var fragment = 0;
    for (var guard = 0; guard < 500; guard++) {
      var rest = splitNode(piece, state.page.content, false) || splitNode(piece, state.page.content, true);
      if (!rest) {
        piece.classList.add('log-oversize');
        warnings.push({ code: 'OVERSIZE', nodeId: piece.getAttribute('data-log-node') });
        return;
      }
      fragment += 1;
      piece.classList.add('log-continues');
      rest.classList.add('log-continued');
      rest.setAttribute('data-log-fragment', String(fragment));
      state.page = newPage(host, g, doc, state.count++);
      state.page.content.appendChild(rest);
      if (!overflows(state.page.content)) return;
      piece = rest;
    }
    warnings.push({ code: 'SPLIT_LIMIT', nodeId: el.getAttribute('data-log-node') });
  }

  async function paginate(host, blocks, doc, rev, warnings) {
    var g = geometry(doc.page, doc.page.columns === 2 ? 2 : 1);
    document.documentElement.style.setProperty('--log-content-height', (g.h - g.top - g.bottom) + 'px');
    var state = { count: 0, page: null };
    state.page = newPage(host, g, doc, state.count++);
    var pendingBreak = false;
    var started = performance.now();
    for (var i = 0; i < blocks.length; i++) {
      if (performance.now() - started > 24) {
        post({ type: 'progress', revision: rev, done: i, total: blocks.length });
        await nextFrame();
        started = performance.now();
        if (rev !== revision) return false;
      }
      var b = blocks[i];
      // 计算样式需要元素在文档内：临时挂入当前页读取 break-before/after（包括模板根元素）。
      state.page.content.appendChild(b);
      sizeImages(b);
      var first = b.firstElementChild;
      var cs = getComputedStyle(b);
      var fcs = first ? getComputedStyle(first) : null;
      var wantsBefore = b.getAttribute('data-break-before') === '1' || isPageBreak(cs.breakBefore) || (fcs && isPageBreak(fcs.breakBefore));
      var wantsAfter = b.getAttribute('data-break-after') === '1' || isPageBreak(cs.breakAfter) || (fcs && isPageBreak(fcs.breakAfter));
      state.page.content.removeChild(b);
      if ((pendingBreak || wantsBefore) && state.page.content.childNodes.length) {
        state.page = newPage(host, g, doc, state.count++);
      }
      pendingBreak = !!wantsAfter;
      place(host, g, doc, state, b, warnings);
    }
    return true;
  }

  function fillPageFields(host, doc) {
    var pages = host.querySelectorAll('.log-page');
    var start = Number(doc.pageNumberStart) || 1;
    for (var i = 0; i < pages.length; i++) {
      var fills = pages[i].querySelectorAll('[data-log-fill="pageNumber"]');
      for (var j = 0; j < fills.length; j++) fills[j].textContent = String(start + i);
    }
  }

  function buildMap(host) {
    var map = {};
    var nodes = host.querySelectorAll('[data-log-node]');
    for (var i = 0; i < nodes.length; i++) {
      var id = nodes[i].getAttribute('data-log-node');
      var page = nodes[i].closest('.log-page');
      var p = page ? Number(page.getAttribute('data-page')) : 0;
      if (!map[id]) map[id] = [];
      if (map[id].indexOf(p) < 0) map[id].push(p);
    }
    return map;
  }

  function fillToc(host, map, doc) {
    var start = (Number(doc.pageNumberStart) || 1) - 1;
    var cells = host.querySelectorAll('[data-log-toc-target]');
    for (var i = 0; i < cells.length; i++) {
      var pages = map[cells[i].getAttribute('data-log-toc-target')];
      cells[i].textContent = pages && pages.length && pages[0] > 0 ? String(start + pages[0]) : '';
    }
  }

  function addDesignControls(host) {
    if (current.canResizeImages) {
      host.querySelectorAll('img[data-log-resizable-image]').forEach(function (img) {
        if (!img.naturalWidth) return;
        var box = img.parentElement;
        ['reset', 'resize'].forEach(function (action) {
          var button = document.createElement('button');
          button.type = 'button';
          button.className = 'log-image-control log-image-' + action;
          button.setAttribute('data-log-image-action', action);
          button.setAttribute('aria-label', action === 'reset' ? '恢复自动尺寸' : '拖动调整图片大小（保持比例）');
          button.title = button.getAttribute('aria-label');
          button.textContent = action === 'reset' ? '↺' : '↘';
          box.appendChild(button);
        });
      });
    }
    var nodes = host.querySelectorAll('.log-node[data-log-kind="message"][data-log-fragment="0"]');
    for (var i = 0; i < nodes.length; i++) {
      var node = nodes[i];
      var bar = document.createElement('div');
      bar.className = 'log-message-actions';
      bar.setAttribute('role', 'toolbar');
      bar.setAttribute('aria-label', '消息快捷操作');

      function actionButton(label, symbol, action, extraClass) {
        var button = document.createElement('button');
        button.type = 'button';
        button.className = 'log-message-action' + (extraClass ? ' ' + extraClass : '');
        button.setAttribute('aria-label', label);
        button.setAttribute('title', label);
        button.setAttribute('tabindex', '-1');
        button.textContent = symbol;
        if (action) button.setAttribute('data-log-action', action);
        return button;
      }

      bar.appendChild(actionButton('编辑正文', '✎', 'edit'));
      var handle = actionButton('拖动以调整顺序', '⠿', '', 'log-handle');
      if (node.getAttribute('data-log-movable') !== '1') {
        handle.disabled = true;
        handle.setAttribute('aria-disabled', 'true');
      }
      bar.appendChild(handle);
      bar.appendChild(actionButton('在上方插入元素', '↑+', 'insert-before'));
      bar.appendChild(actionButton('在下方插入元素', '↓+', 'insert-after'));
      node.insertBefore(bar, node.firstChild);
    }
    var focusables = host.querySelectorAll('.log-node[data-log-fragment="0"]');
    for (var k = 0; k < focusables.length; k++) focusables[k].setAttribute('tabindex', '0');
  }

  async function render(msg) {
    endImageResize(false);
    endDrag(false);
    var rev = Number(msg.revision) || 0;
    revision = rev;
    var anchor = msg.anchor || captureAnchor();
    current.doc = msg.doc;
    current.design = !!msg.design;
    current.canResizeImages = !!msg.canResizeImages;
    current.mode = msg.mode === 'flow' ? 'flow' : 'paged';
    current.spread = !!msg.spread;
    current.selection = new Set(msg.selection || []);
    current.relocation = msg.relocation || null;
    current.multi = msg.multi === true;
    var doc = msg.doc;
    applyRoot(doc);
    var host = document.createElement('div');
    host.className = 'log-pages' + (current.mode === 'flow' ? ' log-flow' : '') + (current.spread && current.mode !== 'flow' ? ' log-spread' : '');
    host.style.visibility = 'hidden';
    host.style.position = 'absolute';
    host.style.left = '0';
    host.style.top = '0';
    document.body.appendChild(host);
    try {
      var blocks = (doc.blocks || []).map(createBlock);
      var staging = document.createElement('div');
      for (var i = 0; i < blocks.length; i++) staging.appendChild(blocks[i]);
      host.appendChild(staging);
      // 在文档内触发布局后再等待 FontFaceSet，避免先用 fallback 字体测量分页。
      void staging.offsetHeight;
      await waitFonts();
      await decodeImages(staging);
      if (rev !== revision) return;
      staging.remove();
      var warnings = [];
      if (current.mode === 'flow') {
        document.documentElement.style.removeProperty('--log-content-height');
        for (var j = 0; j < blocks.length; j++) { host.appendChild(blocks[j]); sizeImages(blocks[j]); }
      } else {
        var finished = await paginate(host, blocks, doc, rev, warnings);
        if (!finished || rev !== revision) return;
        fillPageFields(host, doc);
      }
      var map = buildMap(host);
      fillToc(host, map, doc);
      if (current.design) addDesignControls(host);
      host.style.visibility = '';
      host.style.position = '';
      root.replaceChildren(host);
      root.style.zoom = String(current.zoom);
      requestAnimationFrame(function () { host.classList.add('log-laid-out'); });
      applySelection();
      applyRelocation();
      applyMulti();
      restoreAnchor(anchor);
      post({
        type: 'layout', revision: rev, pageCount: host.querySelectorAll('.log-page').length,
        map: map, warnings: warnings,
        missingAssets: host.querySelectorAll('.log-img-missing').length,
        nodes: host.querySelectorAll('.log-node').length,
        domNodes: document.getElementsByTagName('*').length,
      });
      reportViewport();
    } finally {
      if (host.parentNode === document.body) host.remove();
    }
  }

  // ---------- 选择、定位与滚动锚点 ----------

  function applySelection() {
    var prev = root.querySelectorAll('.log-selected');
    for (var i = 0; i < prev.length; i++) prev[i].classList.remove('log-selected');
    current.selection.forEach(function (id) {
      var list = root.querySelectorAll('[data-log-node="' + cssEscape(id) + '"]');
      for (var j = 0; j < list.length; j++) list[j].classList.add('log-selected');
    });
  }

  function applyRelocation() {
    var active = current.design && !!current.relocation;
    document.body.classList.toggle('log-relocating', active);
    var sources = new Set(active ? current.relocation.sourceMessageIds : []);
    var nodes = root.querySelectorAll('.log-node');
    for (var i = 0; i < nodes.length; i++) {
      var message = nodes[i].getAttribute('data-log-kind') === 'message';
      var source = message && sources.has(nodes[i].getAttribute('data-log-node'));
      nodes[i].classList.toggle('log-relocate-source', source);
      nodes[i].classList.toggle('log-relocate-target', active && message && !source);
    }
  }

  // 多选模式只改变设计态绘制与点击语义：隐藏单条快捷操作，元素不参与选择。
  function applyMulti() {
    document.body.classList.toggle('log-multi', current.design && current.multi);
  }

  function messageNodeAt(target) {
    return target && target.closest ? target.closest('.log-node[data-log-kind="message"]') : null;
  }

  function postContext(node, x, y) {
    post({ type: 'context', nodeId: node.getAttribute('data-log-node'), x: x, y: y });
  }

  function cssEscape(v) { return window.CSS && CSS.escape ? CSS.escape(v) : String(v).replace(/[^a-zA-Z0-9_-]/g, ''); }

  function captureAnchor() {
    var nodes = root.querySelectorAll('[data-log-node]');
    for (var i = 0; i < nodes.length; i++) {
      var r = nodes[i].getBoundingClientRect();
      if (r.bottom > 0) {
        return { nodeId: nodes[i].getAttribute('data-log-node'), fragmentIndex: Number(nodes[i].getAttribute('data-log-fragment')) || 0, offsetWithinViewport: r.top };
      }
    }
    return null;
  }

  function restoreAnchor(anchor) {
    if (!anchor || !anchor.nodeId) return;
    var list = root.querySelectorAll('[data-log-node="' + cssEscape(anchor.nodeId) + '"]');
    var target = list[Math.min(list.length - 1, Math.max(0, anchor.fragmentIndex || 0))];
    if (!target) return;
    var r = target.getBoundingClientRect();
    window.scrollBy(0, r.top - (Number(anchor.offsetWithinViewport) || 0));
  }

  function scrollToNode(id, flash) {
    var el = root.querySelector('[data-log-node="' + cssEscape(id) + '"]');
    if (!el) { post({ type: 'scrollMiss', nodeId: id }); return; }
    var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    el.scrollIntoView({ block: 'center', behavior: reduce ? 'auto' : 'smooth' });
    if (flash) {
      el.classList.remove('log-flash');
      void el.offsetWidth;
      el.classList.add('log-flash');
    }
  }

  function reportViewport() {
    var viewportHeight = window.innerHeight;
    var height = Math.max(document.documentElement.scrollHeight, document.body.scrollHeight);
    var nearEnd = height - (window.scrollY + viewportHeight) <= Math.max(viewportHeight, 600);
    post({ type: 'anchor', anchor: captureAnchor(), nearEnd: nearEnd });
  }

  window.addEventListener('scroll', function () {
    if (scrollTimer) return;
    scrollTimer = setTimeout(function () {
      scrollTimer = 0;
      reportViewport();
    }, 200);
  }, { passive: true });

  root.addEventListener('click', function (e) {
    if (!current.design) return;
    if (current.relocation) {
      e.preventDefault();
      var picked = e.target.closest ? e.target.closest('.log-node[data-log-kind="message"]') : null;
      if (picked) post({ type: 'select', nodeId: picked.getAttribute('data-log-node'), kind: 'message' });
      return;
    }
    if (current.multi) {
      // 多选：只按节点上报，元素与空白由外层忽略；单条快捷操作与图片手柄此时已隐藏。
      var multiSel = window.getSelection();
      if (multiSel && String(multiSel).length > 0) return;
      var multiNode = e.target.closest ? e.target.closest('[data-log-node]') : null;
      post({ type: 'select', nodeId: multiNode ? multiNode.getAttribute('data-log-node') : null, kind: multiNode ? multiNode.getAttribute('data-log-kind') : undefined, shift: e.shiftKey, additive: e.ctrlKey || e.metaKey });
      return;
    }
    var imageAction = e.target.closest ? e.target.closest('[data-log-image-action]') : null;
    if (imageAction) {
      e.preventDefault();
      e.stopPropagation();
      if (imageAction.getAttribute('data-log-image-action') === 'reset') {
        var image = imageAction.parentElement.querySelector('img');
        if (image) postImageResize(image, null);
      }
      return;
    }
    var action = e.target.closest ? e.target.closest('[data-log-action]') : null;
    if (action) {
      var actionNode = action.closest('[data-log-node]');
      if (actionNode) post({ type: 'action', nodeId: actionNode.getAttribute('data-log-node'), action: action.getAttribute('data-log-action') });
      return;
    }
    if (e.target.closest && e.target.closest('.log-message-actions')) return;
    var sel = window.getSelection();
    if (sel && String(sel).length > 0) return; // 正在选取文字，不改变消息选择
    var node = e.target.closest ? e.target.closest('[data-log-node]') : null;
    if (!node) { post({ type: 'select', nodeId: null }); return; }
    post({ type: 'select', nodeId: node.getAttribute('data-log-node'), kind: node.getAttribute('data-log-kind'), shift: e.shiftKey, additive: e.ctrlKey || e.metaKey });
  });

  root.addEventListener('dblclick', function (e) {
    if (!current.design || current.relocation || current.multi) return;
    if (e.target.closest && e.target.closest('[data-log-image-action]')) return;
    var node = e.target.closest ? e.target.closest('[data-log-node]') : null;
    if (node) post({ type: 'open', nodeId: node.getAttribute('data-log-node'), kind: node.getAttribute('data-log-kind') });
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && imageResize) endImageResize(false);
    if (e.key === 'Escape') { post({ type: 'key', key: 'Escape' }); return; }
    if (!current.design) return;
    if (e.target.closest && e.target.closest('[data-log-image-action]')) return;
    var node = document.activeElement && document.activeElement.closest ? document.activeElement.closest('[data-log-node]') : null;
    if (!node) return;
    if ((e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) && !current.relocation && node.getAttribute('data-log-kind') === 'message') {
      e.preventDefault();
      var rect = node.getBoundingClientRect();
      postContext(node, rect.left + 12, rect.top + 12);
      return;
    }
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      post({ type: 'select', nodeId: node.getAttribute('data-log-node'), kind: node.getAttribute('data-log-kind'), shift: e.shiftKey, additive: e.ctrlKey || e.metaKey });
    }
  });

  // 右键原消息：交给外层显示轻量菜单（进入多选等）；元素与空白处保留浏览器默认行为。
  root.addEventListener('contextmenu', function (e) {
    if (!current.design || current.relocation) return;
    var node = messageNodeAt(e.target);
    if (!node) return;
    e.preventDefault();
    postContext(node, e.clientX, e.clientY);
  });

  // ---------- 拖动排序（仅消息、仅同章节；最终由外层校验并调用 message.reorder） ----------

  function postImageResize(img, width) {
    var node = img.closest('[data-log-node]');
    if (!node) return;
    post({ type: 'imageResize', targetKind: img.getAttribute('data-log-resizable-image'), nodeId: node.getAttribute('data-log-node'), imageKey: img.getAttribute('data-log-image-key') || undefined, width: width });
  }

  root.addEventListener('pointerdown', function (e) {
    var control = e.target.closest ? e.target.closest('[data-log-image-action]') : null;
    if (!control || !current.design || !current.canResizeImages || current.relocation || current.multi) return;
    e.stopImmediatePropagation();
    if (control.getAttribute('data-log-image-action') !== 'resize') return;
    if (e.button !== 0 || imageResize || drag) return;
    e.preventDefault();
    var box = control.parentElement;
    var img = box.querySelector('img');
    if (!img) return;
    var limits = imageLimits(img);
    if (!limits.column || !box.offsetWidth) return;
    imageResize = { img: img, box: box, handle: control, pointerId: e.pointerId, x: e.clientX, width: box.offsetWidth, original: box.style.width, scale: box.getBoundingClientRect().width / box.offsetWidth, limits: limits, moved: false };
    box.classList.add('log-image-resizing');
    control.setPointerCapture(e.pointerId);
    window.addEventListener('pointermove', moveImageResize, { passive: false });
    window.addEventListener('pointerup', finishImageResize);
    window.addEventListener('pointercancel', cancelImageResize);
    window.addEventListener('blur', cancelImageResize);
    control.addEventListener('lostpointercapture', cancelImageResize);
  }, true);

  function moveImageResize(e) {
    var state = imageResize;
    if (!state || e.pointerId !== state.pointerId) return;
    e.preventDefault();
    var delta = (e.clientX - state.x) / state.scale;
    if (Math.abs(delta) < 1 && !state.moved) return;
    state.moved = true;
    state.box.style.width = Math.min(state.limits.max, Math.max(state.limits.min, state.width + delta)) + 'px';
  }

  function finishImageResize(e) {
    if (!imageResize || e.pointerId !== imageResize.pointerId) return;
    moveImageResize(e);
    endImageResize(true);
  }

  function cancelImageResize(e) {
    if (!imageResize || (e && e.pointerId !== undefined && e.pointerId !== imageResize.pointerId)) return;
    endImageResize(false);
  }

  function endImageResize(commit) {
    var state = imageResize;
    if (!state) return;
    imageResize = null;
    window.removeEventListener('pointermove', moveImageResize);
    window.removeEventListener('pointerup', finishImageResize);
    window.removeEventListener('pointercancel', cancelImageResize);
    window.removeEventListener('blur', cancelImageResize);
    state.handle.removeEventListener('lostpointercapture', cancelImageResize);
    if (state.handle.hasPointerCapture(state.pointerId)) state.handle.releasePointerCapture(state.pointerId);
    state.box.classList.remove('log-image-resizing');
    if (commit && state.moved) postImageResize(state.img, clampImageRatio(parseFloat(state.box.style.width) / state.limits.column));
    else state.box.style.width = state.original;
  }

  root.addEventListener('pointerdown', function (e) {
    var handle = e.target.closest ? e.target.closest('.log-handle') : null;
    if (!handle || handle.disabled || !current.design || current.relocation || current.multi) return;
    var node = handle.closest('[data-log-node]');
    if (!node) return;
    e.preventDefault();
    handle.setPointerCapture(e.pointerId);
    var indicator = document.createElement('div');
    indicator.className = 'log-drop-indicator';
    indicator.style.display = 'none';
    document.body.appendChild(indicator);
    drag = { id: node.getAttribute('data-log-node'), handle: handle, indicator: indicator, target: null, side: 'after', timer: 0, y: e.clientY };
    var all = root.querySelectorAll('[data-log-node="' + cssEscape(drag.id) + '"]');
    for (var i = 0; i < all.length; i++) all[i].classList.add('log-dragging');
    drag.timer = setInterval(autoScroll, 30);
  });

  function autoScroll() {
    if (!drag) return;
    var edge = 48;
    if (drag.y < edge) window.scrollBy(0, -Math.ceil((edge - drag.y) / 3));
    else if (drag.y > window.innerHeight - edge) window.scrollBy(0, Math.ceil((drag.y - window.innerHeight + edge) / 3));
  }

  root.addEventListener('pointermove', function (e) {
    if (!drag) return;
    drag.y = e.clientY;
    drag.handle.style.visibility = 'hidden';
    var under = document.elementFromPoint(e.clientX, e.clientY);
    drag.handle.style.visibility = '';
    var node = under && under.closest ? under.closest('.log-node[data-log-kind="message"]') : null;
    if (!node || node.getAttribute('data-log-node') === drag.id) { drag.target = null; drag.indicator.style.display = 'none'; return; }
    var r = node.getBoundingClientRect();
    drag.side = e.clientY < r.top + r.height / 2 ? 'before' : 'after';
    drag.target = node.getAttribute('data-log-node');
    drag.indicator.style.display = 'block';
    drag.indicator.style.left = (r.left + window.scrollX) + 'px';
    drag.indicator.style.width = r.width + 'px';
    drag.indicator.style.top = ((drag.side === 'before' ? r.top : r.bottom) + window.scrollY - 1) + 'px';
  });

  function endDrag(commit) {
    if (!drag) return;
    clearInterval(drag.timer);
    drag.indicator.remove();
    var all = root.querySelectorAll('.log-dragging');
    for (var i = 0; i < all.length; i++) all[i].classList.remove('log-dragging');
    if (commit && drag.target) post({ type: 'drop', nodeId: drag.id, targetId: drag.target, side: drag.side });
    drag = null;
  }
  root.addEventListener('pointerup', function () { endDrag(true); });
  root.addEventListener('pointercancel', function () { endDrag(false); });
})();
