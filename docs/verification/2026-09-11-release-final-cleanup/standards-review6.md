# Standards 增量复审（candidate 6）

固定差异：`f0228e09a08ca55d0c85a946bb25aa80a7974ea3..8b835f78aea0ad79df7d0ee5e7b4c1323e875dd4`，仅 `web/e2e/release-multitable.cjs`。`source-candidate6.json` 实测 SHA-256 为 `996f86f5a71fc5e3631bba955e706442e06ac4b852a46c84ec45eea321a84e20`。

## 成文规范违规

无（0）。

新增路径从真实管理员字段配置页选择 `textarea`，等待目标表字段策略 PUT 200，再以公开 GET 核对 `effective.ui_type`；编辑者页面同时断言控件 DOM 为 `TEXTAREA`。这符合 `web/DESIGN.md` 关于多行录入控件、真实字段配置与浏览器验收的约定。大值校验保留完整输入与后端持久值的逐值相等、请求正文大于 9 MiB、同 key/正文恢复及最终重读全文断言，没有用摘要替代业务正确性。新增诊断只写字符数、字节数和 SHA-256，不写 9 MiB 原文；结果日志仍只输出紧凑 evidence。

同步未见不可靠放宽：字段保存响应等待在点击前注册，并限定准确路径、PUT 与 200；随后 API 有效规则和真实 DOM 两层检查，任一失败都会中止。原路由模拟丢响应、角色/账号隔离和恢复断言保持不变。

## 主观坏味道

无（0）。局部 `valueEvidence` 统一五处摘要生成，减少重复；`largeDiagnostic` 名称和字段说明清楚。未发现 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest。

## 证据边界

本次为静态只读复审；未运行测试、数据库、浏览器或 Compose，也不据此宣称 all34 或 Compose 已完成。运行证据由实现代理负责。
