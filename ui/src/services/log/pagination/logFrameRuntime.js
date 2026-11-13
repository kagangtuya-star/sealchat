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
      case 'proofMarks': setProofMarks(msg.nodes); break;
      case 'proofView': setProofView(msg.view); break;
      case 'proofReveal': proofReveal(msg.key); break;
      case 'zoom': endImageResize(false); current.zoom = clampZoom(msg.zoom); root.style.zoom = String(current.zoom); updateProofGutter(); scheduleProof(); break;
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
    suspendProof(true);
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
      suspendProof(false);
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
    teardownProof();
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
      applyProof();
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
    scheduleProof();
    if (scrollTimer) return;
    scrollTimer = setTimeout(function () {
      scrollTimer = 0;
      reportViewport();
    }, 200);
  }, { passive: true });

  window.addEventListener('resize', function () {
    updateProofGutter();
    scheduleProof();
  });

  root.addEventListener('click', function (e) {
    if (!current.design) return;
    // 审阅中点中批注文字只选中批注，不改变消息选择；正在选取文字时保持浏览器行为。
    var pickSel = window.getSelection();
    var picked = !current.relocation && !current.multi && !(pickSel && String(pickSel).length > 0) ? proofHit(e.clientX, e.clientY, e.target) : null;
    if (picked) {
      post({ type: 'proofPick', key: picked });
      return;
    }
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
    // 审阅快捷键交给外层统一处理（与外层窗口获得焦点时相同）。
    var proofKey = String(e.key || '').toLowerCase();
    var keyTarget = e.target && e.target.closest ? e.target.closest('input, textarea, select, button, a[href], [contenteditable], [role="textbox"], [role="dialog"], [role="combobox"], [role="menu"], [role="listbox"]') : null;
    if (proof.nodes && !proof.suspended && PROOF_KEYS[proofKey] && !e.ctrlKey && !e.metaKey && !e.altKey && !e.defaultPrevented && !keyTarget) {
      e.preventDefault();
      post({ type: 'key', key: proofKey });
      return;
    }
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
    if (!drag) {
      hoverProof(e);
      return;
    }
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

  // ---------- 校对审阅层 ----------
  // 只在编辑器预览中出现：文字标记用 CSS Custom Highlight（不改动消息 DOM，不影响测量与分页），
  // 批注卡片与页边锚点画在 body 下独立的绝对定位层；打印、PDF 与发布页面都不会收到这些消息。
  // 批注位置由外层给出的正文文本区间在当前排版的 .log-body 文本节点上重新映射，并逐处核对原文；
  // 核对失败的批注只锚定到整条消息，不猜测字符位置。

  var PROOF_KEYS = { a: 1, r: 1, u: 1, j: 1, k: 1 };
  var PROOF_SKIP = 'button, .log-img-missing, .log-message-actions, .log-image-control';
  var PROOF_GUTTER = 284;
  var PROOF_NOTE_WIDTH = 244;
  var HIGHLIGHTS = window.CSS && window.CSS.highlights && typeof Highlight === 'function' ? window.CSS.highlights : null;
  var proof = {
    nodes: null,
    view: { active: null, states: {}, position: null },
    marks: new Map(),
    byNode: new Map(),
    order: [],
    layer: null,
    svg: null,
    path: null,
    els: new Map(),
    hl: null,
    hover: null,
    gutter: false,
    /** 审阅层曾在当前文档中出现过；未出现时关闭审阅层不触碰 DOM。 */
    shown: false,
    suspended: false,
    raf: 0,
    backup: 0,
    hoverRaf: 0,
    reveal: null,
  };

  function setProofMarks(nodes) {
    proof.nodes = Array.isArray(nodes) && nodes.length ? nodes : null;
    applyProof();
  }

  function setProofView(view) {
    var v = view && typeof view === 'object' ? view : {};
    proof.view = {
      active: typeof v.active === 'string' ? v.active : null,
      states: v.states && typeof v.states === 'object' ? v.states : {},
      position: v.position && typeof v.position === 'object' ? v.position : null,
    };
    refreshProofHighlights();
    scheduleProof();
  }

  /** 消息所有分片中 .log-body 的可见文本节点，按文档顺序；规则与外层 bodyText 一致。 */
  function proofSegments(nodeId) {
    var frags = root.querySelectorAll('[data-log-node="' + cssEscape(nodeId) + '"]');
    var segs = [];
    var total = 0;
    for (var i = 0; i < frags.length; i++) {
      var bodies = frags[i].querySelectorAll('.log-body');
      for (var j = 0; j < bodies.length; j++) {
        var body = bodies[j];
        var outer = body.parentElement && body.parentElement.closest('.log-body');
        if (outer && frags[i].contains(outer)) continue;
        var walker = document.createTreeWalker(body, NodeFilter.SHOW_TEXT, {
          acceptNode: function (t) {
            var skip = t.parentElement && t.parentElement.closest(PROOF_SKIP);
            return skip && body.contains(skip) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT;
          },
        });
        for (var t = walker.nextNode(); t; t = walker.nextNode()) {
          segs.push({ node: t, from: total });
          total += t.data.length;
        }
      }
    }
    return { segs: segs, total: total, first: frags[0] || null };
  }

  /** 区间可能跨分页：每个文本节点各建一个 Range，避免把页眉页脚一起标记。 */
  function proofRanges(segs, start, end) {
    var out = [];
    for (var i = 0; i < segs.length; i++) {
      var s = segs[i];
      var len = s.node.data.length;
      if (s.from + len <= start) continue;
      if (s.from >= end) break;
      if (!len) continue;
      var r = document.createRange();
      r.setStart(s.node, Math.max(0, start - s.from));
      r.setEnd(s.node, Math.min(len, end - s.from));
      out.push(r);
    }
    return out;
  }

  function proofPageOf(node) {
    var el = node && (node.nodeType === 1 ? node : node.parentElement);
    return el ? el.closest('.log-page') || el.closest('.log-pages') : null;
  }

  function applyProof() {
    proof.marks = new Map();
    proof.byNode = new Map();
    proof.order = [];
    proof.hover = null;
    if (!proof.nodes) {
      teardownProof();
      return;
    }
    var missed = [];
    proof.nodes.forEach(function (n) {
      if (!n || typeof n.nodeId !== 'string' || !Array.isArray(n.items)) return;
      var seg = proofSegments(n.nodeId);
      var exact = !!seg.first && seg.total === Number(n.length);
      var keys = [];
      n.items.forEach(function (item) {
        if (!item || typeof item.key !== 'string') return;
        if (!seg.first) { missed.push(item.key); return; }
        var ranges = null;
        if (exact && item.start >= 0 && item.end > item.start) {
          var list = proofRanges(seg.segs, item.start, item.end);
          if (list.length && list.map(String).join('') === item.original) ranges = list;
        }
        if (!ranges && item.start >= 0) missed.push(item.key);
        proof.marks.set(item.key, {
          key: item.key,
          item: item,
          ranges: ranges,
          anchor: seg.first,
          page: proofPageOf(ranges ? ranges[0].startContainer : seg.first),
        });
        proof.order.push(item.key);
        keys.push(item.key);
      });
      if (keys.length) proof.byNode.set(n.nodeId, keys);
    });
    proof.shown = true;
    document.body.classList.add('log-reviewing');
    updateProofGutter();
    refreshProofHighlights();
    post({ type: 'proofMapped', missed: missed });
    tryProofReveal();
    scheduleProof();
  }

  function teardownProof() {
    proof.reveal = null;
    if (!proof.shown) return;
    proof.shown = false;
    if (HIGHLIGHTS && proof.hl) Object.keys(proof.hl).forEach(function (k) { HIGHLIGHTS.delete('log-proof-' + k); });
    if (proof.layer) proof.layer.remove();
    proof.layer = null;
    proof.svg = null;
    proof.path = null;
    proof.els.clear();
    proof.hover = null;
    document.body.classList.remove('log-reviewing', 'log-review-gutter', 'log-proof-pointer');
    document.documentElement.style.removeProperty('--log-review-gutter');
  }

  function suspendProof(on) {
    proof.suspended = on;
    if (proof.layer) proof.layer.style.display = on ? 'none' : '';
    refreshProofHighlights();
    if (!on) scheduleProof();
  }

  function proofState(key) {
    var s = proof.view.states[key];
    return s === 'accepted' || s === 'rejected' ? s : 'pending';
  }

  function refreshProofHighlights() {
    if (!HIGHLIGHTS) return;
    if (!proof.hl) {
      proof.hl = {};
      ['suggest', 'safe', 'rejected', 'accepted', 'hover', 'active'].forEach(function (name, i) {
        var h = new Highlight();
        h.priority = i;
        proof.hl[name] = h;
      });
    }
    var hl = proof.hl;
    Object.keys(hl).forEach(function (k) { hl[k].clear(); });
    if (!proof.nodes || proof.suspended) {
      Object.keys(hl).forEach(function (k) { HIGHLIGHTS.delete('log-proof-' + k); });
      return;
    }
    proof.marks.forEach(function (m, key) {
      if (!m.ranges) return;
      var st = proofState(key);
      var bucket = st === 'accepted' ? hl.accepted : st === 'rejected' ? hl.rejected : m.item.level === 'safe' ? hl.safe : hl.suggest;
      for (var i = 0; i < m.ranges.length; i++) {
        bucket.add(m.ranges[i]);
        if (key === proof.view.active) hl.active.add(m.ranges[i]);
        if (key === proof.hover) hl.hover.add(m.ranges[i]);
      }
    });
    Object.keys(hl).forEach(function (k) { HIGHLIGHTS.set('log-proof-' + k, hl[k]); });
  }

  /** 页面足够宽时为批注留出页边栏（只改变纸张的水平位置，不改变分页）。 */
  function updateProofGutter() {
    var on = false;
    if (proof.nodes && proof.order.length && current.design) {
      var page = root.querySelector('.log-page') || root.querySelector('.log-pages.log-flow');
      if (page) {
        var width = page.getBoundingClientRect().width * (current.spread && current.mode !== 'flow' ? 2 : 1);
        on = document.documentElement.clientWidth >= width + 56 + PROOF_GUTTER;
      }
    }
    proof.gutter = on;
    document.body.classList.toggle('log-review-gutter', on);
    if (on) document.documentElement.style.setProperty('--log-review-gutter', (PROOF_GUTTER / (current.zoom || 1)) + 'px');
    else document.documentElement.style.removeProperty('--log-review-gutter');
  }

  function scheduleProof() {
    if (!proof.nodes || proof.raf) return;
    var run = function () {
      cancelAnimationFrame(proof.raf);
      clearTimeout(proof.backup);
      proof.raf = 0;
      proof.backup = 0;
      drawProof();
    };
    proof.raf = requestAnimationFrame(run);
    // 页面不可见时 rAF 暂停；保底定时器保证恢复显示时位置已更新。
    proof.backup = setTimeout(run, 160);
  }

  /** 批注首行在视口中的位置；无法定位时取消息顶部一行。 */
  function proofBox(m) {
    if (m.ranges) {
      var rects = m.ranges[0].getClientRects();
      var r = rects[0] || m.ranges[0].getBoundingClientRect();
      var last = m.ranges[m.ranges.length - 1].getClientRects();
      var tail = last[last.length - 1] || r;
      if (!r || (!r.width && !r.height && !r.top && !r.left)) return null;
      return { top: r.top, bottom: r.bottom, left: r.left, right: tail.top === r.top ? tail.right : r.right, exact: true };
    }
    if (!m.anchor || !m.anchor.isConnected) return null;
    var a = m.anchor.getBoundingClientRect();
    return { top: a.top, bottom: Math.min(a.bottom, a.top + 24), left: a.left, right: a.right, exact: false, node: a };
  }

  function ensureProofLayer() {
    if (proof.layer) return proof.layer;
    var layer = document.createElement('div');
    layer.className = 'log-proof-layer';
    var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'log-proof-wire');
    var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    svg.appendChild(path);
    layer.appendChild(svg);
    layer.addEventListener('click', onProofLayerClick);
    layer.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter' && e.key !== ' ') return;
      if (e.target.closest && e.target.closest('[data-proof-act]')) return;
      var host = e.target.closest ? e.target.closest('[data-proof-key]') : null;
      if (!host) return;
      e.preventDefault();
      post({ type: 'proofPick', key: host.getAttribute('data-proof-key') });
    });
    layer.addEventListener('pointerover', function (e) {
      var host = e.target.closest ? e.target.closest('[data-proof-key]') : null;
      setProofHover(host ? host.getAttribute('data-proof-key') : null);
    });
    layer.addEventListener('pointerleave', function () { setProofHover(null); });
    document.body.appendChild(layer);
    proof.layer = layer;
    proof.svg = svg;
    proof.path = path;
    return layer;
  }

  function onProofLayerClick(e) {
    var act = e.target.closest ? e.target.closest('[data-proof-act]') : null;
    var host = e.target.closest ? e.target.closest('[data-proof-key]') : null;
    if (!host) return;
    var key = host.getAttribute('data-proof-key');
    e.preventDefault();
    e.stopPropagation();
    if (!act) {
      post({ type: 'proofPick', key: key });
      return;
    }
    var a = act.getAttribute('data-proof-act');
    if (a === 'prev' || a === 'next') post({ type: 'proofNav', step: a === 'next' ? 1 : -1 });
    else post({ type: 'proofDecide', key: key, decision: a === 'accept' ? 'accepted' : a === 'reject' ? 'rejected' : null });
  }

  function proofText(s) {
    return String(s).replace(/ /g, '␣').replace(/\u00a0/g, '⍽').replace(/\t/g, '⇥');
  }

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text != null) node.textContent = text;
    return node;
  }

  function proofDiff(item, cls) {
    var box = el('span', cls);
    box.appendChild(el('del', '', proofText(item.original)));
    box.appendChild(el('i', '', '→'));
    box.appendChild(item.replacement ? el('ins', '', proofText(item.replacement)) : el('ins', 'is-empty', '删除'));
    return box;
  }

  function proofButton(act, text, hint, cls) {
    var b = el('button', 'log-proof-btn' + (cls ? ' ' + cls : ''));
    b.type = 'button';
    b.setAttribute('data-proof-act', act);
    b.setAttribute('title', hint);
    b.setAttribute('aria-label', hint);
    b.textContent = text;
    return b;
  }

  /** 展开的审阅卡片：差异、规则、操作与导航。内容一律以 textContent 写入。 */
  function fillProofCard(node, m) {
    var item = m.item;
    var st = proofState(m.key);
    var head = el('div', 'log-proof-card__head');
    head.appendChild(el('span', 'log-proof-badge is-' + item.level, item.level === 'safe' ? '可自动' : '待确认'));
    head.appendChild(el('span', 'log-proof-card__label', item.label || '排版建议'));
    var pos = proof.view.position;
    if (pos && pos.total) head.appendChild(el('span', 'log-proof-card__pos', pos.index + ' / ' + pos.total));
    node.appendChild(head);
    node.appendChild(proofDiff(item, 'log-proof-card__diff'));
    if (item.reason) node.appendChild(el('div', 'log-proof-card__note', item.reason));
    if (!m.ranges) {
      var ctx = el('div', 'log-proof-card__ctx');
      ctx.appendChild(document.createTextNode('…' + (item.before || '')));
      ctx.appendChild(el('mark', '', proofText(item.original)));
      ctx.appendChild(document.createTextNode((item.after || '') + '…'));
      node.appendChild(ctx);
      node.appendChild(el('div', 'log-proof-card__note', item.note || '无法在排版中精确定位，按原文显示'));
    }
    var acts = el('div', 'log-proof-card__acts');
    acts.appendChild(proofButton('accept', st === 'accepted' ? '已采纳' : '采纳', '采纳 (A)', 'is-accept' + (st === 'accepted' ? ' is-on' : '')));
    acts.appendChild(proofButton('reject', st === 'rejected' ? '已放弃' : '放弃', '放弃 (R)', 'is-reject' + (st === 'rejected' ? ' is-on' : '')));
    if (st !== 'pending') acts.appendChild(proofButton('clear', '↶', '撤回决定 (U)', 'is-icon'));
    acts.appendChild(el('span', 'log-proof-card__spacer'));
    acts.appendChild(proofButton('prev', '‹', '上一处 (K)', 'is-icon'));
    acts.appendChild(proofButton('next', '›', '下一处 (J)', 'is-icon'));
    node.appendChild(acts);
  }

  function proofElement(id, build, signature) {
    var node = proof.els.get(id);
    if (!node) {
      node = document.createElement('div');
      node.setAttribute('data-proof-id', id);
      proof.layer.appendChild(node);
      proof.els.set(id, node);
    }
    if (node.__sig !== signature) {
      node.__sig = signature;
      node.replaceChildren();
      build(node);
    }
    node.__seen = true;
    return node;
  }

  function proofSignature(m, active) {
    var pos = proof.view.position;
    return [proofState(m.key), active ? 1 : 0, m.ranges ? 1 : 0, active && pos ? pos.index + '/' + pos.total : ''].join('|');
  }

  function drawProof() {
    if (!proof.nodes || proof.suspended) return;
    ensureProofLayer();
    proof.els.forEach(function (n) { n.__seen = false; });
    var vh = window.innerHeight;
    var vw = document.documentElement.clientWidth;
    var sx = window.scrollX;
    var sy = window.scrollY;
    var low = -vh * 0.6;
    var high = vh * 1.6;
    var pageRects = new Map();
    var pageRect = function (p) {
      if (!p) return null;
      var r = pageRects.get(p);
      if (!r) { r = p.getBoundingClientRect(); pageRects.set(p, r); }
      return r;
    };
    var active = proof.view.active;
    var visible = [];
    var refRight = 0;
    for (var i = 0; i < proof.order.length; i++) {
      var m = proof.marks.get(proof.order[i]);
      var pr = pageRect(m.page);
      if (!pr || pr.bottom < low || pr.top > high) continue;
      var box = proofBox(m);
      if (!box || box.bottom < low || box.top > high) continue;
      refRight = Math.max(refRight, pr.right);
      visible.push({ m: m, box: box, page: pr, active: m.key === active });
    }
    var wire = '';
    if (proof.gutter) wire = layoutProofNotes(visible, refRight, sx, sy);
    else wire = layoutProofPins(visible, vw, vh, sx, sy);
    proof.path.setAttribute('d', wire);
    drawProofFallback(visible, sx, sy);
    proof.els.forEach(function (n, id) {
      if (!n.__seen) { n.remove(); proof.els.delete(id); }
    });
  }

  function layoutProofNotes(visible, refRight, sx, sy) {
    var x = refRight + 20;
    var items = visible.map(function (v) {
      var node = proofElement('note:' + v.m.key, function (n) {
        if (v.active) fillProofCard(n, v.m);
        else n.appendChild(proofDiff(v.m.item, 'log-proof-note__diff'));
      }, proofSignature(v.m, v.active));
      node.className = 'log-proof-note is-' + v.m.item.level + ' is-' + proofState(v.m.key) + (v.active ? ' is-active' : '') + (v.m.key === proof.hover ? ' is-hover' : '') + (v.m.ranges ? '' : ' is-loose');
      node.setAttribute('data-proof-key', v.m.key);
      node.setAttribute('role', v.active ? 'group' : 'button');
      node.setAttribute('aria-label', (v.m.item.label || '校对建议') + '：' + v.m.item.original + ' 改为 ' + (v.m.item.replacement || '删除'));
      if (!v.active) node.setAttribute('tabindex', '0'); else node.removeAttribute('tabindex');
      node.style.width = PROOF_NOTE_WIDTH + 'px';
      return { v: v, node: node, want: v.box.top - 3 };
    });
    items.forEach(function (it) { it.h = it.node.offsetHeight; });
    var gap = 5;
    var ai = -1;
    for (var i = 0; i < items.length; i++) if (items[i].v.active) ai = i;
    if (ai < 0) {
      var y = -Infinity;
      items.forEach(function (it) { it.y = Math.max(it.want, y); y = it.y + it.h + gap; });
    } else {
      items[ai].y = items[ai].want;
      for (var d = ai + 1; d < items.length; d++) items[d].y = Math.max(items[d].want, items[d - 1].y + items[d - 1].h + gap);
      for (var u = ai - 1; u >= 0; u--) items[u].y = Math.min(items[u].want, items[u + 1].y - items[u].h - gap);
    }
    var wire = '';
    items.forEach(function (it) {
      it.node.style.transform = 'translate(' + Math.round(x + sx) + 'px,' + Math.round(it.y + sy) + 'px)';
      if (!it.v.active) return;
      var b = it.v.box;
      var ly = (b.exact ? b.bottom : b.top + 12) + sy;
      var edge = it.v.page.right + 6 + sx;
      wire = 'M' + Math.round((b.exact ? b.right : it.v.page.right - 2) + sx) + ' ' + Math.round(ly) + 'H' + Math.round(edge) + 'L' + Math.round(x + sx) + ' ' + Math.round(it.y + 16 + sy);
    });
    return wire;
  }

  function layoutProofPins(visible, vw, vh, sx, sy) {
    var groups = [];
    visible.forEach(function (v) {
      var g = groups[groups.length - 1];
      if (g && Math.abs(v.box.top - g.top) < 14 && g.page === v.page) g.list.push(v);
      else groups.push({ top: v.box.top, page: v.page, list: [v] });
    });
    groups.forEach(function (g) {
      var first = g.list[0];
      var hasActive = g.list.some(function (v) { return v.active; });
      var lead = hasActive ? g.list.filter(function (v) { return v.active; })[0] : first;
      var pending = g.list.filter(function (v) { return proofState(v.m.key) === 'pending'; }).length;
      var pin = proofElement('pin:' + first.m.key, function (n) {
        if (g.list.length > 1) n.textContent = String(g.list.length);
      }, g.list.length + '');
      var state = pending ? 'pending' : proofState(lead.m.key);
      pin.className = 'log-proof-pin is-' + lead.m.item.level + ' is-' + state + (hasActive ? ' is-active' : '') + (g.list.length > 1 ? ' is-count' : '');
      pin.setAttribute('data-proof-key', (hasActive && g.list.length > 1 ? g.list[(g.list.indexOf(lead) + 1) % g.list.length] : lead).m.key);
      pin.setAttribute('role', 'button');
      pin.setAttribute('tabindex', '0');
      pin.setAttribute('aria-label', g.list.length > 1 ? '本行 ' + g.list.length + ' 处校对建议' : '校对建议：' + lead.m.item.original);
      var outside = vw - g.page.right >= 26;
      var px = outside ? g.page.right + 7 : g.page.right - 20;
      pin.style.transform = 'translate(' + Math.round(px + sx) + 'px,' + Math.round(g.top + (lead.box.bottom - lead.box.top) / 2 - 7 + sy) + 'px)';
    });
    var act = visible.filter(function (v) { return v.active; })[0];
    if (!act) return '';
    var card = proofElement('pop:' + act.m.key, function (n) { fillProofCard(n, act.m); }, proofSignature(act.m, true));
    card.className = 'log-proof-note log-proof-pop is-active is-' + act.m.item.level + ' is-' + proofState(act.m.key) + (act.m.ranges ? '' : ' is-loose');
    card.setAttribute('data-proof-key', act.m.key);
    card.setAttribute('role', 'group');
    var w = Math.min(300, vw - 16);
    card.style.width = w + 'px';
    var h = card.offsetHeight;
    var b = act.box;
    var below = b.bottom + 10;
    var above = b.top - h - 10;
    var top = below + h > vh - 8 && above > 8 ? above : below;
    card.classList.toggle('is-above', top === above);
    var left = Math.min(Math.max(8, b.left - 20), vw - w - 8);
    card.style.setProperty('--log-proof-caret', Math.round(Math.min(Math.max(14, b.left - left + 6), w - 18)) + 'px');
    card.style.transform = 'translate(' + Math.round(left + sx) + 'px,' + Math.round(top + sy) + 'px)';
    return '';
  }

  /** 不支持 Highlight API 的浏览器：只为视口附近的批注绘制下划线块。无法定位的当前批注画出整条消息的虚线框。 */
  function drawProofFallback(visible, sx, sy) {
    visible.forEach(function (v) {
      if (!v.m.ranges) {
        if (!v.active || !v.box.node) return;
        var r = v.box.node;
        var frame = proofElement('frame:' + v.m.key, function () {}, '');
        frame.className = 'log-proof-frame';
        frame.style.transform = 'translate(' + Math.round(r.left + sx - 3) + 'px,' + Math.round(r.top + sy - 3) + 'px)';
        frame.style.width = Math.round(r.width + 6) + 'px';
        frame.style.height = Math.round(r.height + 6) + 'px';
        return;
      }
      if (HIGHLIGHTS) return;
      var rects = [];
      v.m.ranges.forEach(function (range) { Array.prototype.push.apply(rects, range.getClientRects()); });
      rects.forEach(function (r, i) {
        var mark = proofElement('hl:' + v.m.key + ':' + i, function () {}, '');
        mark.className = 'log-proof-hl is-' + v.m.item.level + ' is-' + proofState(v.m.key) + (v.active ? ' is-active' : '');
        mark.style.transform = 'translate(' + Math.round(r.left + sx) + 'px,' + Math.round(r.top + sy) + 'px)';
        mark.style.width = Math.max(4, Math.round(r.width)) + 'px';
        mark.style.height = Math.round(r.height) + 'px';
      });
    });
  }

  /** 点中的批注：只检查该消息自己的批注区间。 */
  function proofHit(x, y, target) {
    if (!proof.nodes || proof.suspended) return null;
    var node = target && target.closest ? target.closest('[data-log-node]') : null;
    var keys = node ? proof.byNode.get(node.getAttribute('data-log-node')) : null;
    if (!keys) return null;
    for (var i = 0; i < keys.length; i++) {
      var m = proof.marks.get(keys[i]);
      if (!m || !m.ranges) continue;
      for (var j = 0; j < m.ranges.length; j++) {
        var rects = m.ranges[j].getClientRects();
        for (var k = 0; k < rects.length; k++) {
          var r = rects[k];
          if (x >= r.left - 2 && x <= r.right + 2 && y >= r.top - 1 && y <= r.bottom + 1) return m.key;
        }
      }
    }
    return null;
  }

  function hoverProof(e) {
    if (!proof.nodes || proof.hoverRaf) return;
    var x = e.clientX;
    var y = e.clientY;
    var target = e.target;
    proof.hoverRaf = requestAnimationFrame(function () {
      proof.hoverRaf = 0;
      setProofHover(proofHit(x, y, target));
    });
  }

  function setProofHover(key) {
    if (proof.hover === key) return;
    proof.hover = key;
    document.body.classList.toggle('log-proof-pointer', !!key);
    if (HIGHLIGHTS && proof.hl) {
      proof.hl.hover.clear();
      var m = key ? proof.marks.get(key) : null;
      if (m && m.ranges) m.ranges.forEach(function (r) { proof.hl.hover.add(r); });
    }
    proof.els.forEach(function (n) {
      if (n.classList.contains('log-proof-note')) n.classList.toggle('is-hover', n.getAttribute('data-proof-key') === key);
    });
  }

  function proofReveal(key) {
    proof.reveal = typeof key === 'string' && key ? { key: key, until: Date.now() + 10000 } : null;
    tryProofReveal();
  }

  /** 只在批注不在舒适阅读区时滚动，让卡片与原文同时可见。 */
  function tryProofReveal() {
    var want = proof.reveal;
    if (!want) return;
    if (Date.now() > want.until) { proof.reveal = null; return; }
    var m = proof.marks.get(want.key);
    if (!m) return;
    proof.reveal = null;
    var b = proofBox(m);
    if (!b) return;
    var vh = window.innerHeight;
    var vw = document.documentElement.clientWidth;
    var dy = b.top < 80 || b.bottom > vh - 180 ? b.top - vh * 0.34 : 0;
    var dx = b.left < 16 ? b.left - 32 : b.right > vw - 16 ? b.right - vw + 64 : 0;
    if (dx || dy) {
      var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      window.scrollBy({ left: dx, top: dy, behavior: reduce ? 'auto' : 'smooth' });
    }
    scheduleProof();
  }
})();
