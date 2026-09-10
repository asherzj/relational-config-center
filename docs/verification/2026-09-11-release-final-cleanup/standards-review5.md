# Standards 增量复审（candidate 5）

固定差异：`00ea9518131727e429d0ffefbb4ba8c077d8c07d..f0228e09a08ca55d0c85a946bb25aa80a7974ea3`，仅 `web/e2e/release-drafts.cjs`。候选 manifest `source-candidate5.json` 的实测 SHA-256 为 `98424453575ff9ae6c9c3760dd850e1e16ef855394bb03a611d8edfa3bcdfa41`。

## 成文规范违规

无。

双窗口同步没有降低真实断言。第二窗口在点击前注册精确到该发布单 PUT 的响应等待，断言 HTTP 200，再等待编辑 Drawer 关闭和保存值出现；第一窗口随后才发自己的 PUT，直接断言 HTTP 409，并继续验证冲突提示和未保存输入。该顺序消除了仅凭 DOM 文本推测另一窗口写入已提交的竞态，符合 `web/DESIGN.md` 对服务端版本检查、冲突保留输入、浮层状态和真实浏览器验收的要求。失败捕获只在配置输出目录时保存每个未关闭共享上下文页面的截图与 body 文本，单项写证据失败被局部吞掉，原始测试错误仍重新抛出，不会把失败转为通过。

## 主观坏味道

无。新增同步与既有同文件的 `waitForResponse`、HTTP 状态断言和 Drawer 隐藏等待保持一致；失败证据循环范围小且职责单一，未形成 Duplicated Code、Mysterious Name、Speculative Generality 或其他基线坏味道。

## 证据边界

本次仅进行只读 Standards 复审，未运行测试、数据库、浏览器或 Compose，也不据此宣称其完成。运行结果由实现代理和候选 manifest 独立记录。
