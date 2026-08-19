# Fluid Modal 弹窗模板

## 目的

为后台和管理类弹窗提供一套统一的“宽度自适应 + 内部表格横向滚动”模板，避免以下问题：

- 弹窗宽度只靠局部内联 `style`，难复用，难维护
- `n-modal` Teleport 后常规局部样式命中不稳定
- 表格列较多时，直接把弹窗整体撑爆
- 窄屏下内容裁切、分页换行、横向滚动条位置异常

当前模板已用于：

- 用户音频配额
- 清理指定周期未使用素材

相关实现文件：

- [`ui/src/assets/main.css`](../../ui/src/assets/main.css#L473)
- [`ui/src/views/admin/components/AdminAudioQuotaModal.vue`](../../ui/src/views/admin/components/AdminAudioQuotaModal.vue#L304)
- [`ui/src/views/admin/admin-settings-audio.vue`](../../ui/src/views/admin/admin-settings-audio.vue#L769)

## 设计原则

1. 模板类直接挂在 `n-modal` 渲染后的 `n-card` / `n-dialog` 本体上。
2. 宽度由全局模板控制，不依赖每个弹窗单独写内联宽度。
3. 弹窗本身不被表格内容撑爆；宽表格只在内容区横向滚动。
4. 移动端自动缩小左右边距，保持可视区域安全。

## 模板类

定义位置：`ui/src/assets/main.css`

### 基础类

- `sc-fluid-modal`

作用：

- 建立弹窗左右边距变量 `--sc-modal-side-gap`
- 设置统一宽度变量 `--sc-modal-width`
- 直接控制 `n-card` / `n-dialog` 宽度与最大宽度
- 确保 header/content/footer 可收缩，不因子元素撑裂布局

### 宽度档位

- `sc-fluid-modal--wide`
  - 上限 `900px`
  - 适合带中等宽度表格、表单、说明文本的弹窗
- `sc-fluid-modal--xwide`
  - 上限 `1240px`
  - 适合双栏布局、宽表格、管理面板类弹窗

### 表格滚动类

- `sc-modal-table-scroll`

作用：

- 仅为内部内容区提供横向滚动
- 不拉长弹窗外框
- 适合包裹 `n-data-table`、原生 `table`、宽表单栅格

## 正确用法

### 1. 给弹窗挂模板类

`preset="card"` 示例：

```vue
<n-modal
  v-model:show="visible"
  preset="card"
  title="用户音频配额"
  class="sc-fluid-modal sc-fluid-modal--xwide"
>
  ...
</n-modal>
```

`preset="dialog"` 示例：

```vue
<n-modal
  v-model:show="visible"
  preset="dialog"
  title="清理指定周期未使用素材"
  class="sc-fluid-modal sc-fluid-modal--wide"
>
  ...
</n-modal>
```

### 2. 宽表格外包滚动容器

```vue
<div class="sc-modal-table-scroll">
  <n-data-table
    :columns="columns"
    :data="rows"
    :scroll-x="1280"
    :pagination="false"
  />
</div>
```

说明：

- `scroll-x` 写“表格期望最小宽度”，不是弹窗宽度
- 弹窗宽度由 `sc-fluid-modal*` 控制
- 表格超出时，在容器内横滚，不扩张外层弹窗

### 3. 业务组件局部样式只管布局，不重复控宽

推荐：

```css
.example-modal__section {
  min-width: 0;
}
```

不推荐：

```css
.example-modal :deep(.n-card) {
  max-width: 98vw;
}
```

或：

```vue
<n-modal :style="{ width: 'min(1320px, 98vw)' }" />
```

原因：

- 会与全局模板职责重叠
- 后续难统一调整
- Teleport 结构变化时更容易失效或互相覆盖

## 选择宽度档位建议

用 `sc-fluid-modal--wide`：

- 单栏表单
- 中等列数表格
- 内容主宽度约 `700px - 900px`

用 `sc-fluid-modal--xwide`：

- 双栏布局
- 管理后台资料面板
- 多列数据表格
- 左侧列表 + 右侧详情

如果现有两档都不合适，优先新增新档位，例如：

```css
body .n-card.sc-fluid-modal.sc-fluid-modal--compact,
body .n-dialog.sc-fluid-modal.sc-fluid-modal--compact {
  --sc-modal-width: min(640px, calc(100vw - var(--sc-modal-side-gap)));
}
```

不要直接回退到每个弹窗单独写内联宽度。

## 禁忌

### 1. 不要用祖先后代选择器假设命中 modal 外壳

错误思路：

```css
body .some-modal .n-card {
  width: ...;
}
```

原因：

- `n-modal` Teleport 后，类往往直接落在 `n-card` / `n-dialog` 本体
- 不是普通嵌套 DOM，祖先关系常常不存在

### 2. 不要给滚动容器子元素写 `min-width: max-content`

这会导致：

- 内容区被强行拉成长条
- 弹窗视觉上“拖到很宽”
- 空白区域异常增大

### 3. 不要让弹窗宽度和表格宽度绑死

错误：

- 弹窗 `width` 跟着表格列宽增长

正确：

- 弹窗宽度固定在模板档位
- 表格自己横滚

## 验证清单

改完弹窗后，至少检查：

1. 桌面端宽度是否在预期档位内
2. 移动端左右是否仍有安全边距
3. 表格超宽时是否只在内容区横向滚动
4. Header / Content / Footer 是否仍对齐
5. 分页、按钮区是否未被挤压错位
6. `npm run -s type-check` 是否通过

## 后续扩展建议

- 后台所有“大表格弹窗”统一迁到 `sc-fluid-modal`
- 如果后续出现抽屉、悬浮面板等同类需求，可拆出并行模板：
  - `sc-fluid-drawer`
  - `sc-fluid-panel`
- 若表格场景持续增多，可补一个通用业务包装组件，例如 `AdminTableModalShell`
