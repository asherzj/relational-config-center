# Issue 跟踪器：GitHub

本仓库的 Issue 和规格说明都存放在 GitHub Issues 中。所有操作均使用 `gh` CLI。

## 约定

- **创建 Issue**：`gh issue create --title "..." --body "..."`。多行正文使用 here-document（heredoc）。
- **读取 Issue**：`gh issue view <number> --comments`，使用 `jq` 过滤评论，并同时获取标签。
- **列出 Issue**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，并根据需要使用 `--label` 和 `--state` 过滤器。
- **评论 Issue**：`gh issue comment <number> --body "..."`
- **添加／移除标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭 Issue**：`gh issue close <number> --comment "..."`

通过 `git remote -v` 推断仓库；在克隆仓库内部运行时，`gh` 会自动完成此操作。

## 将 Pull Request 作为 Triage 入口

**将 PR 作为请求入口：否。** _（如果本仓库将外部 PR 视为功能请求，可改为 `yes`；`/triage` 会读取此标志。）_

设为 `yes` 后，PR 将使用与 Issue 相同的标签和状态流转，并使用对应的 `gh pr` 命令：

- **读取 PR**：使用 `gh pr view <number> --comments`，并通过 `gh pr diff <number>` 查看差异。
- **列出待 Triage 的外部 PR**：运行 `gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，然后只保留 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR` 或 `NONE` 的项目（排除 `OWNER`、`MEMBER` 和 `COLLABORATOR`）。
- **评论／添加标签／关闭**：使用 `gh pr comment`、`gh pr edit --add-label` / `--remove-label`、`gh pr close`。

GitHub 的 Issue 和 PR 共用同一个编号空间，因此单独出现的 `#42` 可能是其中任意一种：先运行 `gh pr view 42`，失败后再运行 `gh issue view 42`。

## 当技能要求“发布到 Issue 跟踪器”时

创建一个 GitHub Issue。

## 当技能要求“获取相关工单”时

运行 `gh issue view <number> --comments`。

## Wayfinding 操作

供 `/wayfinder` 使用。**地图（map）**是一个单独的 Issue，**子项（child）**是作为工单的子 Issue。

- **地图**：一个带有 `wayfinder:map` 标签的 Issue，其正文包含 Notes、Decisions-so-far 和 Fog。使用 `gh issue create --label wayfinder:map` 创建。
- **子工单**：通过 GitHub 子 Issue 关系链接到地图的 Issue（使用子 Issue API 的 `gh api`）。如果未启用子 Issue，则将子项添加到地图正文的任务列表中，并在子 Issue 正文顶部写入 `Part of #<map>`。标签为 `wayfinder:<type>`，其中类型为 `research`、`prototype`、`grilling` 或 `task`。领取后，将工单指派给负责推进的开发者。
- **阻塞关系**：以 GitHub 原生 **Issue 依赖关系**作为规范且在 UI 中可见的表示。使用 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 添加依赖边，其中 `<blocker-db-id>` 是阻塞项的数字型**数据库 ID**（通过 `gh api repos/<owner>/<repo>/issues/<n> --jq .id` 获取；不是 `#number` 或 `node_id`）。GitHub 通过 `issue_dependencies_summary.blocked_by` 报告尚未关闭的阻塞项，这是实时门禁条件。如果依赖关系不可用，则在子 Issue 正文顶部使用 `Blocked by: #<n>, #<n>` 作为降级方案。当所有阻塞项都已关闭时，工单解除阻塞。
- **前沿查询**：列出地图下所有未关闭的子项（使用 `gh issue list --state open`，范围限定为地图的子 Issue 或任务列表），排除仍有未关闭阻塞项（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行中存在未关闭的 Issue）以及已有负责人指派的项目；按地图中的顺序选择第一个。
- **领取**：运行 `gh issue edit <n> --add-assignee @me`；这是当前会话的第一次写操作。
- **解决**：先运行 `gh issue comment <n> --body "<answer>"`，再运行 `gh issue close <n>`，最后把上下文指针（gist 与链接）追加到地图的 Decisions-so-far 中。
