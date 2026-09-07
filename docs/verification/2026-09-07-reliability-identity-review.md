# 主键返回、写入恢复与事务补审

本补审接续阶段 4 的 `49fc66f`。新增时，返回给页面的 ID 必须对应本次实际写入的记录，并能用于后续查询和修改；若响应丢失，页面不能把用户输入的 ID 当成数据库实际保存的 ID。

## 真实问题与修复

| 已复现问题 | 最终行为 |
| --- | --- |
| 提交整数 ID `00077`，数据库保存 `77`，接口却回显 `00077`。小数、ENUM、时间也可能改变表示。 | 在原 INSERT 事务内按绑定值读取数据库的字符串 ID，再按正常类型解析和 exact 查询确认其可用性，成功后才提交。可定位的规范化值正常接受。 |
| 显式自增 ID `0` 成功后丢响应，数据库已生成另一 ID；只读核对却查 `id=0`，漏掉实际记录。 | 未知 ADD 使用规则默认第一页核对，不猜生成 ID。已有 MODIFY/DELETE 仍按本次操作已知的 ID 查询。读取不会自动解锁写入，也不会把一页没有记录解释为未写入。 |
| 文本 ID `键/值 ?#%` 新增、查询正常，但 PATCH 的编码斜杠被路由提前解码，返回 404。 | Router 使用原始转义路径匹配并只解码参数一次；PATCH/DELETE 将完整 ID 交给原有类型校验与授权。真实代理 PATCH 已通过，HTTP 回归同时覆盖鉴权、未知路由和受保护表。 |
| DECIMAL/FLOAT/CHAR 等转换后可能无法按原值或序列化后的 ID 精确定位。 | 返回已知 400 并在原事务回滚。不会提交一个页面无法重新定位的身份，也不声称支持任意舍入、截断或主键转换。 |
| MyISAM 的 DECIMAL 舍入新增报 400，但回滚后 SQL 仍有一行。 | 所有 ADD 写前确认存储引擎支持事务，否则现有 422 `incompatible_table`，没有 INSERT。先真实读取目标表取得元数据锁，再查引擎能力，避免期间被 ALTER ENGINE 改成非事务表。 |
| 空字符串、`.`、`..` 被新增成功，却无法完成现有 HTTP 路径流程。 | 原始值在 INSERT 前拒绝；数据库规范化后的值也检查。例如 CHAR PAD SPACE 将 `.  ` 规范为 `.` 时回滚。 |
| BEFORE INSERT 触发器把 77 改成 88；表中已有 77 时，接口将旧 77 当作新增结果，SQL 实际有两行。 | INSERT 前使用 `FOR SHARE` 当前锁定读记录提交键是否已存在，仍执行 INSERT。真实 MySQL 1062 才映射 409；INSERT 成功但旧键存在时，不能用旧行证明新增身份，返回 400 并回滚。提交键原先不存在、触发器改成无法按原值定位的新键时也回滚。 |
| `TINYINT(1) AUTO_INCREMENT` 第二次省略 ID，返回 `2` 并落库，但当前 boolean 主键契约只接受 `0/1`。 | 应用层在原事务回调内对数据库返回的 ID 执行快照类型解析，第二次返回 400 并回滚，仅保留首行 `1`。不假设自增计数器随行事务回滚。 |
| 自增表的 BEFORE INSERT 触发器把 ID 改为 `88`，接口却把该连接先前的目录写入 ID `6` 返回成功；省略 ID 和显式 `0` 均复现。 | 从本次 INSERT 的 `Result.LastInsertId()` 获取 ID，不再读取可能残留的会话 `LAST_INSERT_ID()`。锁定的 MySQL driver 将协议中的 uint64 保存为 int64；用 `uint64` 还原位模式再格式化字符串，保留完整无符号范围和实际零 ID。 |

所有 ADD 都可能在获得实际 ID 后发现其无法用于当前公开契约，因此事务能力检查覆盖全部 ADD；查询、修改和删除流程不在该检查范围。公开路径的原始值和规范化值约束属于应用层，并在原事务回调内执行；直接调用存储 adapter 可以合法保存 `.`。省略自增 ID、显式自增零均使用本次 INSERT 结果，取得结果后仍经过应用校验。用户提供非自增 ID 的 ADD 行为是本轮对 #22 旧“隐藏所有 id”选择的明确扩展，MODIFY 仍禁止修改主键。

