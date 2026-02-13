# 角色卡模板开发文档

本文档面向需要编写和维护人物卡 HTML 模板的开发者，覆盖模板运行机制、可用 API、事件约定、调试建议，两个可直接复制的示例模板、一个美观的COC角色卡自定义掷骰模板示例。

---

## 1. 运行架构与数据流

角色卡模板运行在受限 iframe（sandbox=allow-scripts）中，由宿主页面负责注入数据并接收模板事件。

### 1.1 宿主到模板（数据下发）

宿主通过 postMessage 下发：

- type: SEALCHAT_UPDATE
- payload:
  - windowId: string
  - name: string
  - attrs: Record<string, any>
  - avatarUrl: string（可选）

模板通过 sealchat.onUpdate(cb) 订阅更新。

### 1.2 模板到宿主（事件上报）

模板上报统一事件：

- type: SEALCHAT_EVENT
- action 仅支持：
  - ROLL_DICE（掷骰请求）
  - UPDATE_ATTRS（属性更新）

宿主在窗口层接收后分发：

- ROLL_DICE
  - 默认模式：弹出内置掷骰窗口（优势/劣势/修正）
  - 模板模式：直接发送指令，不弹默认窗口
- UPDATE_ATTRS
  - 写回当前人物卡 attrs

---

## 2. 模板可用 API

在模板脚本中可直接调用 window.sealchat：

### 2.1 sealchat.onUpdate(callback)

注册数据更新回调。

~~~js
sealchat.onUpdate(function (data) {
  // data.name, data.attrs, data.avatarUrl, data.windowId
  render(data);
});
~~~

### 2.2 sealchat.roll(template, label, args)

上报掷骰请求。

- template：骰子模板（例如 .ra {skill} / .ra 力量）
- label：展示标签
- args：参数替换字典（例如 { skill: 侦查 }）

~~~js
sealchat.roll('.ra {skill}', '侦查检定', { skill: '侦查' });
~~~

### 2.3 sealchat.updateAttrs(attrsPatch)

提交属性 patch（局部更新）。

~~~js
sealchat.updateAttrs({ 力量: 65, 敏捷: 70 });
~~~

### 2.4 sealchat.setRollDispatchMode(mode) 和 sealchat.setRollMode(mode)

设置模板内掷骰分发模式（两个 API 等价，setRollMode 是别名）。

- mode = default（默认）：走宿主默认掷骰窗口
- mode = template：跳过默认窗口，直接发送最终指令

~~~js
sealchat.setRollDispatchMode('template');
// 或 sealchat.setRollMode('template');
~~~

注意：默认模板里该调用会以注释形式出现，仅作示例，不会默认启用。

---

## 3. 掷骰行为与模式说明

### 3.1 默认模式（推荐给大多数模板）

- 模板触发 ROLL_DICE
- 宿主展示 DiceRollPopover
- 用户在弹窗里设置优势/劣势/修正
- 最终生成表达式并发送到聊天

优点：统一体验、用户可临时调整掷骰参数。

### 3.2 模板模式（自定义窗口或快速投掷）

- 模板先设置 sealchat.setRollDispatchMode('template')
- 模板触发 ROLL_DICE
- 宿主直接发送指令，不展示默认弹窗

适用场景：

- 你在模板内实现了自己的检定确认 UI
- 需要单击即投掷（One-click roll）
- 需要模板完全掌控交互流程

---

## 4. 推荐模板结构

### 4.1 必要前置：桥接脚本（必须保留）

如果你是直接在模板编辑器里粘贴完整 HTML，请确保模板内包含 `window.sealchat` 桥接脚本。
文档后面的两个示例已经内置桥接脚本，可直接复制使用。

建议将模板划分为三层：

1. 数据层：onUpdate 接收并缓存数据
2. 渲染层：纯函数 render(data) 输出 DOM
3. 交互层：点击事件委托（编辑属性、触发掷骰）

最小骨架：

~~~html
<!DOCTYPE html>
<html>
<head>
  <meta charset=UTF-8 />
  <style>
    body { font-family: sans-serif; margin: 0; padding: 12px; }
  </style>
