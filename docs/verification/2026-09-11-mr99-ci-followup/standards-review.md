# MR #99 Web CI 跟进最终 Standards 复审

固定基点：`dafdde3a4a41a2b8afd03ae57fa51b9908fae08b`。范围仅为 `CombinedQueryForm.test.tsx` 与 `browser-accessibility.cjs`。文件 SHA-256 分别为 `beaa0b41d8992270e47e0ee585153dca297a7949fd78cdff93db7b57a0e20c1b`、`2efb5cfbb0345582a8f4b5a249251ad9be0eda6de42c4f99efff7d8b9695a0ef`；`source-final.json` SHA-256 为 `d8b34b0d73f9ea6d6602c527ddc44dc00355720b5eae4db106e0dfdc488f94dc`。

## Findings

- 成文规范违规：**0**
- 主观坏味道：**0**

容量测试缓存两个稳定按钮，仍用字符串可访问名称和逐项精确 label 取得真实控件，再执行真实 click/change；100 项完整有序值、101 项明确拒绝、仅一次 submit 与 20 秒期限均保留。移除不受类型支持的两个 `exact` 选项不放宽字符串名称匹配。

无障碍脚本通过明确 `table_name` URL 打开目标表；“新增记录” trial 只验证真实可操作性，随后独立核对 select 当前值。本次页面捕获的所有表查询都必须存在、返回 200，并严格命中该表的唯一 API 路径。创建请求再断言默认标题及唯一明细的真实表名；因此没有从 `selectOption`、按钮出现或 DOM 内容推断请求已经使用目标表，符合 `web/DESIGN.md` 对真实浏览器、真实请求和可访问控件的验收要求。

改动局限于测试证据强化，没有生产行为、固定等待、全库清理或新增抽象；`git diff --check` 无格式问题。

## Runtime 边界

本次未运行测试、数据库或浏览器。协调者提供的 results3 已证明单文件 13 项、Web 398 项及 typecheck 通过；正在运行的三引擎无障碍结果不属于本轴结论，不能预报为通过。
