# 缺陷转检查

一类错误被发现并修好之后，仓库里应留下一条自动检查，让同样的错误不能再进来。人和智能体都走同一个入口：`make lint`。GitHub Actions 的 `Go lint` 作业执行这条命令；它与原有的 Web、Go 单元与构建、MySQL、浏览器和 Goose 作业并列，不改那些作业的步骤。

## 什么时候加规则

修缺陷时问一句：有没有一条检查能挡住这一整类，而不只是这一处？

- 能，并且当前代码已经满足（或只需改到满足、且不改变既有行为）：把规则放进同一次变更。
- 现有代码还做不到：不要为了打开规则去改写无关行为。先把这一类改干净，或把规则限定到已经干净的包，再启用。
- 表达不了稳定的机械判断：在修复说明里写明为什么没有加规则。不要提交一条会误伤现有正确代码的检查。

## 怎么跑

在仓库根目录：

```bash
make lint
```

它做两件事，任一失败则命令失败：

1. 用仓库锁定的 golangci-lint v2.15.0 执行根目录 `.golangci.yml`。二进制装到 `bin/golangci-lint`（已忽略，不提交）。工作区根目录不是 Go module，所以命令按 `./admin/...`、`./client/...`、`./server/...`、`./shared/...` 和 `./tools/lint/...` 运行。
2. 测试并编译 `tools/lint/cmd/rccvet`，再对每个模块执行 `go vet -vettool`。这个 vet 工具只跑在这里注册的分析器，不替代单独的 `go vet`。

默认构建标签与 `make test` 相同，不包含 `integration`。带 `integration` 标签的测试可以在组合根里装配适配器；非测试代码仍受下面的分层规则约束。

## 现在启用的规则

只启用能解释、且当前代码已经满足的规则。没有打开 golangci-lint 的默认风格套件。

### 分层（depguard）

Admin 的依赖方向以 [ADR 0012](../adr/0012-enforce-admin-ddd-boundaries-with-go-internal.md) 为准：Domain 不依赖 Gin、GORM、`database/sql`、HTTP 或 MySQL；Application 只依赖 Domain 和自己的端口；HTTP 处理器只依赖 Application；具体适配器在 `cmd` 里组装。`platform` 只放部署配置，不反向依赖用例、领域或存储。

Infrastructure 实现 Application 的端口，因此可以导入 Application。这比 `docs/architecture.md` 里 “infrastructure 只指向 domain” 的图更晚，也是当前代码的实际方向。规则按 ADR 0012 和现行代码执行，不把这条已有依赖判失败。

服务之间不共享 Go 包（[ADR 0004](../adr/0004-share-protobuf-contracts-only.md)）：Server、Client、Shared 不导入 Admin；Shared 不导入 Server 或 Client；Client 与 Server 互不导入；Admin 不导入 Server 或 Client。`shared` 仍是以后唯一的跨进程契约位置。

HTTP 规则不覆盖 `*_test.go`。测试可以装配 MySQL 适配器；生产处理器不行。

### 库代码里的 fmt.Print（forbidigo）

`fmt.Print`、`fmt.Printf`、`fmt.Println` 在库代码里会把诊断写到进程的标准输出，调用方无法把它当成错误处理。规则禁止这三项。`fmt.Fprintf` 写入明确的 `io.Writer`（例如摘要哈希）仍然允许。`cmd` 是进程边界，可以写到 stderr 或启动横幅，因此排除 `(^|/)cmd/`。

### 库代码里的 panic（librarypanic）

Admin 处理配置发布和查询。库代码里新的 `panic` 会让进程退出，请求上的恢复中间件看不到这次失败。`tools/lint/passes/librarypanic` 禁止内建 `panic`，但允许两类既有写法：

- `cmd` 和 `*_test.go`：进程入口和测试可以主动失败。
- 把仍保存 `recover()` 结果的变量再 `panic` 出去，或直接 `panic(recover())`。这个变量一旦被重新赋值，豁免消失。查询快照在回滚事务之后把原 panic 重新抛出，HTTP 恢复中间件才能把它收成 `internal_error`，同时不把驱动错误写进响应。这不是引入新的 panic。

同包里自定义的、名字也叫 `panic` 的函数不会被误报。

## 加一条 golangci-lint 规则

编辑 `.golangci.yml` 的 `linters.settings`，然后跑 `make lint`。分层用 `depguard` 的 `list-mode: lax`，只写 `deny`，并用 `**/.../*.go` 同时覆盖包目录本身和子目录。标识符用 `forbidigo`，并打开 `analyze-types`，这样 `import xyz "fmt"` 仍然能认出 `fmt.Println`。

`cmd` 或其他路径需要整段排除时，写到 `linters.exclusions.rules`，不要在业务代码里散落 `nolint`。

单个调用点确实表达不了时，用带原因的指令：

```go
fmt.Println(banner) //nolint:forbidigo // process boundary banner owned by this cmd
```

优先把例外写进规则（像 librarypanic 识别 `recover` 重抛），而不是增加豁免。

## 加一个 go/analysis 分析器

depguard 和 forbidigo 表达不了的结构（控制流、只有某种赋值才允许）放到 `tools/lint/passes/<name>`：

1. 导出 `*analysis.Analyzer`，用 `analysistest` 覆盖应报告和不应报告的例子。
2. 在 `tools/lint/cmd/rccvet/main.go` 的 `multichecker.Main` 里注册。
3. 跑 `make lint`。分析器测试会先执行，再对全部工作区模块跑 `rccvet`。

## 还没启用的检查

`errcheck`（包括赋给 `_` 的错误）没有全局打开。Domain 的 canonical row 校验和会忽略一次已知可以编码的 `json.Marshal`；Application 和 Infrastructure 里也有回滚清理、关闭连接时有意丢掉的错误。把这些行为一次性改掉不属于引入检查本身。

下一次缺陷如果就是“错误被丢掉”，把那个包改到能通过 `errcheck` 的 `check-blank: true`，再只对那个包启用。不要在规则还是红的时候把它设成 CI 门禁。
