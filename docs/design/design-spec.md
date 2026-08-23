# 前端设计规范（v2 — iOS + Notion 系统风格）

> 采用 iOS Human Interface Guidelines 的清晰层级与留白哲学，结合 Notion 的极简排版与柔和阴影。
> 服务 New Vision —— GB/T 28181 视频接入节点管理平台。
> **本文档是活的**：新增/修改任何页面样式前必须先查本文档；产生任何新样式值必须同步回本文档。

---

## 一、设计理念

| 原则 | 说明 |
|------|------|
| **内容优先** | 界面是内容的容器，不抢戏。装饰服务于信息层级，而非相反。 |
| **系统灰画布 + 白色卡片** | 页面背景统一 `#F2F2F7`（iOS 系统灰）；所有内容块装进白色卡片（16px 圆角 + 弥散阴影 + 极淡边框）。层级靠卡片堆叠表达，不用色块。 |
| **克制即优雅** | 减少不必要的线条与颜色。用间距和字重建立层次，而非边框。 |
| **大圆角 + 大量留白** | 卡片 16px、输入框 12px、标签/筛选/主按钮 999px 药丸。字段间距 16px（密集场景）~ 32px（现代场景）。 |
| **弥散阴影** | 层级用柔和阴影表达（`0 1px 3px` / `0 4px 20px`），不靠硬边框。 |
| **毛玻璃表达悬浮层** | Sticky 标题栏、模态框等悬浮元素用 `backdrop-filter` 毛玻璃。 |
| **系统感** | 组件行为遵循平台直觉：hover 浮现操作、按压反馈（`scale(0.985)`）、滚动惯性。 |
| **可访问性** | 文字对比度满足 WCAG AA；支持 `prefers-reduced-motion`；focus 可见（`#007AFF` 轮廓）。 |

---

## 二、颜色系统（iOS 系统色）

### 2.1 Token 总表（浅色模式）

| Token | 值 | 用途 |
|-------|-----|------|
| `--bg-page` | `#F2F2F7` | 页面底层背景 |
| `--bg-card` | `#FFFFFF` | 所有卡片/容器背景 |
| `--bg-table-header` | `#FAFAFA` | 表格表头背景 |
| `--bg-input` | `#FAFAFA` | 搜索框/输入框背景 |
| `--bg-hover` | `#FAFAFA` | 表格行 hover 背景 |
| `--bg-pill-inactive` | `#F2F2F7` | 未选中 Pill 背景 |
| `--bg-tag-blue` | `#E5F0FF` | 「本人」标签背景 |
| `--bg-tag-purple` | `#F3E8FF` | 角色标签背景（节点管理员） |
| `--text-primary` | `#1C1C1E` | 主标题、大数字、用户名 |
| `--text-secondary` | `#3A3A3C` | 正文、次要信息 |
| `--text-tertiary` | `#8E8E93` | 占位符、说明文字、图标默认色 |
| `--text-muted` | `#C7C7CC` | 更弱的提示文字、空状态 |
| `--border-light` | `#F2F2F7` | 卡片内部分隔线、表头下边框 |
| `--border-input` | `#E5E5EA` | 输入框边框、Pill 未选中边框 |
| `--border-card` | `rgba(0,0,0,0.03)` | 卡片外边框（极淡） |
| `--accent-blue` | `#007AFF` | 链接、选中态、本人标签文字、focus 轮廓 |
| `--accent-green` | `#34C759` | 启用状态圆点 |
| `--accent-purple` | `#7C3AED` | 角色标签文字（节点管理员） |
| `--accent-red` | `#FF3B30` | 删除/停用 hover、必填标记、危险状态圆点 |
| `--accent-yellow` | `#FFCC00` | 警示（装饰圆点等） |
| `--btn-primary-bg` | `#1C1C1E` | 主按钮背景（深色，白色文字） |
| `--shadow-card` | `0 1px 3px rgba(0,0,0,0.04)` | 卡片基础阴影 |
| `--shadow-elevated` | `0 4px 20px rgba(0,0,0,0.04)` | 弥散阴影（浮层/强调） |
| `--shadow-button` | `0 2px 8px rgba(0,0,0,0.12)` | 主按钮阴影 |

### 2.2 语义色

