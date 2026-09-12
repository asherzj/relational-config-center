# PR109 final browser repair — Spec review

**结论：本地提交 gate 可通过；Findings 0。**

## Findings（a–e）

未发现需求缺失、验收证据缺口、越界、兼容/退出条件遗漏或可疑实现。

- **需求符合性：** #106 要求“真实浏览器完整桌面/390px/键盘及会话路径”。修复使用基线已支持的 `table_name` 深链，并在 selector、精确 drawer、请求正文、发布单表集合和申请人五处锁定 fixture（`browser-accessibility.cjs:90-97,212,321,326-327,414-415`）。390px 恢复仍执行真实 reload、手机导航与离页确认；随后现有 NewDraft 流程按持久 journal 重试。
- **关键语义：** 保留两次 POST 数量及完整 body/key 相等断言、独立 reviewer 审批、真实发布、SQL 恰一行、禁止旧直写路由和最终 `pageErrors=[]`（`:398-432`）。未接受 403、未扩权、未加错误白名单或 timeout。
- **范围/兼容：** `source.diff` 仅含该 E2E 文件六个 hunk；从固定 base 可精确重建冻结源码。产品代码、共享 helper、权限和设计规则均未变；删除的 helper import 在最终文件无引用，helper 本身无副作用。没有临时生产兼容结构需要退出。

## 证据与映射

manifest SHA-256 与给定值一致；92 个声明文件全部存在且逐项哈希匹配，内部 `SHA256SUMS` 也全部通过。冻结源码 SHA-256 为 `d638254d…e89418c`，与 final evidence 的 source binding 一致。保留证据证明旧路径实际创建错表后被正确 403；深链单修复仍因 page error 失败，未伪装为绿。最终共享数据库序列中 Chromium、Firefox、Linux WebKit 各 7 项通过，均 `failure=null`、`pageErrors=[]`、两次原请求相同、SQL=1、cleanup=0；Web build 与 timeout-cleanup test 通过。

## Unresolved / 必需待补证

**Post-push gate：** 尚须由正式 CI 在 Linux amd64 跑完整 34 个 browser cases。现有 WebKit 是 Linux arm64 客户端连接 macOS arm64 后端，Chromium/Firefox 也在 macOS；这不否定本地提交 gate，但不能宣称正式整套 CI 已通过。
