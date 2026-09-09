# 多表发布单交付拆分（已发布）

父规格：[#81](https://github.com/asherzj/relational-config-center/issues/81)。建议串行按编号推进；硬依赖只表示真正阻塞项，编号不增加额外依赖。

| 工单 | 交付路径 | 阻塞于 | 主要验收 |
| --- | --- | --- | --- |
| [T1 / #82](https://github.com/asherzj/relational-config-center/issues/82) | 三表存储下的原单发布与回滚 | 无（拆分确认并发布后可开始）。 | AC-010 |
| [T2 / #83](https://github.com/asherzj/relational-config-center/issues/83) | 管控键配置、草稿占用与并发编辑 | T1 | AC-002, AC-003, AC-004, AC-005 |
| [T3 / #84](https://github.com/asherzj/relational-config-center/issues/84) | 多表有序明细、完整生命周期与千条整单执行 | T2（已包含T1基础）。 | AC-001, AC-006, AC-007, AC-009, AC-014 |
| [T4 / #85](https://github.com/asherzj/relational-config-center/issues/85) | 复制与重新准备的跨表占用流转 | T3 | AC-008 |
| [T5 / #86](https://github.com/asherzj/relational-config-center/issues/86) | 异常报错、安全重推与失败历史 | T3 | AC-011, AC-012 |
| [T6 / #87](https://github.com/asherzj/relational-config-center/issues/87) | 回滚原因的事后补填与修改留痕 | T1 | AC-013 |
| [T7 / #88](https://github.com/asherzj/relational-config-center/issues/88) | 移除过渡结构并验收升级路径 | T4、T5、T6（传递覆盖T1～T3）。 | AC-015 |

每个功能切片都包含对应存储/API/UI和验证。T1先验证单表完整路径，T3再完成多表扩展，不把T1当作完整功能交付。

临时结构责任：T1可能保留的单表外观由T3解除业务限制，T7删除余下外观及旧字段；T5替换旧确认流程，T7删除剩余过渡字段。没有旧数据兼容或双写；各工单能直接删除时无需人为引入过渡结构。T7负责最终零残留与升级验收。

用户已确认七张工单的粒度、依赖、唯一AC归属和T7收缩责任，并授权每张由独立subagent隔离实现，按复杂度选择模型及思考强度。七张工单与原生父子/阻塞关系已发布。

## 执行配置

- T1 / #82：gpt-6-astra，xhigh，独立分支及worktree。
- T2 / #83：gpt-6-astra，max，独立分支及worktree。
- T3 / #84：gpt-6-astra，max，独立分支及worktree。
- T4 / #85：gpt-5.6-sol，high，独立分支及worktree。
- T5 / #86：gpt-6-astra，xhigh，独立分支及worktree。
- T6 / #87：gpt-5.6-sol，high，独立分支及worktree。
- T7 / #88：gpt-6-astra，high，独立分支及worktree。

默认串行按可执行前沿推进，留出双轴评审所需槽位。各工单提交推送并关闭后，主协调者在本功能集成分支快进接入；不合并或推送main。