</head>
<body>
  <div id=app></div>
  <script>
    var _windowId = null;
    var _rollDispatchMode = 'default';
    function postEvent(action, payload) {
      if (!_windowId) return;
      window.parent.postMessage({ type: 'SEALCHAT_EVENT', version: 1, windowId: _windowId, action: action, payload: payload }, '*');
    }
    window.sealchat = {
      onUpdate: function(cb) {
        window.addEventListener('message', function(e) {
          if (e.source !== window.parent) return;
          if (e.data && e.data.type === 'SEALCHAT_UPDATE') { _windowId = e.data.payload.windowId; cb(e.data.payload); }
        });
      },
      setRollDispatchMode: function(mode) { _rollDispatchMode = mode === 'template' ? 'template' : 'default'; },
      setRollMode: function(mode) { _rollDispatchMode = mode === 'template' ? 'template' : 'default'; },
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', { roll: { template: template, label: label || '', args: args || {}, dispatchMode: _rollDispatchMode } });
      },
      updateAttrs: function(attrs) { postEvent('UPDATE_ATTRS', { attrs: attrs }); },
    };

    function render(data) {
      document.getElementById('app').textContent = JSON.stringify(data.attrs || {});
    }
    sealchat.onUpdate(render);
  </script>
</body>
</html>
~~~

---

## 5. 示例一：默认掷骰窗口模式（保持系统默认流程）

特点：不设置 setRollDispatchMode('template')，点击属性后走默认掷骰弹窗。

