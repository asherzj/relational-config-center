## Standards

审查对象：基点 `ea36801375a9e888708c6e9c51caea45cff66be4`；不可变快照 manifest SHA-256 `db1b08eced553cfacc725daad3c44620300502090f52ac66117f5b813fd6d6f4`；无新增 commit。

状态：通过（本地修复提交门禁）。

- **硬性违规：0。** `CombinedQueryForm.test.tsx:98-109` 缓存持续存在的“添加集合值”按钮，并以精确 `aria-label` 定位第 2～100 项，仍逐项驱动真实输入；第 1、101 项继续通过角色与可访问名称定位。它保留 `web/DESIGN.md`“查询值控件的可访问名称带‘筛选’前缀”和“容量超限明确阻止查询”的规则，也未引入共享设计或领域语义变化。
- **主观坏味道：0。** 对照 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man、Refused Bequest，差异未出现应报告项。缓存变量 `firstValue`、`addValue` 直接揭示用途；局部测试优化没有新增抽象或重复逻辑。
- **阻塞：0。** 快照内 `ac017-fixed.json` 显示目标用例通过（1.582 秒），`combined-query-form-fixed.json` 显示该文件 13/13 通过（AC-017 1.383 秒）；README 与原 CI 日志一致记录原 Linux 失败为唯一用例超时。
- **待补证：1（非阻塞）。** 修复后 Linux CI 尚未复跑；现有材料不能声称 Linux 已绿。按本次约定，这不阻止本地修复提交，后续 push 的 CI 补证。

结论：硬性违规 0，主观坏味道 0，阻塞 0，待补证 1（非阻塞；Linux CI）。
