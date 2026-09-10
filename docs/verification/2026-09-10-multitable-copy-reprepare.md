# #85 多表复制与重新准备验收

固定实现基点：`9fd38ddda5de05d341491bf93752159e9631cc9f`。本票对应父规格 #81 的 AC-008，只在隔离工作树实现。

## 行为证据

| 场景 | 真实 HTTP + MySQL 证据 |
| --- | --- |
| 派生明细身份 | `TestMultitableDerivedDraftRequiresExactDetailIdentity` 用完整合法正文只替换第二项 `detail_id`，返回定位到该项的 `422 release_invalid`；替换 `table_name` 保留稳定的 `release_cross_table`；旧调用省略公开身份时从源单恢复两表及稳定 `detail_id`。 |
| 复制当前多表基线 | `TestMultitableCopyCompetesForEveryTargetAndLinksBothOrders` 在第二表外部漂移后重新预览；晚表目标冲突返回全局位置、实际表、占用单和申请人，且无第一表占用、半张新单或幂等结果。解除冲突后新草稿保留标题、顺序、操作、内容、两表和稳定明细身份，使用漂移后的基线；同键同正文重放返回同一结果。源终态/原内容保留，源与新单互相保存 `COPY` 关联。 |
| 重新准备原子转移 | `TestMultitableReprepareTransfersChangedTargetsAtomically` 从两表已批准单读取漂移后的基线。末端 table-reference 写入被真实 MySQL trigger 拒绝时返回 503，源单 JSON、审批、两项明细、6 个目标、2 个表引用和幂等记录均保持原状。成功后源单取消，新草稿属于实际 ADMIN 操作者，保留标题、顺序、操作、内容及明细身份，旧目标/引用全部转移到新单；同键重放不重复创建。 |
| 无释放窗口与新审批 | 同一用例以 `GET_LOCK` 将事务停在保留目标的源 DELETE 后，并等待 `performance_schema.data_lock_waits` 确认第三单真实阻塞在 `rcc_release_targets` 主键，再放行。重新准备提交 201 后第三单返回 409 且占用者为新草稿。新申请人不能自批，独立审批人可重新批准；旧审批没有继承。 |

最后受影响后端集成运行 `/private/tmp/rcc-issue-85-backend-final.log`：以下 **8/8 PASS**，共 66.421 秒：

- `TestReleaseRejectedCopyRechecksBaseline`
- `TestReleaseBatchCopyInvalidItemIsLocated`
- `TestMultitableDerivedDraftRequiresExactDetailIdentity`
- `TestMultitableCopyCompetesForEveryTargetAndLinksBothOrders`
- `TestMultitableReprepareTransfersChangedTargetsAtomically`
- `TestReleaseReprepareReplacesApprovedOrderWithEditableDraft`
- `TestReleaseReprepareRequiresCurrentApplicantEditorOrAdmin`
- `TestReleaseReprepareFailureKeepsApprovedOrderAndTarget`

该范围包括本票三条新增边界、已有复制基线/批量错误定位，以及重新准备成功、权限和失败保护回归。#84 已验证且本票未修改的发布/回滚执行面复用 [284 项真实集成清单](2026-09-10-multitable-integration-manifest.json)，没有为本票重复全仓数据库套件。

## 浏览器

最终日志 `/private/tmp/rcc-issue-85-browser-final-v2.log`：`TestAccountBrowserSystemPath/release-multitable.cjs` 与父测试均 PASS。真实 Chrome 经 Vite 同源代理、Admin 和单个隔离 MySQL 完成 7 项检查；本票新增两项证明：

- 复制抽屉读取两表当前基线，UI 确认在晚表冲突时于同一可视区显示第二项、实际表、占用单链接和申请人，同时保留两表原申请值；解除冲突后由同账号 API 创建成功，再通过真实 UI 核对源/新单双向跳转。完整 UI 重推链路属于 #86。
- 重新准备经二次破坏性确认生成实际操作者的新草稿，源/新单双向跳转；新单重新提交后由另一个真实账号批准。

已人工查看 [复制晚表冲突与保留输入](2026-09-10-multitable-copy-reprepare/multitable-copy-conflict.png) 和 [重新准备后的独立审批与源单关联](2026-09-10-multitable-copy-reprepare/multitable-reprepare-approved.png)。前者在 1440×1000 真实抽屉内同时呈现两表 current/proposal、冲突表、单号与申请人；后者呈现两表新单、实际申请人、独立审批人和来源关联。

浏览器失败来源均保留且未标绿：最初 `/private/tmp/rcc-issue-85-browser.log` 是隔离工作树缺少锁定依赖的 setup 失败；`/private/tmp/rcc-issue-85-browser-v2.log` 暴露夹具复用了已发布/回滚记录的旧版本；`/private/tmp/rcc-issue-85-browser-v3.log` 暴露 locator 命中抽屉后的同名页面明细；`/private/tmp/rcc-issue-85-browser-final.log` 是新证据父目录未建立的 setup 失败。依赖按 frozen lock 安装、夹具改用明确当前版本、locator 限定到 dialog，并预建父目录后得到上述最终 PASS。

## 普通检查与评审

- Admin 普通 `go test ./...` 全部通过：`/private/tmp/rcc-issue-85-go-unit-v2.log`；`go build ./...` 通过：`/private/tmp/rcc-issue-85-go-build.log`。
- Web 受影响 `ReleaseOrdersPage.test.tsx` **54/54 PASS**：`/private/tmp/rcc-issue-85-web-affected-v2.log`；`pnpm build` 的类型检查与生产构建通过：`/private/tmp/rcc-issue-85-web-build.log`。本票未修改 Web 生产源码，其他 Web 行为复用 #84 的 **333/333** 完整套件；真实浏览器已覆盖本票新增脚本。
- `node --check web/e2e/release-multitable.cjs`、`gofmt`、`git diff --check` 通过。

Standards 与 Spec 两个独立只读子代理审查固定基点以来的完整 tracked/untracked diff。Standards 初审发现表身份错误码、复制源历史契约文档和测试 helper 重复，均修正后复核为 0 剩余发现。Spec 初审发现 range 副本未把恢复的身份写回实际 prepare 输入、完整意图断言不足，均修正并增加成功/失败证据；复核 AC-008 为 0 剩余发现。两轴 residual risk 仅为并发夹具依赖目标 MySQL 8.4 的 `performance_schema` 可见性，已由本机真实 MySQL 通过结果覆盖。

## TDD 与资源

准确 RED `/private/tmp/rcc-issue-85-identity-red-v2.log` 证明完整有效请求只改合法 `detail_id` 时旧实现错误创建 201；`identity-green.log` 是最小身份校验转绿。更早的 `identity-red.log` 缺少已知记录版本，422 来自别的校验，明确不是身份保护证据。复制初轮 `/private/tmp/rcc-issue-85-copy-red.log` 在其他原子断言通过后只缺源侧关系；`copy-green.log` 在补真实双向关联后通过。重新准备最初 `/private/tmp/rcc-issue-85-reprepare-initial.log` 因应用账号无 binlog trigger 权限失败，改由同容器 root 测试连接注入末端故障；最终原子/并发证据以 `backend-final.log` 为准。

所有数据库与浏览器运行串行，任意时刻最多一个本任务 MySQL 容器；用例内部保留多连接和 goroutine 屏障。每次运行由 testcontainers 清理自身容器，未操作其他容器、开发数据或全局 Colima。
