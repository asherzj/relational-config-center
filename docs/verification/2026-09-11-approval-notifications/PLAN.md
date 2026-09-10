# #96 验收边界与进行中记录

基点 836c95da4100d06a102dc6cd0266035cafe18ceb；原规格和工单完整内容见 issue-92.json / issue-96.json。本票主要交付 AC-013～018，#97 接入发布/完结/回滚/重新准备，#98 统一全仓验证及旧权限退出。

已确认接缝：正式 Web；已认证 Admin 公开 HTTP 与真实 MySQL；正式 schema-migrate 和维护程序。测试不以私有方法或查询数据库读取业务结果代替 HTTP。SQL 仅用于外部依赖故障注入、结构事实及维护夹具。

逐切片：提交收件聚合（404红→绿）；审批/拒绝/取消（缺结果提醒红→绿）；成员/角色/账号/ADMIN 正常入口（旧待办未清理红→绿）；详情观察序号（缺详情进度红→绿）；通知故障验证沿现有事务直接具备，无为仪式制造红灯。首次故障测试误将未知字段解析400写成422，保留原轮并将其改为真正同键异正文409验证。首次Docker未显式设置host导致provider不可用，单独保留，不冒充HTTP行为红灯。

重型限制：显式 Colima socket，TESTCONTAINERS_RYUK_DISABLED=true；每次仅一台本票自有MySQL；真实业务并发仍在同一fixture保留。浏览器、迁移、真实MySQL套件串行。用户 deploy-mysql-1、rcc-main-preview-admin-1 不操作。