```
Success ── #34C759（状态圆点/成功反馈）
Danger  ── #FF3B30（删除、停用 hover、必填星号）
Warning ── #FFCC00（警示装饰）
Accent  ── #007AFF（链接、选中 Pill、focus 轮廓、本人标签文字）
```

> 正文文字不得使用 `#8E8E93` 及更浅色（对比度不足，见第十二节）。

---

## 三、字体

### 3.1 字族（v2 起弃用 DM Sans / Noto Sans SC）

```
font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
```

- 系统字体栈：零加载、跨平台观感一致、iOS 原生质感。
- 代码／设备 ID：等宽字体（`'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace`）。

### 3.2 字号层级

| 用途 | 字号 | 字重 | 字距 |
|------|------|------|------|
| 页面标题（h1） | 28px | 700 | -0.8px |
| 卡片标题 | 22px | 700 | -0.5px |
| 大数字（统计卡） | 32px | 700 | -1px |
| 区块标题 | 15px | 600 | — |
| 正文 | 14–16px | 400–500 | -0.2 ~ -0.3px |
| 辅助文字 | 13px | 400–500 | — |
| 标签/表头 | 12px | 600–700 | 0 ~ 0.5px |
| 微型 | 11px | 600 | — |

> **负字距是 iOS 微排版的精髓**：标题 -0.5 ~ -0.8px、正文/按钮 -0.2 ~ -0.3px，务必保留。

---

## 四、间距与布局

- 以 **4px 为最小单位**（间距取 4 的倍数）。
- 页面内容区：`max-width: 1200px`、`margin: 0 auto`、左右 `padding: 32px`（移动端 16px）。
- 卡片内边距：20–32px。
- 区块间距：统计卡片区下 28px；搜索筛选卡片下 16px；表格卡片下 16px。
- 统计卡片：`grid-template-columns: repeat(4, 1fr)`，`gap: 16px`。
- 字段间距：现代范式 24–32px；密集场景 16px。
- 搜索筛选区：`flex-wrap: wrap`，搜索框 `min-width: 240px`。

---

## 五、圆角

| 层级 | 值 | 用途 |
|------|-----|------|
| sm | 8px | 小图标按钮、行内操作按钮、分页按钮 |
| md | 10px | 刷新按钮 |
| lg | 12px | 搜索框、输入框 |
| xl | 16px | 卡片、统计卡、表格卡、空状态、错误提示 |
| full | 999px | Pill 标签、状态筛选、主按钮、头像 badge |

**规则：** 卡片统一 16px；输入控件 12px；标签/筛选/主按钮用药丸形（999px）。圆角层级要一致，不允许同层级混用。

---

## 六、阴影与材质

### 6.1 阴影

| 层级 | 值 | 用途 |
|------|-----|------|
| card | `0 1px 3px rgba(0,0,0,0.04)` + `border: 1px solid rgba(0,0,0,0.03)` | 所有卡片（统计、搜索筛选、表格、空状态） |
| elevated | `0 4px 20px rgba(0,0,0,0.04)` | 弥散阴影（浮层、强调） |
| button | `0 2px 8px rgba(0,0,0,0.12)` | 深色主按钮 |

- 状态圆点外发光：`box-shadow: 0 0 0 3px rgba(52,199,89,0.15)` —— 这是 iOS 精致感的关键细节，不要遗漏。
- 所有卡片阴影一致，保持统一质感。

### 6.2 毛玻璃（Glass Material）

用于**悬浮层**：Sticky 标题栏、模态框等。

```
background: rgba(255,255,255,0.85);
-webkit-backdrop-filter: blur(20px);
backdrop-filter: blur(20px);
border-bottom: 1px solid rgba(0,0,0,0.05);
```

- 必须提供 `-webkit-` 前缀；不支持 `backdrop-filter` 时降级为近实底背景（≥0.92 不透明）。
- 毛玻璃上的文字与控件保持实底，对比度不依赖模糊。
- 静态内容上的面板用实底卡片 + 阴影即可，不滥用模糊。

---

## 七、页面骨架与组件规范

标准页面采用**五区纵向堆叠**：

