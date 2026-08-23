# new-vision 文档 Wiki

> 文档分两条线：**设计文档**（目标架构，在 `design/` 下）和 **当前状态知识库**（`knowledge-base.md`，从代码生成）。
> 需要找设计决策 → 从 `design/README.md` 入口；需要看当前实现 → 读 `knowledge-base.md`。

---

## 目录

| 路径 | 内容 | 维护者 |
|---|---|---|
| [`design/`](design/README.md) | 设计文档索引（目标架构、重构提案、规范） | 人工 |
| [`design/design-spec.md`](design/design-spec.md) | 前端设计规范（v2 iOS + Notion 系统风格） | 前端 |
| [`design/federated-video-platform-architecture.md`](design/federated-video-platform-architecture.md) | 架构基线：中心-自治节点联邦式架构 | 架构 |
| [`design/identity-scope-refactor.md`](design/identity-scope-refactor.md) | 身份与权限模型重构：区域→组织单元 | 架构 |
| [`knowledge-base.md`](knowledge-base.md) | 当前实现状态（从 working tree 生成，含已完成的业务逻辑与工程状态） | 工程 |

---

## 文档间关系

```
架构基线 (federated-video-platform-architecture.md)
├── 定义了：中心-自治节点、多租户、节点本地完整平台
├── 约束：区域/地市/行业只是管理属性，不是固定层级
└── 被引用：identity-scope-refactor.md 以此为设计依据

身份与权限模型重构 (identity-scope-refactor.md)
├── 范围：自治节点本地身份与权限模型
├── 状态：设计提案（0.x，待确认后实现）
└── 影响：knowledge-base.md 中的身份/权限部分需在实现后更新

前端设计规范 (design-spec.md)
├── 范围：所有 Vue 页面的视觉规范
└── 与身份/权限模型无关，但设备/用户/组织页面需遵循其 token 系统

当前状态知识库 (knowledge-base.md)
├── 描述：已实现的业务逻辑和工程状态
└── 注意：identity-scope-refactor 实现后，§6 数据模型、§5 API 表面等需同步更新
```

---

## 文档约定

- 设计文档优先使用 Markdown，代码示例用 fenced code block
- 决策记录（D1, D2, ...）放在设计文档末尾，作为可追溯的决策日志
- 开放问题（Q1, Q2, ...）标注"待确认"，确认后收入决策记录
- 跨文档引用使用相对路径（如 `[架构基线](../design/federated-video-platform-architecture.md)`）