`FOR SHARE` 保留只授予 SELECT/INSERT 的使用边界；预查使用当前锁定读，是因为管理规则读取已经可能建立较早的 REPEATABLE READ 视图。[MySQL 锁定读说明](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html)确认该读取使用当前值，锁在事务结束时释放。实际表读取所持有的[元数据锁](https://dev.mysql.com/doc/refman/8.4/en/metadata-locking.html)使能力检查与 INSERT 之间的引擎变更需要等待；[非事务表](https://dev.mysql.com/doc/refman/8.4/en/nontransactional-tables.html)不提供相同的回滚保证。数据库查询、超时、权限或并发 DDL 错误继续按未知写入结果处理，不能被归成确定未写入的普通输入错误。

## 证据层次

- 真实 Chromium → production Web 同源代理 → Bearer Admin → 专属 MySQL：复杂字段原 18 项之外，加入 17 种主键表示、规范整数回读、丢失零 ID 响应、四个不可用主键/引擎边界及读权限预检。主键矩阵的 ADD、Change Set 和结果查看通过页面完成；后续精确查询/PATCH使用 Playwright APIRequest 经过真实代理，没有将其记成页面点击修改。
- TIME 默认舍入 `.777` 可精确定位 `.78`，因此正常接受；单独以 `TIME_TRUNCATE_FRACTIONAL` 启动专属 MySQL，返回 `.77` 的查询与 PATCH 也通过。该模式只用于本次容器，未改既有数据库配置。
- 最终真实 MySQL focused integration 共 7 个顶层组，日志 `identity-contract-final.log`，09:03:29Z–09:04:22Z、53.412 秒、全部通过、无跳过。每组专属 Testcontainer，结束后停止并删除；此前 6 组日志保留为修订前证据。
- 写后权限故障：仅测试代码的 GORM callback 在实际 INSERT 后，先读到事务内一行，再经独立管理员连接撤销该表 SELECT；下一次真实 MySQL SELECT 返回 1142，最终独立连接查零行。没有产品测试钩子或伪造数据库错误。
- 引擎并发：实际观察到 ALTER ENGINE 的 metadata lock 为 PENDING；放行 INSERT 后，本次 MySQL 以 1213 安全失败、SQL 零行，事务结束后 ALTER 完成。测试也接受事务正常提交一行后 DDL 完成，不能要求并发 DDL 下写入必然成功。
- 旧快照与触发器：真实 SQL 分别证明旧记录未被改变、新记录回滚、另一事务后提交的重复键被当前读取看见，并由实际 INSERT 的 MySQL 1062 裁决冲突。

## 红灯与测试夹具修正

持久记录位于 worktree 同级 `.records-reliability-20260907`。`stage4-identity-review-red` 保存错误 `00077` 返回和丢响应零 ID 查询；`stage4-id-matrix-red` 保存表示与回查边界；`stage4-identity-guards-red` 的 MyISAM 400/SQL 一行和三个不可用 ID 201/SQL 一行均为真实产品红灯。`identity-trigger-red.log` 保存返回旧 77、实际新增 88 的触发器红灯。

权限夹具最初关闭连接池后，下一次请求先遇旧连接 EOF，并未到 INSERT；后改为通过有限次只读 catalog 请求耗尽被关闭的池连接，写请求始终只发一次。general_log 的 BLOB 参数也必须显式转 UTF-8 后加入 JSON。MySQL 可在记录 SELECT 语句前就因权限拒绝准备，因此浏览器证据只断言事务开始、回滚、没有 INSERT、SQL 零行，不声称看到未记录的失败语句。写后 SELECT 1142 的证据由上述真实 Go 集成提供。

第一次全 complex-fields 运行的前 28 项通过，之后权限夹具准备失败；它不是最终通过记录。页面选表与响应等待现使用同一个 Promise.all，使准备失败也能保留结构化结果并走清理，不再因未处理的等待 promise 丢失部分结果。

本机第一次未配置 Colima provider 的 integration 命令退出 0 但健康检查跳过；只补 DOCKER_HOST 时又遇 Ryuk 挂载 macOS socket 失败。这两次都不算真实集成通过。成功配置包含 `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`，保持 Ryuk 正常运行；最终验证逐项检查真实执行和无测试跳过。

## 双轴审查

### Standards

冻结 `439fa33...49fc66f` 的独立审查最终没有成文规范违规；初次对 1264/1406 分类的质疑已按既有类型精度和错误契约撤回，原始审查经过保存在 records。新增实现的 delta 审查发现两项成文规范问题：HTTP 身份约束不应放在存储 adapter；唯一冲突须由真实数据库唯一索引裁决。现将公开 ID 校验移至应用事务回调，并保留 INSERT 的真实 1062 判定；最终只读复核另行记录。

`review-standards-identity-final.md` 已确认上述两项关闭，没有新增成文规范违规。自增触发器的后续修复另以实际 driver 返回值和数据库内容核对：小 ID 为 driver `88` / session `6` / SQL `88`；大 ID 为 driver `-2` / SQL `18446744073709551614`；`NO_AUTO_VALUE_ON_ZERO` 为 driver `0` / SQL `0`。这项转换依赖仓库锁定的 go-sql-driver/mysql v1.10.0 实现，升级 driver 时须保留这些真实数据库回归。

### Spec

原冻结审查发现 1 个 P2：丢响应的零 ID ADD 核对错误主键。根代理已补确定性组件红绿和真实写入故障回路。后续主键补审另外发现非事务回滚与不可表达 ID 两个 P2，以及实际触发器误认旧行；均依据真实数据库证据修复，不将这些追加发现混算到原冻结审查的数量。

## 最终检查点

自增触发器补修前的稳定组合 `final-combined-macos` 于 09:07:55Z–09:10:32Z 完成：14 + 6 + 28 + 20 + 42 + 21，共 131 项通过；三引擎各 7 项、页面异常为空、fixture 全内容相同、资源清理通过。它代表该次代码检查点，不能替代后续自增修复及 main 账号功能合并后的结果。

MR #44 于 08:58:53Z 合入 main `a9f5830`，更新为 Cookie 会话与账号操作人。本分支与其存在交叉修改，最终完整组合、提交和 Linux CI 必须在合并这些修改后验证。

自增触发器修复后的最终 focused MySQL 回归 `identity-auto-result-final.log` 于 09:14:51Z–09:15:56Z 完成，8 个顶层组和新增组 9 个子用例全部通过，无跳过，64.753 秒。应用及 MySQL 非 integration 回归通过。便携证据见[主键补审 JSON](2026-09-07-reliability-identity-review.json)。
