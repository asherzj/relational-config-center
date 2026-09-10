# #102 表发布流程设置 T2 验收记录

验证日期：2026-09-11。固定基点 `8b79faa3b720f7e2a6837aa43ba7952b0ee76028`，分支 `codex/issue-102-table-release-templates`。

## 交付范围

本票新增每表每发布类型的模板关联配置、正式管理抽屉、常规停用、应急完整性保护、原子切换、表规则创建/启用同事务保障、独立版本与永久账号审计。关联及表规则全部相关管理写入保存原请求结果；手动重推不重复执行、不覆盖后续选择，每次重新验证当前权限。同键异内容拒绝，旧版本冲突保留输入。模板仍通过单一 `node_list` 保存整套节点；本票只新增第二张配置表。

本票没有接通流程实例、应急执行、审批旁路或原单回滚。候选迁移 9 只在隔离库验证，尚未部署；已发行 1–5 与已交付候选 8 均保持字节不变。外部 #94 的已验证提交 `2612bb354ef71d42e193648564b9f30bd9281eb4`（包含6/7）由 #103 接入，保留外部发行文件、调整未发行候选并从真实 MySQL 重建累计清单。历史接管边界固定5。

## AC 索引

| AC | 验收内容 | 证据 |
| --- | --- | --- |
| AC-006 | 两个表共享或选择不同模板；每表每类型唯一；同类型外键；常规可停用；应急不能停用/解绑/删除；整表可停用；被引用标准模板删除受保护 | `TestTableReleaseTemplateHTTPAC006AndAC008SelectShareProtectAndDisable`、`TestTableReleaseTemplateHTTPCurrentAuthorizationAndRequiredRequestIdentity`；[后端记录](backend/README.md)、[浏览器记录](browser/README.md) |
| AC-007 | 既有表默认关联初始化幂等，不覆盖选择；标准默认停用或删除不恢复；创建/启用与有效应急关联同事务，失败全回滚；结构升级与只读ready | `TestTableReleaseTemplateHTTPAC007AtomicCreateEnableAndRequestResultFailure`、`TestTablePolicyHTTPConcurrentCreateAndEnablePreserveEmergency`；[迁移11项有效覆盖](migrations/effective-passing-coverage.json)、[迁移记录](migrations/README.md) |
| AC-008 | 两个独立ADMIN同旧版本竞争只成功一次；有效应急始终保留；原请求历史结果重放不覆盖后续选择；页面保留冲突输入/未知原包，重新登录后手动原样重推 | `TestTableReleaseTemplateHTTPConcurrentSwitchAndOriginalReplay`、结果写入故障测试；[Web回归](web/README.md)、[真实浏览器](browser/README.md) |

## 最终结果

- 37个后端公开HTTP/真实MySQL顶层用例有效通过：36项来自批次07，旧双账号调用者修复后由08补验。批次原失败保留，逐项映射见 [后端有效覆盖](backend/effective-passing-coverage.json)。包含全部6个T2及T1原17项。
- 11个迁移/ready/历史基线顶层用例及子用例通过。
- Web四文件70项有效通过；最后互斥修复重跑受影响两组件20项通过，其余50项源码未变。最终类型检查和生产build通过。
- 正式浏览器最终运行05通过，8项系统检查，23.82s；真实桌面、390px、输入保留、会话恢复和原请求重推均验证。
- application/HTTP包完整单元及边界通过；cmd包升级权限后完整通过。源码空白检查无输出；`--no-index --check` 对新文件返回1表示与空文件有差异，其输出为空，没有空白问题；原始退出码记录在 `source-checks.json`。5个改动cjs脚本语法检查通过。

## 验证边界

后端公开 HTTP 使用真实 MySQL，不替换自有服务、仓库或 Catalog；故障注入位于真实数据库触发器或网络响应边界。全部容器按测试串行创建/清理。迁移与ready 11个顶层用例全部通过，范围与最终输入哈希见迁移子目录。共享事务变更涉及的T1模板HTTP重新运行；未受影响的历史模板设计和已有工作区隔离实现仍参考基点下 [T1记录](../2026-09-11-release-template-t1/README.md)，不声称旧事务运行验证了新代码。

Web组件、HTTP边界、类型检查/构建、正式Chromium浏览器分别记录原命令、失败与复验。源码/文档空白检查与原始日志/diff分开；原始产物保留工具输出空白，不为通过检查而改写证据。没有运行全仓全量集成；最终跨切片回归由T6负责。

## 最终独立评审

Standards与Spec三轮均以固定基点独立审查；第三轮均为0项未解决发现。初轮和第二轮原始报告完整保留，修复与真实红绿回归在对应Web记录中。

- [Standards末次放行](reviews/rcc-102-standards-review-round3.md)
- [Spec末次放行](reviews/rcc-102-spec-review-round3.md)

暂存后的全部58个源码/文档路径执行 `git diff --cached --check` 退出0且输出为空。最终两轴之后只归档报告及更新证据索引，没有改变生产源码或测试。

## 清单

`source-sha256.txt` 记录本票源码、迁移和文档；`evidence-sha256.txt` 记录全部验收产物（清单自身除外）。两轴评审原文与逐轮修复在 `reviews/`，均从相同固定基点独立进行。
