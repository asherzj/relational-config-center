# PR #109 Web CI 超时修复

基点 `ea36801375a9e888708c6e9c51caea45cff66be4`。GitHub Actions run `34657662155`、Web job `103453445187` 的 480 个测试中 479 个通过，唯一失败是 `CombinedQueryForm.test.tsx` 的 AC-017。该用例在 Linux runner 上耗时 28.283 秒，并触发用例自身的 20 秒超时。原始日志逐字保存在 [`web-ci-initial.log`](./web-ci-initial.log)，SHA-256 为 `e378ce3ac5ff94c06e4f85b1266f37a3919e6154ef62627e274390f5ae5f3e3c`。

## 原因与修复

AC-017 原测试在新增每一个集合值前后都调用 `screen.getByRole`：99 次重复查找同一个“添加集合值”按钮，100 次在持续扩大的无障碍树中查找文本框。测试循环同步阻塞事件循环，因此 runner 直到循环退出后才报告已经超过 20 秒。

本机未修改基线单用例耗时 8.98 秒；临时计时插桩的一轮耗时 10.09 秒，其中按钮角色查询 2.879 秒、文本框角色查询 3.887 秒、点击并渲染 1.537 秒、修改并渲染 1.406 秒。重复无障碍树扫描占已计时循环的约 70%，证明主要瓶颈在测试装置，而不是业务请求或等待。

修复只调整测试驱动方式：缓存 React 更新期间保持同一 DOM 身份的添加按钮；第一项和第 101 项仍通过 `getByRole` 验证可访问名称；中间第 2～100 项通过精确 `aria-label` 选择器定位，并断言每项确实是 `HTMLInputElement`。业务组件、容量、校验与超时均未修改。用例仍逐项驱动真实 UI，验证 100 个实际值成功提交、101 个实际值显示“集合值数量必须是 1 到 100”，且第二次查询不调用 `onSubmit`。

## 复验

- `pnpm exec vitest run src/features/managed-data/CombinedQueryForm.test.tsx -t 'AC-017 100 values'` 连续三次通过：用例分别耗时 1.82、1.80、1.85 秒；测试进程分别耗时 2.77、2.76、2.81 秒。
- 同一目标的最终 JSON 复验通过，用例耗时 1.582 秒，见 [`ac017-fixed.json`](./ac017-fixed.json)。
- `pnpm exec vitest run src/features/managed-data/CombinedQueryForm.test.tsx`：13/13 通过，用例总耗时 3.33 秒；最终 JSON 复验 13/13 通过，AC-017 耗时 1.383 秒，见 [`combined-query-form-fixed.json`](./combined-query-form-fixed.json)。
- `pnpm typecheck`：通过。
- `pnpm test:dev`：受限 sandbox 首次因不允许监听 `127.0.0.1` 而得到 `listen EPERM`；允许临时 localhost 监听后 2/2 通过（128.48 ms、12.51 ms），总计 284.07 ms。这是执行环境限制，不是代码失败。
- `pnpm build`：通过；TypeScript 检查完成，Vite 转换 2,158 个模块并在 807 ms 内完成生产构建。
- `git diff --check`：通过；`[DEBUG-` 搜索无结果，临时计时插桩已清除。

没有重跑完整 Web 测试集：原 CI 已证明其余 479 个测试通过，本次只改变该用例的定位方式；完整 CI 复验由 PR 后续 push 触发。本修复不改变共享设计规则或领域语义，因此 `web/DESIGN.md`、`CONTEXT.md` 与 ADR 无需更新。
