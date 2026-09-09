# T1 / #82 过渡结构与退出责任

以下只服务从单表路径逐步推进 #81；没有旧数据兼容、双写或独立回滚业务路径。

| 当前结构 | 当前约束 | 退出责任 |
| --- | --- | --- |
| 主单 `TableName` / `Frozen` / `FrozenDigest`，明细 `table_name` 必须与主单相同 | 现有单表审批快照；明细表已有显式所属表与整单 `position` | #84 解除单表限制；#88 清除剩余默认字段 |
| `ReleaseOrder.Items`、`PublicationResult.Commands`、`Rollback` | API/应用聚合外观由 `rcc_release_details` 和成功摘要组装，主单与执行表不保存整单明细 JSON | #84 分页/多表结果，#88 清理余下外观 |
| `rcc_release_requests.result` 的申请及流程快照 | 精确重放草稿历史版本；实际执行 commands 只引用不可变明细，未复制到请求结果 | #86 维持重推契约，#88 审查并清除不再使用的字段 |
| `RollbackOfID` / `RollbackOrderID` / `RollbackPending`、旧动作枚举/错误与拒绝路由，以及不可达的反向关联分支 | API 不接受来源单字段；没有入口能创建独立回滚单，旧 `/rollback` 明确拒绝；新 UI 不显示旧动作 | #88 删除全部残余字段、分支、路由和旧组件外观 |
| 既有请求恢复组件和相关 sessionStorage 枚举 | 原子性、账号隔离和原键重推保持；T1 未提前删除未知结果保护 | #86 替换最终错误交互，#88 清理剩余恢复外观 |
| 旧字节预算/历史余量与单表整单读取 | 不再用于主单中保存执行明细；现有准备/命令预算及请求大小暂保留 | #84 交付最终1000条无旧整单字节上限，#88 清理预算外观 |

T2 接入注意：草稿内容和目标写入沿用同一个 `ReleaseOrderSession` 事务，先锁原请求再锁主单，随后读写明细。主单 `item_count` 决定当前已有明细的完整主键集合；空明细不取子表范围锁，增删时只锁实际存在的明细键，不能恢复为 `order_id` 空范围 `FOR UPDATE` 或无条件删除末尾范围，否则独立原单会因 MySQL 间隙锁互相阻塞。执行摘要同样按状态读取已存在的完整 `(order_id, kind)` 主键。锁定读取须看到当前已提交值，不能改成事务内先前 RR 快照的普通读取。Adapter 的并发空明细和已有明细增长验收覆盖此协议。`position` 为零基整单顺序，当前替换整单按顺序保存；如分页修改/排序需要稳定明细身份，应在对应切片明确增加。

T3 接入注意：`rcc_release_details` 每行保存 `application`、`publication`、`rollback` 三列 JSON；逆向实际 Command 在同一原始明细行保存，组装 `rollback.commands` 时倒序输出。成功执行键 `(order_id,kind)` 限定每种成功一次；摘要 `table_versions` 为表名映射，`notification` 暂为单表外观。命令已有 `execution_id` 及 `execution_kind`，通知键支持执行+表。统一执行器与记录版本/真实值校验必须继续使用。

T6 接入注意：当前原因只在原单 `QUICK_ROLLBACK` 事件中选填保存；新 `ReleaseExecution` 提供稳定 ID、kind、actor_id、executed_at，补填接口不得改写执行事实或原申请。
