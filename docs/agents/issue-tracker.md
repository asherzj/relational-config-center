# Issue 跟踪器：GitHub

本仓库的 Issue 和规格说明都存放在 GitHub Issues 中。所有操作均使用 `gh` CLI。

## 约定

- 在仓库目录运行 `gh`，由 Git remote 自动定位仓库。
- 读取或检索工单时，同时获取正文、评论和标签；按需过滤状态与标签。
- 多行正文写入临时文件并使用 `--body-file`；Triage 标签见 [标签约定](triage-labels.md)。

## 将 Pull Request 作为 Triage 入口

**将 PR 作为请求入口：否。**

GitHub 的 Issue 和 PR 共用同一个编号空间，因此单独出现的 `#42` 可能是其中任意一种：先运行 `gh pr view 42`，失败后再运行 `gh issue view 42`。

## Wayfinding 操作

供 `/wayfinder` 使用。**地图（map）**是一个单独的 Issue，**子项（child）**是作为工单的子 Issue。

- **地图**：一个带有 `wayfinder:map` 标签的 Issue，其正文包含 Notes、Decisions-so-far 和 Fog。使用 `gh issue create --label wayfinder:map` 创建。
- **子工单**：通过 GitHub 子 Issue 关系链接到地图的 Issue（使用子 Issue API 的 `gh api`）。如果未启用子 Issue，则将子项添加到地图正文的任务列表中，并在子 Issue 正文顶部写入 `Part of #<map>`。标签为 `wayfinder:<type>`，其中类型为 `research`、`prototype`、`grilling` 或 `task`。领取后，将工单指派给负责推进的开发者。
- **阻塞关系**：以 GitHub 原生 **Issue 依赖关系**作为规范且在 UI 中可见的表示。使用 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 添加依赖边，其中 `<blocker-db-id>` 是阻塞项的数字型**数据库 ID**（通过 `gh api repos/<owner>/<repo>/issues/<n> --jq .id` 获取；不是 `#number` 或 `node_id`）。GitHub 通过 `issue_dependencies_summary.blocked_by` 报告尚未关闭的阻塞项，这是实时门禁条件。如果依赖关系不可用，则在子 Issue 正文顶部使用 `Blocked by: #<n>, #<n>` 作为降级方案。当所有阻塞项都已关闭时，工单解除阻塞。
- **前沿查询**：列出地图下所有未关闭的子项（使用 `gh issue list --state open`，范围限定为地图的子 Issue 或任务列表），排除仍有未关闭阻塞项（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行中存在未关闭的 Issue）以及已有负责人指派的项目；按地图中的顺序选择第一个。
- **领取**：运行 `gh issue edit <n> --add-assignee @me`；这是当前会话的第一次写操作。
- **解决**：先运行 `gh issue comment <n> --body "<answer>"`，再运行 `gh issue close <n>`，最后把上下文指针（gist 与链接）追加到地图的 Decisions-so-far 中。
