# 配置组织与视图：产品概念

本文保留从根 `CONTEXT.md` 迁出的产品概念及展示约定。截至 2026-09-08，当前 Admin/Web 尚未实现这些组织与视图能力；后续实现范围需由具体规格明确。

Configuration Model（配置模型）与 Configuration Record（配置记录）的定义见[项目词汇表](../../CONTEXT.md)。

## 概念与展示约定

**Configuration Domain（配置域）**:
A configuration's domain of belonging from the horizontal platform perspective. Each configuration belongs to exactly one Configuration Domain.
_Avoid_: Business Configuration Set, 业务配置集

**Business Configuration Set（业务配置集）**:
A collection of configurations assembled for a particular vertical business, expressing that business's configuration needs. It may combine configurations from multiple Configuration Domains without changing their domain membership.
_Avoid_: Configuration Group, 配置组, Configuration Domain, 配置域

**Domain View（领域视图）**:
A domain-oriented view showing the configurations in one Configuration Domain, their count, and the relationships among them, supporting understanding of domain architecture and modeling.
Configuration Models and Configuration Records are presented and counted separately.
_Avoid_: Business View, 业务视图, database view

**Business View（业务视图）**:
A business-oriented view showing the Configuration Domains involved in a business and the configurations assembled for it, including domain and configuration counts, supporting understanding of the business's configuration composition.
Configuration Models and Configuration Records are presented and counted separately within that business's scope.
_Avoid_: Domain View, 领域视图, database view
