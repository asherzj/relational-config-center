# Web 验证

所有命令从工作树根目录执行，使用冻结 lockfile 的离线 pnpm 依赖。未改变依赖清单。

- `01-components.log`：旧表规则测试仍期待独立只读核对及无版本正文，四项失败；适配原请求手动重推后 `02-components.log` 的组件/API 17 项通过。
- `03-association-components.log` 与 `04-association-components-fixed.log` 保留版本冲突被客户端误归类为未知结果的失败；补充明确拒绝错误码后 `05-association-errors-fixed.log` 的关联组件与客户端 50 项通过。
- `06-build.log`：初次类型检查和生产构建通过。
- `07-association-toast.log`：使用共享 Sonner Toast 后关联组件 3 项通过。
- `08-review-regressions-red.log`：新增未知结果→确定版本冲突与历史重放结果/当前状态区分两项真实红。
- `09-table-review-red.log`：新增整表状态命令版本冲突恢复测试真实红。
- `10-review-regressions-green.log`：生产修复后关联 5 项通过，表规则新断言把“请求编号”误写为“请求 ID”，保留此失败；修正断言后 `11-review-regressions-fixed.log` 四个文件共 70 项通过（5.26s）。
- `12-build.log`：修复后 TypeScript 与生产构建再次通过。

最终组件命令：

```sh
pnpm --dir web exec vitest run src/features/table-policies/TableReleaseTemplatesDrawer.test.tsx src/features/table-policies/TablePoliciesPage.test.tsx src/api/table-policies.test.ts src/api/client.test.ts
pnpm --dir web build
```

回归证明：同类型选择与标准停用；失败保留输入和请求编号；403 期间保留未知原包；明确版本冲突释放未提交原包，读最新后用新键/新版本提交；原请求历史成功结果不替换当前读取；干净关联表单同步最新版本；读取最新失败时即使存在缓存，也保留原错误并显示真实读取错误，成功核对后才能清除错误。既有同账号重新登录保留表规则输入也通过。组件只替换浏览器 HTTP 边界。

第二轮评审补验：13 的慢恢复读取期间写按钮仍可点击，真实红；恢复读取纳入共享 inFlight/pending、禁用保存/启停后，14 重跑两个组件共20项通过。15 对最终生产源码重新类型检查/构建通过。13只选择 `-t 'recovers a state-command'`；14命令为：

```sh
pnpm --dir web exec vitest run src/features/table-policies/TablePoliciesPage.test.tsx src/features/table-policies/TableReleaseTemplatesDrawer.test.tsx
pnpm --dir web build
```

其余API/客户端50项仍复用11，期间这些源码和测试文件未变。旧e2e脚本的表规则键/版本调用适配以静态语法检查覆盖，其关联后端语义由当前HTTP回归覆盖；未声称运行全部旧浏览器套件。
