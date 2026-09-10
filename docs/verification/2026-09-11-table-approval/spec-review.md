# Spec review

独立只读代理 `/root/implement_94/spec_review`，与 Standards 并行。读取工单 #94/#92 原始正文/评论/标签和仓库规格，以固定基点 `2baec220f4fc0a54e722f3dc7ed9711853079c04` 的完整待提交差异为范围。

Spec：0 项未解决发现，1 项发现已修复。

AC-009 要求“新提交拒绝并指出问题表，草稿保留”。原实现把明确的 422 当成未知结果，并保留旧审批安排；现由 useReleaseWrite 清除经原键去重后明确未提交的请求，由 ReleaseActionDialog 展示当前提交安排。初次拒绝、未知后原键拒绝两项回归通过，类型检查通过；批准/拒绝的原确认范围仍冻结。原 `web/unavailable-classification-red.txt` 保留两项真实红灯；第一次修复运行 `unavailable-classification-green.txt` 实际仍因“关闭”定位存在两个按钮失败，不记为通过；后续 `review-regressions.txt` 四文件131项通过，最终 `unavailable-classification-final.txt` 两项通过。测试误用 ByRoleOptions.exact 的类型错误保留在 `review-typecheck.txt`，删去多余选项后的 `review-typecheck-final.txt` 通过，未改变定位的默认精确语义。

AC-003～011 的角色永久引用、完整表范围、实时资格、动态 ADMIN、自审禁止、并发顺序和合法历史保留，与源码及 MySQL/Web/七组浏览器证据相符。未发现实际 APPROVER 授权旁路、未要求的功能扩张或超期兼容结构。旧账号值迁移与通知明确属于后续工单。

初评时旧11脚本回归尚在运行；评审没有将未执行结果记作通过，最终调用方结果由实现代理归并后再复核。最终增量已完成，结论见下。

最终 Spec 通过：原1项发现已关闭，0未解决。修复后7组浏览器全部通过；回滚竞争以同一publisher在拒绝前后严格比较完整响应，符合查看者相关revision契约，配置值/版本/历史/终态断言均保留。11个旧脚本由首轮6个与最终5个完整覆盖，后者109.968s；含失败日志仍按原结果保留。未发现新的规格缺失、关键证据缺口、范围扩张或未清理的临时兼容。