```
┌──────────────────────────────────────────┐
│ 深色站点导航栏（全站统一，sticky top:0）    │
├──────────────────────────────────────────┤
│ 毛玻璃 Sticky 页面标题栏                   │
│   （主标题 + 右侧主操作按钮）              │
├──────────────────────────────────────────┤
│ 统计卡片区（4 列 grid）                    │
│ 搜索与筛选卡片（搜索框 + Pill + 刷新）     │
│ 列表表格卡片（Grid 布局、hover 浮现操作）  │
│ 分页 / 空状态 / 错误 / 加载骨架            │
├──────────────────────────────────────────┤
│ 页脚（可选，保持克制）                    │
└──────────────────────────────────────────┘
```

### 7.1 毛玻璃 Sticky 标题栏

- `position: sticky; top: <站点导航栏高度>; z-index: 10`（当前导航栏 62px → top: 62px）。
- 主标题：28px / 700 / -0.8px / `#1C1C1E`。
- 副标题可选：14px / `#8E8E93` / line-height 1.5。
- 右侧主操作按钮（如「新增用户」）。
- **禁止出现营销式/技术科普式描述文字**（如「基于 Casbin 域内 RBAC 实时生效」），正式系统保持克制。

### 7.2 统计卡片

- 4 列 grid、gap 16px、圆角 16px、`padding: 20px 24px`。
- 结构：图标/标识行（flex，gap 8px，13px / `#8E8E93` / 500）+ 大数字（32px / 700 / -1px / line-height 1 / `#1C1C1E`）。
- 标识行示例：用户图标、绿点（启用中）、红点（已停用）、盾牌图标（管理员）。

### 7.3 搜索与筛选卡片

- 白色卡片，`padding: 16px 20px`，内部 flex `gap: 16px`、`flex-wrap: wrap`。
- **搜索框**：相对定位，flex 1，`min-width: 240px`；放大镜图标绝对定位 `left: 14px` 垂直居中（16px，`#C7C7CC`，stroke-width 2.5）；输入框 `padding: 10px 14px 10px 40px`、`border: 1.5px solid #E5E5EA`、圆角 12px、15px、`#FAFAFA`、focus 边框变 `#007AFF` + 3px `rgba(0,122,255,0.12)` 光晕。
- **状态 Pill 组**：选中 `#1C1C1E` 白字 600；未选中 `#F2F2F7` / `#636366` / 500 + `1px solid #E5E5EA`。padding `8px 16px`，999px，gap 6px。
- **刷新按钮**：36×36、圆角 10px、`border: 1.5px solid #E5E5EA`、白底、`#636366`。

### 7.4 表格

- 卡片容器（`overflow: hidden` 保证圆角裁剪内部）。
- **CSS Grid 布局** + `role="row" / "columnheader" / "cell"` 语义（本系统不用原生 `<table>` 实现列表页表格）。
- 列定义按内容比例：用户页示例 `grid-template-columns: 2.5fr 1fr 1.5fr 1fr 1.5fr 100px`。
- 表头：`#FAFAFA`、12px / 600 / `#8E8E93`、`uppercase`、`letter-spacing: 0.5px`、`padding: 14px 24px`、下边框 `#F2F2F7`。
- 行：`padding: 16px 24px`、`align-items: center`、下边框 `#F2F2F7`、hover `#FAFAFA`（transition 0.15s）。
- **操作列**：右对齐；操作按钮默认 `opacity: 0`，行 hover 或按钮 focus 时浮现（降低噪音）。
- **头像**：40px 圆形，`linear-gradient(135deg, #667eea 0%, #764ba2 100%)`，首字母白字 15px/600，居中。
- **「本人」标签**：`#E5F0FF` 底 + `#007AFF` 字，999px，11px/700。
- **角色标签**：低饱和底 + 同色系深字（节点管理员 `#F3E8FF` / `#7C3AED`）。
- **状态**：8px 圆点 + 外发光（`box-shadow: 0 0 0 3px rgba(52,199,89,0.15)`），旁附 14px/500 文字。

### 7.5 按钮