~~~html
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 12px; }
    .card { border: 1px solid #e5e7eb; border-radius: 8px; padding: 12px; }
    .title { font-size: 18px; font-weight: 600; margin-bottom: 8px; }
    .row { display: grid; grid-template-columns: 1fr auto auto; gap: 8px; align-items: center; padding: 6px 0; }
    .roll { color: #2563eb; cursor: pointer; }
    .roll:hover { text-decoration: underline; }
    .value { cursor: pointer; min-width: 44px; text-align: right; }
    .muted { color: #6b7280; }
  </style>
</head>
<body>
  <div id="app" class="card"></div>

  <script>
    var _windowId = null;
    var _rollDispatchMode = 'default';
    function postEvent(action, payload) {
      if (!_windowId) return;
      window.parent.postMessage({ type: 'SEALCHAT_EVENT', version: 1, windowId: _windowId, action: action, payload: payload }, '*');
    }
    window.sealchat = {
      onUpdate: function(cb) {
        window.addEventListener('message', function(e) {
          if (e.source !== window.parent) return;
          if (e.data && e.data.type === 'SEALCHAT_UPDATE') { _windowId = e.data.payload.windowId; cb(e.data.payload); }
        });
      },
      setRollDispatchMode: function(mode) { _rollDispatchMode = mode === 'template' ? 'template' : 'default'; },
      setRollMode: function(mode) { _rollDispatchMode = mode === 'template' ? 'template' : 'default'; },
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', { roll: { template: template, label: label || '', args: args || {}, dispatchMode: _rollDispatchMode } });
      },
      updateAttrs: function(attrs) { postEvent('UPDATE_ATTRS', { attrs: attrs }); },
    };

    function escapeHtml(text) {
      var div = document.createElement('div');
      div.textContent = String(text == null ? '' : text);
      return div.innerHTML;
    }

    function render(data) {
      var attrs = (data && data.attrs) || {};
      var keys = ['力量', '敏捷', '体质', '意志', '侦查', '聆听'];

      var html = '';
      html += '<div class="title">' + escapeHtml((data && data.name) || '未命名角色') + '</div>';
      html += '<div class="muted">点击属性名掷骰，点击属性值可编辑</div>';

      for (var i = 0; i < keys.length; i += 1) {
        var key = keys[i];
        var raw = attrs[key];
        var val = raw == null ? '--' : String(raw);
        html += '<div class="row">';
        html += '  <span class="roll" data-roll=".ra {skill}" data-skill="' + escapeHtml(key) + '" data-label="' + escapeHtml(key + '检定') + '">' + escapeHtml(key) + '</span>';
        html += '  <span class="value" data-attr="' + escapeHtml(key) + '" data-value="' + escapeHtml(val) + '">' + escapeHtml(val) + '</span>';
        html += '  <span class="muted">%</span>';
        html += '</div>';
      }

      document.getElementById('app').innerHTML = html;
    }

    function promptAndUpdate(target) {
      var key = target.dataset.attr;
      if (!key) return;
      var current = target.dataset.value || '';
      var next = window.prompt('输入新的属性值（数字）', current === '--' ? '' : current);
      if (next == null) return;
      var num = Number(String(next).trim());
      if (!Number.isFinite(num)) return;
      var patch = {};
      patch[key] = num;
      sealchat.updateAttrs(patch);
    }

    document.addEventListener('click', function (e) {
      var target = e.target;
      while (target && target !== document.body) {
        if (target.dataset && target.dataset.attr) {
          promptAndUpdate(target);
          return;
        }
        if (target.dataset && target.dataset.roll) {
          sealchat.roll(
            target.dataset.roll,
            target.dataset.label || target.innerText || '',
            { skill: target.dataset.skill }
          );
          return;
        }
        target = target.parentElement;
      }
    });

    sealchat.onUpdate(render);
  </script>
</body>
</html>
~~~

---

## 6. 示例二：模板内直发模式（跳过默认掷骰窗口）

特点：展示如何启用模板模式。默认注释为示例状态，你可按需取消注释。

~~~html
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <style>
    body { font-family: Inter, "PingFang SC", "Microsoft YaHei", sans-serif; margin: 0; padding: 12px; }
    .panel { border: 1px solid #334155; border-radius: 10px; padding: 12px; background: #0f172a; color: #e2e8f0; }
    .name { font-size: 18px; margin-bottom: 10px; }
    .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
    .btn { border: 1px solid #475569; border-radius: 8px; background: #1e293b; color: #e2e8f0; padding: 8px 10px; cursor: pointer; text-align: left; }
    .btn:hover { border-color: #60a5fa; }
    .sub { color: #94a3b8; font-size: 12px; margin-top: 8px; }
  </style>
</head>
<body>
  <div id="app" class="panel"></div>

  <script>
    var _windowId = null;
    var _rollDispatchMode = 'default';
    function postEvent(action, payload) {
      if (!_windowId) return;
      window.parent.postMessage({ type: 'SEALCHAT_EVENT', version: 1, windowId: _windowId, action: action, payload: payload }, '*');
    }
    window.sealchat = {
      onUpdate: function(cb) {
        window.addEventListener('message', function(e) {
          if (e.source !== window.parent) return;
          if (e.data && e.data.type === 'SEALCHAT_UPDATE') { _windowId = e.data.payload.windowId; cb(e.data.payload); }
        });
      },
      setRollDispatchMode: function(mode) { _rollDispatchMode = mode === 'template' ? 'template' : 'default'; },
      setRollMode: function(mode) { _rollDispatchMode = mode === 'template' ? 'template' : 'default'; },
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', { roll: { template: template, label: label || '', args: args || {}, dispatchMode: _rollDispatchMode } });
      },
      updateAttrs: function(attrs) { postEvent('UPDATE_ATTRS', { attrs: attrs }); },
    };

    var current = { name: '', attrs: {} };

    function escapeHtml(text) {
      var div = document.createElement('div');
      div.textContent = String(text == null ? '' : text);
      return div.innerHTML;
    }

    function getSkillVal(skill) {
      var v = Number(current && current.attrs ? current.attrs[skill] : 0);
      return Number.isFinite(v) ? v : 0;
    }

    function render() {
      var skills = ['侦查', '聆听', '图书馆使用', '潜行'];
      var html = '';
      html += '<div class="name">' + escapeHtml(current.name || '未命名角色') + '</div>';
      html += '<div class="grid">';
      for (var i = 0; i < skills.length; i += 1) {
        var skill = skills[i];
        var val = getSkillVal(skill);
        html += '<button class="btn" data-skill="' + escapeHtml(skill) + '">';
        html += '  <div>' + escapeHtml(skill) + '</div>';
        html += '  <div class="sub">当前值: ' + escapeHtml(val) + '</div>';
        html += '</button>';
      }
      html += '</div>';
      html += '<div class="sub">已配置为模板内直发模式（可按需改回默认模式）</div>';
      document.getElementById('app').innerHTML = html;
    }

    // 示例：启用模板内直发掷骰（跳过默认掷骰窗口）
    // 默认请保持注释；需要模板全权处理时再取消注释
    // sealchat.setRollDispatchMode('template');

    // 也可以显式设置默认模式（可选）
    // sealchat.setRollDispatchMode('default');

    document.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-skill]');
      if (!btn) return;
      var skill = btn.dataset.skill;
      if (!skill) return;

      // 模板内部可以在这里自行做更多逻辑（冷却、资源判断、二次确认等）
      sealchat.roll('.ra {skill}', skill + '检定', { skill: skill });
    });

    sealchat.onUpdate(function (data) {
      current = {
        name: (data && data.name) || '',
        attrs: (data && data.attrs) || {},
      };
      render();
    });
  </script>
</body>
</html>
~~~

---

## 7. 开发规范与最佳实践

### 7.1 建议

- 尽量使用事件委托，减少大量元素逐个绑定监听。
- 属性编辑建议只发送 patch（updateAttrs({ key: value })），避免全量覆盖。
- 模板内保持渲染函数纯净 + 交互函数独立，便于维护。
- 复杂模板建议把样式和脚本结构化分区（render / handlers / utils）。

### 7.2 注意事项

- 模板运行在沙箱内，不应依赖外部全局对象。
- 不要假设宿主一定提供除 sealchat 外的额外 API。
- attrs 值类型可能是字符串、数字或对象，渲染前请做类型守卫。
- 当你启用模板模式（template）时，用户将不再看到默认掷骰弹窗。

---

## 8. 调试清单（Checklist）

发布模板前，建议至少验证：

1. 打开人物卡后能正确收到 onUpdate。
2. 属性编辑后 updateAttrs 生效并可回显。
3. 点击掷骰元素后：
   - 默认模式下弹出默认掷骰窗口；
   - 模板模式下直接发送指令。
4. 窗口刷新和重开后模板渲染正常。
5. 缺失属性、空角色名时页面不报错。

---

## 9. 与当前实现对应关系（便于查源）

- 模板桥接与事件类型：ui/src/views/chat/components/character-sheet/IframeSandbox.vue
- 掷骰分流（默认弹窗 vs 模板直发）：ui/src/views/chat/components/character-sheet/CharacterSheetManager.vue
- 默认掷骰弹窗：ui/src/views/chat/components/character-sheet/DiceRollPopover.vue
- 默认模板源码（通用 + COC）：ui/src/stores/characterSheet.ts



## 10 COC角色卡自定义掷骰模板示例



~~~html
<script>
  var _windowId = null;
  var _rollDispatchMode = "default";

  function postEvent(action, payload) {
    if (!_windowId) return;
    window.parent.postMessage({
      type: "SEALCHAT_EVENT",
      version: 1,
      windowId: _windowId,
      action: action,
      payload: payload
    }, "*");
  }

  window.sealchat = {
    onUpdate: function (cb) {
      window.addEventListener("message", function (e) {
        if (e.source !== window.parent) return;
        if (e.data && e.data.type === "SEALCHAT_UPDATE") {
          _windowId = e.data.payload.windowId;
          cb(e.data.payload);
        }
      });
    },
    setRollDispatchMode: function (mode) {
      _rollDispatchMode = mode === "template" ? "template" : "default";
    },
    roll: function (template, label, args) {
      postEvent("ROLL_DICE", {
        roll: {
          template: template,
          label: label || "",
          args: args || {},
          dispatchMode: _rollDispatchMode
        }
      });
    },
    updateAttrs: function (attrs) {
      postEvent("UPDATE_ATTRS", { attrs: attrs });
    }
  };

  window.sealchat.setRollDispatchMode("template");
  window.sealchat.onUpdate(function (data) {
    render(data);
  });
</script>


<!DOCTYPE html>
<!-- sealchat-custom-roll:v1-coc-dark-mode -->
<html>
<head>
  <meta charset="UTF-8">
  <style>
    :root {
      /* 基础配色 */
      --c-bg: #0f1115;
      --c-card-bg: #161920;
      --c-text-main: #c9d1d9;
      --c-text-dim: #6e7681;
      --c-accent: #3fb950;
      --c-danger: #f85149;
      --c-magic: #a371f7;
      --c-sanity: #e3b341;
      --c-border: #30363d;
      --c-hover: #21262d;
      
      /* 模态框配色 */
      --c-modal-bg: #1c2128;
      --c-btn-bg: #21262d;
      --c-btn-border: #30363d;
      --c-btn-hover: #30363d;
      
      --font-serif: "Songti SC", "SimSun", "Georgia", serif;
      --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }

    * { box-sizing: border-box; margin: 0; padding: 0; }
    
    body {
      font-family: var(--font-sans);
      background: var(--c-bg);
      color: var(--c-text-main);
      padding: 12px;
      padding-bottom: 40px; /* 为底部留空 */
      font-size: 13px;
      line-height: 1.5;
    }

    /* 滚动条 */
    ::-webkit-scrollbar { width: 4px; height: 4px; }
    ::-webkit-scrollbar-track { background: transparent; }
    ::-webkit-scrollbar-thumb { background: var(--c-border); border-radius: 2px; }

    /* 容器 */
    .sheet-container {
      max-width: 600px;
      margin: 0 auto;
      background: var(--c-card-bg);
      border: 1px solid var(--c-border);
      border-radius: 6px;
      box-shadow: 0 4px 20px rgba(0,0,0,0.5);
      overflow: hidden;
    }

    /* --- 原始样式保持不变 --- */
    .header {
      display: flex; align-items: center; padding: 16px;
      background: linear-gradient(180deg, rgba(22,25,32,1) 0%, rgba(13,17,23,1) 100%);
      border-bottom: 1px solid var(--c-border);
    }
    .avatar {
      width: 56px; height: 56px; border-radius: 4px; background: #000;
      border: 1px solid var(--c-border); display: flex; align-items: center; justify-content: center;
      margin-right: 16px; overflow: hidden; font-size: 24px; color: var(--c-text-dim); flex-shrink: 0;
    }
    .avatar img { width: 100%; height: 100%; object-fit: cover; }
    .info { flex: 1; }
    .name { font-family: var(--font-serif); font-size: 20px; font-weight: bold; color: #fff; margin-bottom: 4px; }
    .pl-label { font-size: 11px; color: var(--c-text-dim); text-transform: uppercase; letter-spacing: 1px; }

    .status-bar { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 1px; background: var(--c-border); border-bottom: 1px solid var(--c-border); }
    .status-item { background: var(--c-card-bg); padding: 8px 12px; text-align: center; }
    .status-label { font-size: 11px; color: var(--c-text-dim); display: block; margin-bottom: 2px; }
    .status-val { font-family: var(--font-serif); font-size: 18px; font-weight: bold; cursor: pointer; border-bottom: 1px dashed transparent; }
    .status-val:hover { border-bottom-color: currentColor; }
    .st-hp .status-val { color: var(--c-danger); }
    .st-mp .status-val { color: var(--c-magic); }
    .st-san .status-val { color: var(--c-sanity); }

    .section-title {
      font-family: var(--font-serif); background: rgba(255,255,255,0.03); color: var(--c-text-dim);
      font-size: 12px; padding: 6px 16px; border-bottom: 1px solid var(--c-border); border-top: 1px solid var(--c-border);
      margin-top: -1px; text-transform: uppercase; letter-spacing: 1px;
    }
    .stats-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1px; background: var(--c-border); padding-bottom: 1px; }
    .stat-box { background: var(--c-card-bg); padding: 8px; display: flex; justify-content: space-between; align-items: center; }
    .stat-box:hover { background: var(--c-hover); }
    .stat-name { color: var(--c-text-dim); font-size: 12px; cursor: pointer; }
    .stat-name:hover { color: var(--c-accent); text-decoration: underline; }
    .stat-val { font-family: var(--font-serif); font-size: 16px; font-weight: 600; color: var(--c-text-main); cursor: pointer; min-width: 24px; text-align: right; }
    
    .skills-container { padding: 12px; column-count: 2; column-gap: 20px; }
    @media (max-width: 400px) { .skills-container { column-count: 1; } }
    .skill-item { display: flex; justify-content: space-between; align-items: center; padding: 4px 0; border-bottom: 1px solid rgba(48, 54, 61, 0.5); break-inside: avoid; }
    .skill-item:hover { background: rgba(255,255,255,0.02); }
    .skill-name { font-size: 12px; cursor: pointer; color: var(--c-text-main); }
    .skill-name:hover { color: var(--c-accent); }
    .skill-val { font-family: var(--font-serif); font-size: 13px; color: var(--c-text-dim); cursor: pointer; padding: 0 4px; }

    /* 编辑框 */
    .inline-editor { width: 50px; background: #000; color: #fff; border: 1px solid var(--c-accent); border-radius: 2px; text-align: center; }
    .empty-msg { padding: 40px; text-align: center; color: var(--c-text-dim); font-style: italic; }

    /* =========================================
       自定义掷骰面板 (Custom Roll Modal)
       ========================================= */
    .modal-overlay {
      position: fixed; top: 0; left: 0; width: 100%; height: 100%;
      background: rgba(0,0,0,0.7);
      display: none; justify-content: center; align-items: center;
      z-index: 100;
      backdrop-filter: blur(2px);
    }
    .modal-overlay.active { display: flex; animation: fadeIn 0.2s; }
    
    @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }

    .roll-modal {
      background: var(--c-modal-bg);
      border: 1px solid var(--c-border);
      border-radius: 8px;
      width: 90%; max-width: 320px;
      box-shadow: 0 10px 30px rgba(0,0,0,0.8);
      padding: 16px;
    }

    .modal-header {
      margin-bottom: 16px; text-align: center;
      border-bottom: 1px solid var(--c-border);
      padding-bottom: 12px;
    }
    .modal-skill-name { font-size: 18px; font-weight: bold; color: #fff; }
    .modal-skill-val { font-family: var(--font-serif); color: var(--c-accent); font-size: 24px; display: block; margin-top: 4px; }

    .roll-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-bottom: 12px; }
    
    .btn-roll {
      background: var(--c-btn-bg);
      border: 1px solid var(--c-btn-border);
      color: var(--c-text-main);
      padding: 10px; border-radius: 6px;
      cursor: pointer; font-size: 13px;
      transition: all 0.2s;
      display: flex; flex-direction: column; align-items: center; justify-content: center;
    }
    .btn-roll:hover { background: var(--c-btn-hover); border-color: var(--c-text-dim); }
    .btn-roll:active { transform: translateY(1px); }
    
    .btn-roll span { font-size: 10px; opacity: 0.6; margin-top: 2px; }

    /* 特殊按钮颜色 */
    .btn-normal { grid-column: span 2; background: rgba(63, 185, 80, 0.1); border-color: rgba(63, 185, 80, 0.3); color: #fff; font-weight: bold; }
    .btn-normal:hover { background: rgba(63, 185, 80, 0.2); }
    
    .btn-hidden { grid-column: span 2; background: rgba(163, 113, 247, 0.1); border-color: rgba(163, 113, 247, 0.3); color: #d2a8ff; }
    .btn-hidden:hover { background: rgba(163, 113, 247, 0.2); }

    .btn-cancel {
      width: 100%; padding: 8px; background: transparent; border: none;
      color: var(--c-text-dim); cursor: pointer; margin-top: 8px;
    }
    .btn-cancel:hover { color: #fff; }

  </style>
</head>
  <script>

    function normalizeRollDispatchMode(mode) {
      return mode === 'template' ? 'template' : 'default';
    }

    function withRollDispatchMode(roll) {
      return Object.assign({}, roll || {}, { dispatchMode: _rollDispatchMode });
    }

    function postEvent(action, payload) {
      if (!_windowId) return;
      window.parent.postMessage({
        type: 'SEALCHAT_EVENT',
        version: 1,
        windowId: _windowId,
        action: action,
        payload: payload
      }, '*');

    window.sealchat = {
      onUpdate: function(cb) {
          if (e.data && e.data.type === 'SEALCHAT_UPDATE') {
            _windowId = e.data.payload.windowId;
            cb(e.data.payload);
          }
        });
      setRollDispatchMode: function(mode) {
      },
      },
      roll: function(template, label, args) {
        postEvent('ROLL_DICE', {
          roll: withRollDispatchMode({
            template: template,
            label: label || '',
            args: args || {}
          })
        });
      },
      updateAttrs: function(attrs) {
        postEvent('UPDATE_ATTRS', { attrs: attrs });
      }
    };
  </script>
<body>
  <!-- 主界面容器 -->
  <div id="content"></div>

  <!-- 自定义掷骰模态框 -->
  <div id="rollModal" class="modal-overlay">
    <div class="roll-modal">
      <div class="modal-header">
        <div class="modal-skill-name" id="modalTitle">技能名</div>
        <span class="modal-skill-val" id="modalVal">50</span>
      </div>

      <!-- 按钮组 -->
      <div class="roll-actions">
        <!-- 普通检定 -->
        <button class="btn-roll btn-normal" onclick="confirmRoll('')">
          普通检定
          <span>.ra</span>
        </button>
        
        <!-- 暗骰 (用户需求重点) -->
        <button class="btn-roll btn-hidden" onclick="confirmRoll('h')">
          暗骰 / 隐藏
          <span>.rah</span>
        </button>

        <!-- 奖励/惩罚 -->
        <button class="btn-roll" onclick="confirmRoll('b1')">奖励骰 1 <span>.ra b1</span></button>
        <button class="btn-roll" onclick="confirmRoll('p1')">惩罚骰 1 <span>.ra p1</span></button>
        <button class="btn-roll" onclick="confirmRoll('b2')">奖励骰 2 <span>.ra b2</span></button>
        <button class="btn-roll" onclick="confirmRoll('p2')">惩罚骰 2 <span>.ra p2</span></button>
      </div>

      <button class="btn-cancel" onclick="closeModal()">取消</button>
    </div>
  </div>

  <script>
    // --- 1. 初始化设置 ---
    // 关键：接管掷骰事件，不再弹出系统默认窗口
    try {
      if (window.sealchat && window.sealchat.setRollDispatchMode) {
        window.sealchat.setRollDispatchMode('template');
        console.log('Template roll mode enabled.');
      }
    } catch (e) { console.error(e); }

    // --- 2. 状态与变量 ---
    var _windowId = null;
    var _currentSkill = null; // 当前正在掷骰的技能名
    var _currentVal = 0;      // 当前技能值

    // --- 3. 基础工具函数 ---
    function escapeHtml(text) {
      if (text === null || text === undefined) return '';
      var div = document.createElement('div');
      div.textContent = text;
      return div.innerHTML;
    }

    // --- 4. 渲染逻辑 (与原CoC Dark模板一致，仅微调) ---
    const STAT_KEYS = ['力量', '体质', '体型', '敏捷', '外貌', '智力', '意志', '教育', '幸运'];
    const STATUS_KEYS = ['生命值', '魔法值', '理智'];
    const STATUS_MAP = {
      '生命值': { cls: 'st-hp', label: 'HP' },
      '魔法值': { cls: 'st-mp', label: 'MP' },
      '理智': { cls: 'st-san', label: 'SAN' }
    };

    function render(data) {
      var el = document.getElementById('content');
      if (!data || !data.attrs || Object.keys(data.attrs).length === 0) {
        el.innerHTML = '<div class="empty-msg">Waiting for data...</div>';
        return;
      }

      var attrs = data.attrs;
      var otherSkills = [];
      var foundStats = {};
      var foundStatus = {};

      for (var key in attrs) {
        if (STAT_KEYS.includes(key)) foundStats[key] = attrs[key];
        else if (STATUS_KEYS.includes(key)) foundStatus[key] = attrs[key];
        else {
          var val = attrs[key];
          if (typeof val === 'number' || (typeof val === 'string' && /^-?\d+$/.test(val))) {
             otherSkills.push({ key: key, val: val });
          }
        }
      }
      otherSkills.sort(function(a, b) { return a.key.localeCompare(b.key, 'zh'); });

      var html = '<div class="sheet-container">';

      // Header
      var avatarUrl = data.avatarUrl || '';
      var avatarHtml = avatarUrl ? '<img src="' + escapeHtml(avatarUrl) + '">' : (data.name || '?').charAt(0);
      html += '<div class="header"><div class="avatar">' + avatarHtml + '</div><div class="info"><div class="name">' + escapeHtml(data.name || 'Unknown') + '</div><div class="pl-label">CoC Investigator</div></div></div>';

      // Status Bar
      html += '<div class="status-bar">';
      STATUS_KEYS.forEach(function(k) {
        var conf = STATUS_MAP[k];
        var val = foundStatus[k] !== undefined ? foundStatus[k] : '--';
        html += '<div class="status-item ' + conf.cls + '"><span class="status-label">' + conf.label + '</span><div class="status-val" data-attr="' + k + '" data-value="' + val + '">' + val + '</div></div>';
      });
      html += '</div>';

      // Stats
      html += '<div class="section-title">Characteristics</div><div class="stats-grid">';
      STAT_KEYS.forEach(function(k) {
        var val = foundStats[k] !== undefined ? foundStats[k] : '';
        if (val === '') return;
        html += '<div class="stat-box">';
        // 注意：data-roll 保留作为标识，但实际行为被JS拦截
        html +=   '<span class="stat-name" data-roll-target="' + k + '">' + k + '</span>';
        html +=   '<span class="stat-val" data-attr="' + k + '" data-value="' + val + '">' + val + '</span>';
        html += '</div>';
      });
      html += '</div>';

      // Skills
      if (otherSkills.length > 0) {
        html += '<div class="section-title">Skills</div><div class="skills-container">';
        otherSkills.forEach(function(item) {
          html += '<div class="skill-item">';
          html +=   '<span class="skill-name" data-roll-target="' + escapeHtml(item.key) + '">' + escapeHtml(item.key) + '</span>';
          html +=   '<span class="skill-val" data-attr="' + item.key + '" data-value="' + item.val + '">' + item.val + '</span>';
          html += '</div>';
        });
        html += '</div>';
      }
      html += '</div>';
      el.innerHTML = html;
    }

    // --- 5. 交互逻辑：自定义模态框 ---

    function openModal(skill, val) {
      _currentSkill = skill;
      _currentVal = val;
      
      document.getElementById('modalTitle').textContent = skill;
      document.getElementById('modalVal').textContent = val;
      document.getElementById('rollModal').classList.add('active');
    }

    function closeModal() {
      document.getElementById('rollModal').classList.remove('active');
      _currentSkill = null;
    }

    // 执行掷骰
    // type: '' (普通), 'h' (暗骰), 'b1' (奖励1)...
    window.confirmRoll = function(type) {
      if (!_currentSkill) return;
      
      var template = '.ra'; // 默认
      var label = _currentSkill + '检定';

      // 构建指令
      if (type === 'h') {
        template = '.rah'; // 暗骰
        label = '暗骰 ' + label;
      } else if (type) {
        // b1, p1, etc.
        template = '.ra ' + type;
      }

      // 加上技能变量
      template += ' {skill}';

      // 发送给宿主
      window.sealchat.roll(template, label, { skill: _currentSkill });
      
      closeModal();
    };

    // --- 6. 交互逻辑：数值编辑 ---
    
    function openInlineEditor(target) {
      if (target.dataset.editing === '1') return;
      var attrKey = target.dataset.attr;
      var currentVal = target.dataset.value;
      if (currentVal === '--') currentVal = ''; 
      
      var input = document.createElement('input');
      input.type = 'number';
      input.value = currentVal;
      input.className = 'inline-editor';
      var originalWidth = target.offsetWidth;
      input.style.width = Math.max(originalWidth + 20, 50) + 'px';

      target.textContent = '';
      target.appendChild(input);
      target.dataset.editing = '1';
      input.focus(); input.select();

      var commit = function() {
        var val = input.value.trim();
        var num = Number(val);
        if (val === '' || isNaN(num)) { cancel(); return; }
        target.textContent = val;
        target.dataset.value = val;
        target.dataset.editing = '';
        var patch = {}; patch[attrKey] = num;
        
        // 使用官方API更新属性
        if (window.sealchat.updateAttrs) {
          window.sealchat.updateAttrs(patch);
        }
      };
      var cancel = function() { target.textContent = currentVal || '--'; target.dataset.editing = ''; };
      input.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') { e.preventDefault(); commit(); }
        if (e.key === 'Escape') { e.preventDefault(); cancel(); }
      });
      input.addEventListener('blur', commit);
      input.addEventListener('click', function(e) { e.stopPropagation(); });
    }

    // --- 7. 全局事件监听 ---

    document.addEventListener('click', function(e) {
      var target = e.target;
      
      // A. 点击数值 -> 编辑
      if (target.classList.contains('stat-val') || 
          target.classList.contains('skill-val') || 
          target.classList.contains('status-val')) {
        openInlineEditor(target);
        return;
      }

      // B. 点击技能名 -> 打开自定义模态框
      // 向上查找包含 data-roll-target 的元素
      var rollEl = target.closest('[data-roll-target]');
      if (rollEl) {
        var skillName = rollEl.dataset.rollTarget;
        // 尝试从兄弟节点获取数值用于展示
        var valNode = rollEl.parentElement.querySelector('.stat-val, .skill-val');
        var val = valNode ? (valNode.dataset.value || '') : '';
        
        openModal(skillName, val);
        return;
      }
      
      // C. 点击遮罩层关闭
      if (target.id === 'rollModal') {
        closeModal();
      }
    });

    // 注册更新回调
    if (window.sealchat && window.sealchat.onUpdate) {
      window.sealchat.onUpdate(function(data) {
        // 缓存 windowId (如果需要)
        if (data.windowId) _windowId = data.windowId;
        render(data);
      });
    }
  </script>
</body>
</html>
~~~
