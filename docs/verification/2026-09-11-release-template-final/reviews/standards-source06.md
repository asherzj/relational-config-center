Standards 增量复核通过：本轮新增硬性违规 0，主观坏味道 0；累计未解决 0。

已核实 source04→source06 仅变化 6 个测试文件。共享 helper 仅由明确的夹具准备调用，通过正式 API 为指定表补缺失关联，保留已有选择；账号准备提前到 SQL 夹具加载前。8 处 Go 调用显式配置测试前提，原业务与异常断言保留，未引入生产兜底。

固定 /tmp/rcc106-review-source06。manifest 46379dc944b475a27a8336f39c2f2eea42ea57b8ee9ab20e56c3c92bac66ccd5；diff 62a332230201e2685477c8adf141cec76b5f5d84b71719db45d9be514c9dcc5e。待跑 MySQL 用例及浏览器后续项仍需补齐执行结果。