- **主按钮**：深色 `#1C1C1E`、白字 14px/600、999px、`padding: 10px 20px`、`box-shadow: 0 2px 8px rgba(0,0,0,0.12)`、字距 -0.2px、hover 背景 `#2c2c2e`、active `scale(0.985)`。
- **文字/次要按钮**：透明底，`#3A3A3C` → hover `#1C1C1E` + `#F2F2F7` 底。
- **行内操作按钮**（表格行内）：32×32、圆角 8px、默认 `#8E8E93`；hover 底 `#F2F2F7` 字 `#1C1C1E`；危险操作 hover 底 `#FFF2F2` 字 `#FF3B30`。

### 7.6 输入框

12px 圆角、`1.5px solid #E5E5EA`、`#FAFAFA` 底、15px、placeholder `#C7C7CC`；focus：边框 `#007AFF` + 3px `rgba(0,122,255,0.12)` 光晕；`box-sizing: border-box`。

### 7.7 空状态 / 错误 / 加载骨架

- **空状态**：卡片容器、居中、`padding: 56px 20px`、54px 圆角图标底（`#F2F2F7`）。
- **错误**：浅红底 `#FDF2F2` + 红字 + 重试按钮，圆角 16px。
- **骨架屏**：灰渐变 shimmer（`#F2F2F7 → #FAFAFA`，1.3s），列宽与表头一致。

### 7.8 分页

- 左：`x-y / total`（12px / `#8E8E93`，等宽数字）；右：翻页按钮 32×32、8px、白底、`1px solid #E5E5EA`。

---

## 八、动效

- hover 过渡：**0.15s**。
- 面板展开/收起、入场动画：0.2–0.3s，使用 `opacity + translateY`（避免 scale 引起布局抖动）。
- 按钮按压反馈：`transform: scale(0.985)`（0.08s）。
- 操作按钮浮现：`opacity` 0.15s。
- 所有动效在 `prefers-reduced-motion: reduce` 时归零。

---

## 九、图标

