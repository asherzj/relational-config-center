# 可靠性阶段 1：真实浏览器验收接入 CI

- 日期：2026-09-07
- 分支：`codex/management-reliability-20260907`
- 基线：`439fa33c5ec0d20be97c9c3efe04e35683f406cf`
- 本机验证：macOS arm64、Node.js 24.19.0、pnpm 10.28.2、Go 1.27.0、Docker Client 29.7.2 / Server 29.5.2、Chromium 151.0.7922.34

## 当前结论

仓库现在提供 `make test-browser-acceptance`。它从仓库根目录安装锁定的 Web 依赖和 Chromium，构建 Web preview 与临时 Admin 二进制，创建本次运行独占的 MySQL 8.4 容器和命名数据卷，动态分配 MySQL、Admin、Web 的宿主端口，加载四份 SQL，并运行：

- `web/e2e/unsaved-changes.cjs` 的 14 项真实浏览器检查；
- `web/e2e/rule-clarity.cjs` 的 6 项真实浏览器检查。

本机冷依赖目录和带空格路径均已跑通。成功与失败都会保存前置安装、构建、服务、浏览器 runner、数据库、结构化结果和可取得的截图；退出 trap 只按本次唯一运行名停止进程并删除本次容器和数据卷。

`.github/workflows/ci.yml` 已增加独立的 `Browser acceptance` job，执行同一 Make 入口，并以 `if: always()` 上传证据。该 job 尚未在 GitHub Linux runner 上实际执行；在父代理提交、推送并触发 Actions 前，不能称为 CI 已通过。

## 一键运行与隔离边界

从仓库根目录执行：

```sh
make test-browser-acceptance
```

可用 `RCC_E2E_ARTIFACTS` 指定新的空证据目录。脚本拒绝非空目录，不覆盖既有证据。每个运行名包含时间和 PID；MySQL 容器、命名卷及 Docker label 都使用该运行名。MySQL 使用 Docker 随机发布端口，Admin 与 Web 使用动态 loopback 端口并启用严格端口绑定；严格端口绑定与本次随机凭据的认证探测共同防止端口冲突时误报成功。

SQL 不依赖宿主目录 bind mount，而是经容器标准输入加载，因此可用于没有共享 `/private/tmp` 的 Colima。MySQL readiness 同时要求容器 PID 1 已进入正式 `mysqld` 和带本次 root 密码的 ping 成功，避免连接镜像初始化期间的临时 server。SQL 命令显式继承 stdin，fixture 加载后立即读取 5 行种子数据作为基线。

Admin 使用本次生成的随机 Bearer Token，浏览器不持有 Token。Token 只进入 Admin 与 Vite preview 进程环境；Vite 同源代理为 `/api/v1` 注入。验收前实测 Admin 直连无 Token 返回 401，同源 Web 代理返回 200。退出前在 Web build 和全部 artifact 中按字面扫描本次 Token、MySQL 应用密码和 root 密码；匹配、扫描错误都会失败，扫描不会输出凭据内容。

依赖安装上限 300 秒，Chromium 安装上限 600 秒，Web build 上限 180 秒，Admin build 上限 300 秒，SQL 单次上限 60 秒，服务 readiness 分别为 90 / 45 秒，两组浏览器脚本默认各 180 秒，CI job 总上限 30 分钟。超时 wrapper 将命令放入独立进程组；先请求 leader 终止，宽限期后清理整个进程组。回归 fixture 真实覆盖 leader 收到 TERM 后退出、孙进程已经安装 TERM handler 且忽略 TERM 的情况。

## 成功实跑

| 运行 | 命令与前置状态 | 真实结果 | 证据 |
| --- | --- | --- | --- |
| 冷依赖目录 | 删除本 checkout 的 `web/node_modules` 和 `web/dist` 后运行 Make 入口 | 04:26:52Z–04:27:37Z，exit 0；依赖安装、Web build、14 + 6、数据库回查、清理通过 | `/private/tmp/rcc-stage1-ci-run4` |
| 带空格路径 | 将当前源码复制到 `/private/tmp/rcc reliability stage1 space`，排除 `.git`、`node_modules`、`dist` 后运行 Make 入口 | 最终运行 04:35:23Z–04:36:03Z，exit 0；14 + 6 通过，0 page error，清理通过 | `/private/tmp/rcc-stage1-ci-space-run3` |
| 超时进程树 | `node --test scripts/run-with-timeout.test.cjs` | 本机多次通过；父代理用独立 probe 复核修复后孙进程不存活 | `/private/tmp/rcc-reliability-record-20260907/timeout-descendant-comparison.json` |

成功运行的数据库 post-check 为：

```text
5|1|5|0|notification_page_query_v1|stage1_mutation_v1|1|DEPRECATED
```

它依次表示 fixture 行数、最小 id、最大 id、残留 `stage2_unsaved_query_` 草稿数、表分配 Query、表分配 Mutation、启用状态和 Mutation 生命周期。除此之外，`fixture-before.tsv` 与 `fixture-after.tsv` 对 id、所有业务字段和审计字段逐行规范化后完全相同；没有仅凭 count/min/max 判断种子数据未变。AUTO_INCREMENT 等允许变化的数据库元数据不在比较范围。

