# #93 审批角色管理交付验证

隔离分支 `codex/issue-93-approval-role-management`。最终基点 `bd3e5d16389d7e82dca54ea022dafcb99b993778`，包含已验证 #88；本张最终源码和测试逐文件标识见 [source-manifest.json](source-manifest.json)，其 SHA256 为 `aeffd90b9054c3dd3a6bb6285334353273f877db74c1a39a9093c815ad6c1624`。验收来源 GitHub [#93](https://github.com/asherzj/relational-config-center/issues/93)/[#92](https://github.com/asherzj/relational-config-center/issues/92) 的已确认规格；读取时无评论，AC-001/002 为本张主验收。

环境：Go1.27、Node24.19、pnpm10.28.2、MySQL8.4、Chrome见[browser/result.json](browser/result.json)、Colima Docker29.5.2。真实 MySQL 用例独立创建、清理测试库；浏览器通过 Vite 同源代理连接真实 Admin/MySQL。网络故障在浏览器边界注入，事务及迁移故障用真实触发器、权限和进程中断；不mock内部协作者。

| 最终证据 | 范围与结果 |
| --- | --- |
| [25项MySQL逐项清单](mysql-reconciliation.json) | 角色多对多、身份稳定、成员持久化、重新登录、全局权限不扩张、创建/修改/删除幂等、同键异意图拒绝、并发版本、管理员/认证/CSRF/输入校验、撤权、确定回滚；以及受影响账号维护、旧审批权限、控制表保护、冻结接管与readiness。25个顶层用例最终全部PASS |
| [综合MySQL原记录](mysql-integrated.txt) | 原运行23PASS/1FAIL，266.853s。失败是角色迁移fixture在历史017之后才导入旧数据；原失败保留，不称整次全绿 |
| [迁移定向复验](schema-integration-targeted.txt) | 修正为先导入冻结数据，再应用014～017后，角色迁移PASS14.50s：baseline5、显式up6、部分DDL、确认失败恢复、原数据/账本保留、新装结构等价、运行中表/约束损坏readiness拒绝并可恢复。同次历史v5测试因latest漂移FAIL，保留原记录 |
| [固定历史v5后复验](schema-v5-pinned-green.txt) | 旧3→4→5验证明确构建5，PASS14.980s；未弱化数据/技术身份/账本断言 |
| [最终浏览器](browser-final-integrated.txt) | 12个场景PASS，47.937s，page errors为空：真实人员选择/跨搜索保留、刷新编辑、读取失败缓存及重试、确定回滚可编辑、冲突审阅、已提交响应丢失原请求重试、未送达重试冲突、同账号登录恢复、撤权、换账号清理、启停和无引用删除、取消优先确认 |
| [桌面成员](browser/desktop-members.png)、[390px操作列](browser/mobile-list-actions.png)、[未知结果](browser/mobile-unknown.png)、[同账号恢复](browser/mobile-recovered.png) | 截图等待toast和动画完成，实际字段/抽屉已可见；390px整页不横向溢出，表格可用方向键滚动且操作固定可见 |
| [六Compose结果](compose/results.json)、[原始输出](compose/compose.txt) | 全PASS：新装、重复部署、未接管阻断、显式接管恢复部署、未确认阻断、显式恢复。核查迁移→fixture→Admin顺序、原数据保留、仅最新up变为未确认且其他尝试逐字不变 |
| [Go检查](go-checks-integrated.txt) | Application/Domain/HTTP/MySQL/cmd五包编译、边界与非integration测试全PASS |
| [Web回归](web-regression-integrated.txt) | 请求、真实TCP中断、账号角色和WorkspaceAccess会话隔离，72项PASS |
| [类型](web-typecheck-integrated-green.txt)、[生产构建](web-build-integrated-green.txt) | 均PASS。初次缺fake-indexeddb的环境失败保留；随后按集成锁文件[离线冻结安装](web-install-integrated.txt)，未改锁文件 |
| [Standards](standards-review.md)、[Spec](spec-review.md) | 两位独立只读代理分别复核；原3项规范/坏味道与2项规格/证据问题均关闭；最后测试适配增量检查后两轴均0剩余 |

[实际命令](commands.md)和[依赖集成说明](integration-notes.md)保留范围与来源。Compose使用[逐文件校验的本任务源码副本](compose-source-export.json)以适应Colima对家目录的共享；正式部署配置未改。557份文件复制后逐一核对哈希；验收后只清理随机项目/卷、该副本与本任务manifest生成容器。

迁移最终为 `00006_approval_roles.sql` 和24表完整manifest。[发布文件哈希](released-migrations-unchanged.json)证明00001～00005 SQL/manifest无修改。完整00005的20表定义逐项不变，只增角色、成员、请求结果和永久引用四表。未接管库遵循历史014～017达到冻结5，再baseline登记1～5并显式up6；恢复旧尝试仍使用原目标和摘要。

历史红灯与失败保留：ac001-red→green证明先有创建404；ac002-retry-red→green证明重复创建缺幂等；lifecycle-red证明DELETE路由缺失；schema-red记录旧基点新增迁移错误移动接管目标；browser-red为页面缺失；browser-label-failure为说明可访问名；browser-uncertain-conflict-red为未知请求重试后版本冲突不能审阅；domain-boundary-red→green证明领域ORM标签外泄；rollback-classification-red及browser-review-red证明明确回滚误作未知结果。browser-toast-capture-failure和browser-stable-capture仅为截图等待/隐藏节点定位错误。旧3351112基点的00004验收与全部原日志保留，最终版本6结论只使用上述集成证据。原始终端输出包含ANSI和空白，不改写失败结果。

接口变化见 [角色管理契约](../../admin-approval-roles.md)。本张保留旧全局APPROVER审批能力；真实表绑定/审批快照引用写入及永久引用竞争的完整验收归#94，旧APPROVER最终退出与完整全仓集成归#98。未提前实现通知。父规格、工单收尾及完整功能Notion记录由主协调者负责。
