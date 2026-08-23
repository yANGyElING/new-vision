# 设计文档索引

> 设计文档是**目标架构**的描述，不等同于当前实现。
> 当前实现状态见 [`../knowledge-base.md`](../knowledge-base.md)。

---

## 文档列表

| 文档 | 主题 | 状态 | 最后更新 |
|---|---|---|---|
| [`federated-video-platform-architecture.md`](federated-video-platform-architecture.md) | 架构基线：中心-自治节点联邦式架构（节点自治、中心纳管、多租户、SIP 接入） | ✅ 已确认基线 | 2026-08-18 |
| [`identity-scope-refactor.md`](identity-scope-refactor.md) | 身份与权限模型重构：区域(Region)→组织单元(Org Unit)，去 Casbin | 🔄 设计提案（0.x） | 2026-08-19 |
| [`design-spec.md`](design-spec.md) | 前端设计规范（v2 iOS + Notion 系统风格） | ✅ 已确认基线 | — |

---

## 按主题查找

| 想找什么 | 看哪个文档 |
|---|---|
| 系统整体架构、中心/节点拓扑、多租户约束 | `federated-video-platform-architecture.md` |
| 权限模型、用户/组织/设备范围、角色矩阵 | `identity-scope-refactor.md` |
| 前端页面风格、颜色 token、组件规范 | `design-spec.md` |
| 当前已实现的代码行为 | `../knowledge-base.md` |

---

## 状态说明

- ✅ **已确认基线**：与代码一致或已由用户确认的现行设计
- 🔄 **设计提案**：尚未实现，或实现中；实现后应更新状态
- ⏳ **待确认**：设计未定，开放问题未拍板