## 故障与清理实跑

| 触发 | 真实退出 | 诊断与清理结果 | 证据 |
| --- | ---: | --- | --- |
| 首版直接 Vite Node 入口未切到 Web root | 1 | `web.log` 明确报告找不到 `dist`；本次容器、卷、端口清理通过 | `/private/tmp/rcc-stage1-ci-run1` |
| 浏览器在 LeaveDialog 关闭后未等待焦点恢复即清空输入 | 1 | `failure.png`、页面文本和 `result.json` 证明输入仍为旧值；最终脚本改为等待 dialog detached 和焦点恢复后再输入 | `/private/tmp/rcc-stage1-ci-run2` |
| 缺少 `pnpm-lock.yaml` 的受控副本 | 1 | `dependencies-install.log` 保存 frozen install 错误；尚未创建 Docker 资源 | `/private/tmp/rcc-stage1-dependency-failure` |
| 临时移走 `web/src/main.tsx` | 1 | `web-build.log` 保存 Vite 解析失败；测试后文件已恢复，尚未创建 Docker 资源 | `/private/tmp/rcc-stage1-build-failure` |
| Docker 指向不存在的 socket | 1 | 当秒失败；`docker-check.log` 保存连接错误，脚本记录当时尚未创建资源 | `/private/tmp/rcc-stage1-docker-unavailable` |
| 非空 artifact 目录 | 2 | 明确拒绝，原 sentinel 仍存在；未创建 runtime、进程或 Docker 资源 | `/private/tmp/rcc-stage1-nonempty-artifact` |
| `RCC_E2E_TIMEOUT_SECONDS=1` | 124 | browser runner 日志与失败 `result.json` 保存；数据库仍为预期终态；容器、卷、服务和浏览器进程均无残留 | `/private/tmp/rcc-stage1-suite-timeout` |
| Admin/Web 就绪后向主脚本发送 SIGTERM | 143 | 主动终止当前 suite 和两项服务；精确容器与卷不存在，监听端口不再响应，`cleanup verified: true` | `/private/tmp/rcc-stage1-sigterm` |
| 抗 TERM 孙进程已 ready 时向主脚本发送 SIGTERM | 143 | 内部 wrapper 在主 trap 的 4 秒等待内完成 1 秒宽限及进程组 KILL；`pgrep` 确认孙进程消失 | `/private/tmp/rcc-stage1-sigterm-process-tree` |

带空格路径的前一次受控运行还发现：将 timeout wrapper 改成可跟踪的后台进程后，如果不显式继承 stdin，MySQL 命令会接收空输入并以 0 返回。fixture 存在性检查使该问题失败关闭；最终 wrapper 使用 `<&0`，随后带空格路径完整运行通过。

## 证据内容

每次完整运行的 artifact 包含：

- `run.txt`：工具版本、精确容器和卷名、起止时间、exit status、cleanup 验证；
- `dependencies-install.log`、`chromium-install.log`、`web-build.log`、`admin-build.log`；
- `docker-check.log`、`mysql-start.log`、`mysql.log`、`sql-load.log`；
- `admin.log`、`web.log`、`auth-boundary.txt`；
- 两个 suite 各自的 `runner.log` 和 `result.json`；
- 正常及断言失败时可取得的页面截图和失败页面文本；
- `fixture-before.tsv`、`fixture-after.tsv`、`database-postcheck.txt`、`database-final.txt`。

进程级强制超时发生时，页面可能已被浏览器进程关闭，因而不能保证取得新的 failure screenshot；此路径仍保存 runner log、结构化失败结果、此前已产生的截图、服务日志、数据库终态和 cleanup 结果。普通 Playwright 断言/等待失败会在浏览器仍可用时保存 `failure.png` 与 `failure-body.txt`。

## 尚待验证与边界

- GitHub Actions 的 Ubuntu runner 尚未实跑新增 job；父代理负责提交、推送、创建 draft MR 并跟踪 Linux 结果。
- 本机冷目录仍复用了 pnpm content-addressable store 和已下载的 Chromium cache；它验证 checkout 不需要 `node_modules`、`dist`、旧端口、旧服务、人工数据库或宿主绝对 Playwright 路径，不等于离线空缓存安装测试。
- Web 使用本次 production build 的 Vite preview 和同源代理做验收；这验证编译产物与当前代理配置，不代表生产托管、TLS 或外部反向代理部署已验收。
- 正常错误可取得 failure screenshot；进程级 KILL 前浏览器可能先关闭，截图为尽力保存，日志、结果与资源清理仍为强制要求。
- 没有重复 Web unit、Go unit/build 全套或 MySQL integration CI。本轮基线 `439fa33` 的既有 CI run `34081770150` 三个原 job 已由父代理核实成功；新增 browser job 必须单独取得 Linux 结果。

父代理独立核验了成功、suite 超时及 SIGTERM 三组测试的精确容器/数据卷均不存在，并先确认 Docker 服务仍可用；读取 `fixture-before.tsv` 与 `fixture-after.tsv` 后 `cmp` 返回 0。对应机器记录保存在本轮外部工作记录目录的 `parent-stage1-cleanup-verification.json`。