- 使用 [Lucide](https://lucide.dev)（`lucide-vue-next`）。
- **stroke 描边风格**（非填充），`stroke-width` 按尺寸在 1.5–2.5 之间调整。
- 默认色 `#8E8E93`，hover `#1C1C1E`；危险语义 hover `#FF3B30`。
- 图标仅辅助理解，不单独传达关键信息。

---

## 十、响应式策略

| 断点 | 调整内容 |
|------|----------|
| < 900px | 统计卡 2 列；搜索框全宽；表格 `min-width: 860px` 横向滚动；内容边距 16px |
| < 560px | 标题栏纵向堆叠、主按钮左对齐；表单单列 |

断点间只调整间距与栅格列数，不改变组件语义。

---

## 十一、深色模式

**目标：跟随系统**（`prefers-color-scheme`），不提供页面内手动开关。

- 现状：**演进中**。v2 浅色 token 的深色对应值待全局落地，方向沿用 v1：
  - 背景 `#0d1218`、卡片 `#151b23`、边框 `#262e38`、文字 `#e8ecf1`、次要 `#aab3bd`、辅助 `#7d8791`
  - 深色下阴影降为 `rgba(0,0,0,0.5)` 量级，靠毛玻璃边缘高光（`inset 0 1px 0 rgba(255,255,255,0.08)`）表达层级
- 沉浸场景（登录页）深色毛玻璃已实现，作为特例保留。

---

## 十二、色彩对比度（WCAG AA）

| 用途 | 色值 | 背景 | 对比度 |
|------|------|------|--------|
| 正文/标题 | `#1C1C1E` | `#FFFFFF` | ≈16:1 ✅ |
| 次要文字 | `#3A3A3C` | `#FFFFFF` | ≈11:1 ✅ |
| 辅助文字 | `#8E8E93` | `#FFFFFF` | ≈3.3:1 ⚠️ 仅限 12px 以下说明文字与图标 |
| 占位符 | `#C7C7CC` | `#FFFFFF` | ≈2:1（占位符豁免） |
| 链接/选中/focus | `#007AFF` | `#FFFFFF` | ≈4:1 ✅（UI 组件 ≥3:1） |
| 角色标签 | `#7C3AED` | `#F3E8FF` | ≈4.8:1 ✅ |
| 本人标签 | `#007AFF` | `#E5F0FF` | ≈3.5:1 ✅（UI 组件） |
| 状态圆点 | `#34C759` | `#FFFFFF` | ≈2.2:1（装饰性非文字指示；语义文字用 `#1C1C1E` 保证可读） |

**铁律：正文不得使用 `#8E8E93` 及更浅色；状态永远由「圆点 + 深色文字」成对表达，不单靠颜色。**

---

## 十三、设计令牌（Design Tokens）

所有样式值收敛为语义化 CSS 变量，组件 scoped style 只引用变量、不再硬编码色值。

```css
:root {
  /* 背景 */
  --bg-page: #F2F2F7;
  --bg-card: #FFFFFF;
  --bg-table-header: #FAFAFA;
  --bg-input: #FAFAFA;
  --bg-hover: #FAFAFA;
  --bg-pill-inactive: #F2F2F7;
  --bg-tag-blue: #E5F0FF;
  --bg-tag-purple: #F3E8FF;

  /* 文字 */
  --text-primary: #1C1C1E;
  --text-secondary: #3A3A3C;
  --text-tertiary: #8E8E93;
  --text-muted: #C7C7CC;

  /* 边框 */
  --border-light: #F2F2F7;
  --border-input: #E5E5EA;
  --border-card: rgba(0,0,0,0.03);

  /* 语义色 */
  --accent-blue: #007AFF;
  --accent-green: #34C759;
  --accent-purple: #7C3AED;
  --accent-red: #FF3B30;
  --accent-yellow: #FFCC00;

  /* 按钮 */
  --btn-primary-bg: #1C1C1E;

  /* 阴影 */
  --shadow-card: 0 1px 3px rgba(0,0,0,0.04);
  --shadow-elevated: 0 4px 20px rgba(0,0,0,0.04);
  --shadow-button: 0 2px 8px rgba(0,0,0,0.12);

  /* 字体 */
  --font-sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  --font-mono: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;

  /* 圆角 */
  --r-sm: 8px;
  --r-md: 10px;
  --r-lg: 12px;
  --r-xl: 16px;
  --r-full: 999px;
}
```

**命名规则：** 按用途前缀分组（`--bg-*` / `--text-*` / `--border-*` / `--accent-*` / `--shadow-*` / `--r-*`）；组件内派生色用 `color-mix()` 基于令牌派生，不新造色值。

---

## 十四、设计范式对比（为什么是 iOS + Notion）

系统内保留一个「表单范式对比」参考页（`/users-form-demo`），两个范式对照如下；**本系统一律采用右侧（现代）范式**，左侧仅作对照参考。

| 维度 | 传统后台表单（不采用） | iOS + Notion 现代范式（本系统标准） |
|------|----------------------|-------------------------------------|
| 密度 | 字段密集（间距 16px） | 大量留白（核心字段间距 24–32px） |
| 圆角 | 输入框 6px（工具感） | 输入框 12px（亲和力） |
| 按钮 | 6px 直角感圆角 | 999px 药丸形 |
| 必填标记 | 红色星号 `*` | 隐藏必填标记，靠提交校验 |
| 选择器 | 原生 `<select>` | 横向 Pill 胶囊单选 |
| 阴影 | 几乎无（扁平） | 弥散阴影（层级感） |
| 字段数 | 一次展示全部（10 字段） | 3 个核心字段 + 高级设置折叠区 |

---

## 十五、与现有代码的关系

**规范 = 描述现状 + 指引方向，落地状态：**

- ✅ **已对齐（v2 风格）**：
  - 用户管理页 `/users`（毛玻璃标题栏、统计卡、搜索筛选卡、Grid 表格、hover 浮现操作）
  - 表单范式对比页 `/users-form-demo`（readonly 设计参考页）
- 🔄 **待迁移（v1 风格）**：设备管理 `/devices`、组织架构 `/identity`、测试控制台 `/` —— 沿用旧深色表单/旧字体，需按 v2 逐步对齐
- ⏳ **待实施**：全局 `main.css` 提取为 v2 令牌（第十三节）；深色模式令牌落地；上述页面迁移

**铁律：**

1. 后续新增/修改任何页面，必须先查本文档。
2. 新页面必须使用 v2 系统风格：iOS 色板 + 系统字体栈 + 卡片化布局 + 药丸/大圆角 + 弥散阴影 + 负字距微调。
3. 产生任何新样式值，必须同步回本文档（规范是活的）。
