# #104 Spec Web 初次发现留档

本文件保留修复前已即时发给 root 的发现；不是对当前修复后源码的结论。固定基点 `e2564842e72e96b6ffb1e86708978b11dd8a651b`，工作树 `/private/tmp/rcc-issue-104-emergency-release`。未读取 Standards 报告。

## [P2] 最新读取或预览失败后仍能绕过审阅重建应急提交

位置：`web/src/features/release-orders/ReleaseRequestReview.tsx` 修复前 84～87 行。提交被明确拒绝后，GET 成功而 preview 失败，或最新单据已不允许 submit，界面仍显示可编辑的应急原因；输入有效原因便直接生成新请求并启用确认。已成功读取后再次 GET 失败也可用旧 current 重新启用。确认先调用 confirmRebuild，从而在本轮完整状态/记录基线审阅未成功时替换保留的原请求，违背 AC-023、工单失败决策与 DESIGN 的显式恢复要求。服务端仍会检查状态和版本，不能据此宣称业务数据已遭错误发布。

应在每次审阅开始撤销提交资格；仅当当前 GET/完整明细、preview、最新 submit 资格及记录基线全部通过后允许建包，并保留原原因。

原源码 SHA-256：`ff93d5a9e9440de70790b16a9549c9fa69d8fe8d20b8da35d4928b43cdbbb3d6`，记录于 `web/40-conflict-review-complete-red.json`。39/40 原始红例日志保留，40 为 4 FAIL / 2 PASS。当前修复见独立 round2 报告。
