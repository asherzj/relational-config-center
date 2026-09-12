# 模板与审批依赖合并：Spec 初轮

2026-09-11；只读检查，未运行测试、未读取 Standards。工作树 `/private/tmp/rcc-release-template-approval-integration`；ours `7eb9dd36affe16739d8e8b178afdd9977d4976a3`、theirs `2612bb354ef71d42e193648564b9f30bd9281eb4`，文本冲突已解、merge 未提交。范围采用 `/tmp/rcc-template-approval-integration-review-scope.json` 的 23+7 路径，并核对关键实现与父提交字节相等。

**当前源码 0 发现；验收证据尚未闭合，暂不最终 Spec 放行。**

依据为 #100/#102 的 AC006–008（每表两类唯一关联、新建仅默认应急、历史补缺不覆盖、应急持续可用）及已捕获完整 #93/#94 正文评论（按表资格、实时成员与默认审批人、独立审批、原范围恢复）。本轮未发现规格缺失、额外业务行为或临时层退出遗漏；不要求尚未开始的 #103 实例和 #104 应急执行。

- `admin/cmd/admin/application.go:41–44`、`admin/internal/interfaces/http/router.go:31–39,103–172` 同时保留角色、模板、关联入口及表规则原请求键/版本。审批门禁 `security.go:214–215` 保持 VIEWER，实际逐表授权和事务实现等同外部父提交；T2 表规则请求、关联事务实现等同 ours，保留当前权限、原结果与版本保护。
- `web/src/features/table-policies/TablePoliciesPage.tsx:69–75` 同时保留字段、模板、审批入口；两个 T2 编辑器及外部审批恢复代码等同各自已交付父提交。冲突解决没有引入自动写重试或重建原请求。
- 正式 1–7 共 14 个 SQL/manifest 与 theirs 逐字节一致。候选 8/9 累计 27/28 表，均含六张审批控制表。`template_approval_schema_integration_test.go:9–35,37–59` 证明版本 7 旧事实保留、新装等价、SELECT-only 启动/持续 ready 的六表缺失拒绝与恢复；历史接管仍固定 5。

实际证据：schema 03 全部 14 顶层 PASS，197.021s；旧错误 ready=200 的红例保留。Web 有效 188 项、TS/build 通过，后端三个单元包通过。

**唯一待核验清单**：合并后 19 项真实 HTTP/MySQL、桌面/390px/键盘与原请求恢复的真实浏览器，以及最终源码—证据快照。当前 `backend/mysql-selected-result.json` 为 exit1、0 PASS；19 项均 Docker provider unavailable，属未带 Colima `DOCKER_HOST` 的环境失败，不能计为业务通过或业务缺陷。父代理已确认在相同源码下重跑；README 的“尚未运行”应随结果更新。Schema 完成后已捕获 224 个 Admin 输入，最终应核对其与交付源码一致。复用父提交证据不能替代这些合并路径。

当前 SHA-256：

```text
router.go a8e834eff83e3adeb5f0c7f1e3c7a470f8244d1e1c7ef3978da67476b1014ee6
security.go f74193f72487eacaf73e90e5cbac624f117ce2f94f2aacd71c457a1aeb7f5705
TablePoliciesPage.tsx f778f0b551eab6249dd1441a6706795bc96d74092fb80a1c482b1de31e8f32f1
template_approval_schema_integration_test.go 53e95471fd335d868aa712bd099b4e3c991e214e6b0c186064b8f0a4346194b2
00008_schema.json 8571dab0e2e548ccab60d5e931334a3a05b0910ba151dd9a75e136ae349056b0
00009_schema.json dc6c725fa5341a375670c7ddedad3a83325cf96823d55034acf128e14f39e80c
```

证据目录：`docs/verification/2026-09-11-template-approval-integration/`。后续仅需针对新改动和上述证据增量收口。
