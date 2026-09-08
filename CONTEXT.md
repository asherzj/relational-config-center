# Relational Configuration Center

Relational Configuration Center is a platform for publishing and distributing relational business configuration. It treats configuration as relational data with explicit schemas, constraints, references, and query rules rather than as independent key-value pairs or opaque documents.

This glossary defines project-wide language. Admin-specific terms are defined in [Admin](./admin/CONTEXT.md); planned configuration organization and views are recorded in [the product design note](./docs/design-notes/configuration-organization-and-views.md).

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

**Admin**:
The management-plane backend that owns configuration authoring and exposes HTTP APIs to Web.
_Avoid_: admin frontend, Web

**Web**:
The browser frontend used to interact with Admin.
_Avoid_: Admin

**Server**:
The data-plane backend that serves configuration to runtime consumers.
_Avoid_: Admin, Gateway
