# #104 Spec Web 增量复核（round2）

结论：**初次 P2 已修复，本次增量未发现新增规格问题。** 原发现留在 `/tmp/rcc-104-spec-web-round1.md`，没有覆写原始红例。

只读核对当前 `ReleaseRequestReview.tsx` 28～94 行及 `ReleaseOrdersPage.test.tsx` 对应新增场景。每次 inspect 先撤销 submitReviewed 并清空 rebuilt；仅完整最新读取、允许 submit、preview 成功且记录基线未变后取得资格。textarea 与建包均受资格保护，第二次 GET 失败不能使用旧 current 绕过，原原因与本窗口补充保留。重建随最新 STANDARD/EMERGENCY 类型，常规不带应急原因；应急原因非空且按 Unicode 码点限制 2,000 字。成功提示仅称已提交，未知结果保留新原包且无成功提示。

实际证据：40 为 4 FAIL / 2 PASS，41 为 6 PASS；42 的发布页、发布 API、共享 ManagedDataPage 共 108 PASS。43 类型检查、44 构建 exit 0。41～44 的源码哈希与审查时工作树完全匹配。未重跑测试或修改源码，未读取 Standards 报告。

03 浏览器验证输入与当前仅此恢复组件及其测试不同；03 不进入被明确拒绝后的重建路径，原包恢复证据仍适用，新增分支由 41/42 覆盖。本结论不替代 #98/#104 接入后的最终集成门禁。

实际输入清单：`/tmp/rcc-104-spec-final.inputhash.json`，SHA-256：`e4686e7bda4f02828e6e6506c68560067333a48cd7a0d58f2aceec68d94b7c21`。
