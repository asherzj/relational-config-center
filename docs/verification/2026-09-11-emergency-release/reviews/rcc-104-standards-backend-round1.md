# #104 Standards 后端初审

范围：工作树 `/private/tmp/rcc-issue-104-emergency-release`，基点 `e2564842e72e96b6ffb1e86708978b11dd8a651b`；未提交，commit 列表为空。审查 `git diff <base> -- admin` 及未跟踪验收文件，固定输入为六个生产文件和一个集成测试，并按调用路径读取不变依赖。不读 Spec 结论、不运行测试、不改源码。

**成文规范：0 发现。** 切换使用领域 ReleaseType；保存方式、全部实例、主单版本及原结果在既有事务内完成。应急提交记录真实原因与 SUBMIT，进入 PENDING_PUBLICATION，不制造审批。发布继续复用整单深执行端口、锁内当前权限和原请求授权；取消/重新准备覆盖新状态。符合 ADR-0012/0013 的分层与事务边界、ADR-0011 的安全稳定错误、ADR-0018 的永久身份，以及 ADR-0025/0026/0027 的整单原子性、当前授权和实例事实原则。

**主观发现：1 项，P3（非阻塞，可能是 Divergent Change）。** `admin/internal/application/release_orders.go:655–657` 将提交专属 `EmergencyReason` 加入了多个操作共享的输入；`admin/internal/interfaces/http/release_orders.go:128,157,169` 也用它解析回滚预览、完结和发布。调用这些操作时，应急原因被接受但不记录；发布/完结还将未使用字段计入原请求摘要。建议提交使用含原因的专用输入，其余动作使用仅版本输入，使应急提交的变化不扩大无关契约。这是职责耦合判断，不是成文违规或新的产品验收要求。

完整主观基线：Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest。仓库优先、工具已强制项跳过，其余无可行动发现。

证据时点：读取 01–11 原日志结果，七个新顶层用例已有初步 PASS，原红保留；11 消费者补验实际 PASS 8.70s。最终证据输入对应、受影响回归、Web/真实浏览器及领域/ADR/API 文档仍待稳定后增量核验；本报告不宣布整票放行。

快照：七个固定输入全 MATCH；输入清单 SHA256 `b7a8655da5b54af3c4b946ae4b0b414e235713a4d16cf609bb15cb56bc6eaa44`。独立源码和依赖哈希保存在 `/tmp/rcc-104-standards-backend-source-round1.json`。后续仅复核变动文件。
