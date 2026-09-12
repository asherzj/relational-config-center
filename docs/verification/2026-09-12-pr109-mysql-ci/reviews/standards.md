## Standards

审查对象：基点 `4092bd08db134b5282914556f60ec7f07b534091`；不可变快照 manifest SHA-256 `55481f454748fcd29d62eada58b4e4c0e9eed408c849dd0f918323c2f09a9ae3`；6 个源码路径、15 个证据路径。

状态：通过本地固定快照 Standards gate。

- **Hard violations：0。** 运行器从 `go test -tags=integration -list . -json ./...` 动态发现包限定的顶层 Test/Example/Fuzz，拒绝重复身份和空清单；按 `(package, name)` 排序轮转四组，并以锚定 `-run`、JSON 事件和包终态拒绝漏跑、额外顶层项、skip、失败及非零退出。普通通过子测试保留在所属顶层项内；包失败后继续后续包，`all` 模式组失败后继续后续组并最终失败。`Makefile` 默认仍串行执行全部四组，每个包保持 60 分钟；CI 四个独立 runner、`fail-fast: false`、`always()` 上传原始证据，与 README 和 `docs/schema-migrations.md` 一致。
- **Subjective smells：0。** 新模块围绕发现、分组、执行、核账和证据一个职责组织；未见重复逻辑、神秘命名、无需求抽象、发散修改或其余基线坏味道。
- **Unresolved findings：0。** 未发现会导致漏跑、重复运行、错误接受 skip/失败或破坏默认入口兼容性的路径。
- **Local gate pending：0。** 21/21 快照文件、内层 13/13 证据和 6/6 源码哈希匹配。运行器 5 项测试通过；独立与运行器清单精确匹配 431 项，分组为 108/108/108/107；原超时栈中的 MySQL 测试独跑 10.23 秒通过，普通 Go 套件通过。
- **Post-push CI：1（非本地 gate 阻塞）。** 四个完整 Linux MySQL 分组尚未执行，不能声称 CI 已绿。

结论：hard 0，subjective 0，unresolved 0，local gate pending 0；post-push CI 待补 1。
