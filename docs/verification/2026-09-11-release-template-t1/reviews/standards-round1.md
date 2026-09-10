# #101 Standards 首轮评审

固定基点：56dff5360e94d904ab85c17cea5cc575919a29b1；工作树：/private/tmp/rcc-issue-101-release-templates。预提交工作区含未跟踪文件；评审 /root/review_101_standards。3项P2成文规范违规，12项坏味道基线无值得单列的主观发现。

1. P2，危险操作保护不完整：ReleaseTemplatesPage.tsx:44 使用window.confirm，35/55未把停用和删除纳入整体pending，写入中可关闭或导航。违反web/DESIGN.md:66的ConfirmDialog职责及95的取消初始焦点、写入关闭保护。使用既有确认组件，全部变更操作纳入关闭、导航及冲突操作保护。
2. P2，成功收尾被未保存保护拦截：Page:38成功直接close，但Form:20注册dirty/pending后丢弃afterSave，成功后仍可能出现正在提交或放弃修改；写入成功无反馈。违反DESIGN:91真实状态与92的Sonner成功反馈。通过afterSave导航并明确成功反馈。
3. P2，错误后无恢复路径：Page:42遇任意commandError永久return，唯一reset在其后；409等确定失败后刷新或重开仍无法重试。违反DESIGN:91真实说明和可执行下一步。区分确定拒绝与未知结果，提供核对最新版本、保留输入及重新操作路径。

已核对新增源文件、测试、迁移及main00001～00005字节未变。最终仍需复核修复差异、真MySQL/浏览器证据、领域术语、设计基准和迁移说明。

快照SHA256前缀：Page 0125cc31a13d；Form 686c4bef91b4；MySQL release_templates adapter 5e307c7787bc；00008清单 d96af644b6d3。
