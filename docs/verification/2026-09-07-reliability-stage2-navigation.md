# 确认离开后快速后退的旧草稿拦截修复

日期：2026-09-07；基于工作分支 `9f62a41`，归入可靠性阶段 2。

## 行为与原因

用户在修改规则后确认放弃，再立即后退，偶尔会在已离开的目录上再次看见“放弃未保存的修改”。React Router 已先完成历史地址切换，React 尚未卸载旧草稿；旧的全局 dirty 注册因此错误拦截了从新历史记录发起的导航。

现在每个草稿记录自身的历史 entry key。路由拦截判断使用 Router 提供的 currentLocation key，只有当前 entry 的草稿才参与判断。当前页的未保存输入、同步 in-flight 标记、提交完成信号和 beforeunload 检查继续由原有保护负责。旧状态没有被永久清除，返回后继续编辑仍然受保护。

## 复现与验证

1. Linux run `34090889947` 的 Web job 在原 history back/forward 用例失败，Go、MySQL 与 48 项真实浏览器组合通过。随后 `34092243061` 四项全部成功，说明单次绿色不足以排除这个时序问题。
2. 本机默认并发测试约 9 秒复现同一失败。失败后探针确认 Router 已在 mutation 目录、blocker 正拦截返回 query 草稿，页面也显示旧草稿的离开提示。降低并发或在每步记录日志会改变时序，不能当作修复。
3. 新增确定性组件回归：在一个异步 act 中确认离开、等待 Router 切换，再在 React 提交前立即后退。旧实现 278ms 失败（实际仍在 mutation 目录），修复后 262ms 通过；另断言返回后的当前草稿再次编辑仍阻止离开。反馈命令：

```bash
pnpm --dir web test:run src/test/UnsavedChanges.test.tsx -t 'does not let a discarded draft'
```

4. 对生产构建使用真实 Chromium 151 与 API 响应夹具独立复核，修复前第 4 次失败，修复后 20/20 正常返回原始表单值且无多余离开提示。初始 SPA 历史由程序化 Link 激活构造，随后使用原生 back/forward 和可见确认按钮；该实验不作为 MySQL 全链路或遮罩下主导航可点击的证据。
5. 最终相关三个测试文件共 29 项通过，TypeScript no-emit 与独立 production build 通过。阶段 4 同时在共享树开发，因此没有用其尚未完成的全套运行冒充本补丁的独立检查。

原长序列测试也按交互边界等待确认导航的 React 更新，并断言目标标题和提示关闭。此调整遵循 [React act 文档](https://react.dev/reference/react/act)；它改善测试等待，新增的确定性回归仍覆盖产品在 React 提交前的真实时间窗口。

## 持久证据

工件保存在项目 `.worktrees/.records-reliability-20260907/`：`history-failure-capture-1.log`、`history-failure-probe.jsonl`、`history-discard-red.log`、`history-discard-green.log`、`history-navigation-final.log`、`history-native-red-results.json`、`history-native-red-failure.png`、`history-native-results.json`、`history-native-green.log`。原型 `history-native-repro.cjs` 明确使用响应夹具，并保留原构建与修复构建；未记录凭据。

阶段 2 Linux 浏览器成功工件 ID `10006835687` 已下载并核验 SHA256 `d8d69b67bce615c6c97c95e4f6d0dbe1d11890c1c2a5e7949f7d78186fb29548`，目录 `stage2-linux-success`。其中 run.txt 为正常退出与清理通过，完成于 06:29:47Z。阶段 3 Linux 四项 CI 完成于 06:57:40Z；本补丁推送后的 CI 单独跟进。
