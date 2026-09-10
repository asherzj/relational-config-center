# 正式浏览器验收

真实 Chromium → 同源 Vite 代理 → Admin 独立进程 → MySQL 8.4。测试通过正式注册与维护命令授予 ADMIN，业务准备使用公开 API，操作与故障保留使用正式页面。每次运行只启动一个任务自有 MySQL 容器，测试结束完整终止。

环境：`TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache`，另将 `RCC_E2E_OUTPUT` 设为本目录内的输出子目录。

```sh
go test -tags=integration,browser ./admin/cmd/admin -run '^TestTableReleaseTemplateBrowserSystemPath$' -count=1 -v
```

原始日志逐次保留：01 脚本误取预认证 CSRF，业务写入403；02 下拉框与节点列表采用模糊 label 导致定位歧义；03 所有主要业务路径通过后，最后关闭按钮定位到共享 Drawer 的两个关闭入口而失败，三张截图保留在 `attempt-03/`；04 修复定位并补充真实注销/同账号重新登录，8 个系统检查全部通过，用例23.88s/package24.863s。

最终检查覆盖：真实提交后中断创建响应，以相同 URL/正文/键手动重推；关联提交后中断响应，注销会话再登录仍保留原包，跨401原样手动重推；两编辑器旧版本冲突保留输入；标准停用和应急保护；390px 长名称、键盘可达与无水平溢出；Escape 通过共享未保存确认保留选择；整表仍能停用。数据库读回核对永久 Account ID 审计。截图人工复核桌面布局、390px、冲突与动作区。

最终源码运行05通过（用例23.82s/package24.788s）。04的产物归档 `attempt-04/`；最终 `final/` 包含 JSON 和四张截图，冲突错误及390px底部操作区先实际滚入视口再截图。期间只修复恢复读取/写入互斥以及增强截图取景，全部8个系统检查重新通过。
