# Relational Configuration Center

Relational Configuration Center is a platform for publishing and distributing relational business configuration. It treats configuration as relational data with explicit schemas, constraints, references, and query rules rather than as independent key-value pairs or opaque documents.

## Language

**Relational Business Configuration**:
Configuration whose meaning and validity are modeled through entities, fields, constraints, references, and relations.
_Avoid_: key-value configuration, configuration file, arbitrary business data

**Configuration Model（配置模型）**:
The definition of a kind of business configuration, describing its fields, constraints, and relationships. For example, channel configuration is one Configuration Model.
_Avoid_: Configuration Record, 配置记录

**Configuration Record（配置记录）**:
A concrete instance of a Configuration Model, such as the WeChat Pay channel under the channel configuration model.
_Avoid_: Configuration Model, 配置模型

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

**Managed Table**:
A relational table exposed through Admin because it has an active Table Policy.
_Avoid_: arbitrary table, raw table

**Table Policy（表规则）**:
Runtime configuration maintained by a user after deployment that defines how a Managed Table may be queried or changed, including visible fields, permitted operators, sorting, and mutations. Changing a Table Policy does not require rebuilding or restarting Admin.
_Avoid_: table config, database permission, 表策略

**Policy Catalog（规则目录）**:
The built-in collection of Table Policies managed through dedicated Admin capabilities. It is not a Managed Table and cannot be queried or changed through the generic table API.
_Avoid_: policy table resource, self-managed table, 策略目录

**Query Specification**:
A client-supplied, database-independent description of filters, sorting, and pagination that must be accepted by a Table Policy before execution.
_Avoid_: SQL, query string

**Admin**:
The management-plane backend that owns configuration authoring and exposes HTTP APIs to Web.
_Avoid_: admin frontend, Web

**Web**:
The browser frontend used to interact with Admin.
_Avoid_: Admin

**Server**:
The data-plane backend that serves configuration to runtime consumers.
_Avoid_: Admin, Gateway
