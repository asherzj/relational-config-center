# T3：原子化重新准备已批准发布单

状态：已交付；[#62](https://github.com/asherzj/relational-config-center/issues/62) 已关闭。实现与验证见[验收索引](../../verification/2026-09-09-release-detail-acceptance-index.md)。

## 父工单

Part of #59：https://github.com/asherzj/relational-config-center/issues/59

## 要构建什么

原申请人或 ADMIN 从已批准普通单核对最新配置并重新准备，旧单取消与新草稿同时形成；新申请人是实际操作者，必须重新审批。

## 验收标准

- [x] AC-009：成功时旧单取消且创建同一可恢复新草稿；新申请人为实际操作者且不可自批；明确失败保留原审批与占用。

## 验收用例

主要交付：AC-009。采用父规格的 Given/When/Then，不引入未确认需求。

## 验证证据

真实 HTTP/MySQL 验证所有权、自批拒绝、明确故障原单不变、重试同一草稿；Web 验证预览确认与结果未知恢复。

## 临时结构与退出条件

无。完成实现和本工单验证，修复此前中间改动；不得保留假成功、旧数据兼容或只声明未来测试。

## 阻塞于

- #61

## 执行分配

一名全新上下文的独立 Sub Agent，在专属分支和可写 git worktree 中完成本工单实现、验证、评审与交付；模型 GPT-5.6 Sol，思考强度 high。主智能体负责依赖核验、接口协调与功能级证据汇总；本工单所需的后端、界面、测试和上下文更新均由本工单负责。
