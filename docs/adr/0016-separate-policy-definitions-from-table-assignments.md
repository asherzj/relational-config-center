---
status: accepted
---

# Separate Policy definitions from table assignments

Query Policies and Mutation Policies are reusable, runtime-managed definitions in the Policy Catalog, while a Table Policy only assigns one Active Policy of each kind to an existing table. This supersedes ADR-0010 and the JSON persistence boundary of ADR-0006: per-table JSON overrides are removed, Policy behavior is versioned behind immutable domain-level Codes, and changing behavior requires creating and activating a new Policy rather than modifying an Active one.

## Model

`rcc_query_policies` stores `id`, immutable `code`, display `name` and `description`, code-owned `type_code`, `default_order_field`, `default_order_direction`, `default_page_size`, `max_page_size`, lifecycle `status`, and audit columns. Query scalar rules stay in this table because the current `page_query` Type has exactly one value for each and Admin reads the complete definition.

`rcc_mutation_policies` stores the same identity, Type, lifecycle, and audit fields plus `allow_add`, `allow_modify`, `allow_delete`, `create_operator_field`, `create_time_field`, `modify_operator_field`, and `modify_time_field`. Operation authorization belongs to the reusable Mutation Policy, not the Table Policy. Standard Auto Fill is deliberately restricted to these nullable scalar field slots: ADD fills configured Create and Modify fields, MODIFY fills configured Modify fields, Operator fields use the current Operator, Time fields use database time, DELETE does not fill, and client input for server-managed fields is rejected. Arbitrary sources, literal values, JSON rules, and an Auto Fill child table are outside this model.

`rcc_table_policies` stores only `id`, immutable unique `table_name`, `query_policy_code`, `mutation_policy_code`, `enabled`, and audit columns. Both Codes are required; no per-table Policy field can override its referenced definitions. New assignments are disabled, replacement is fully validated and atomic, replacing an enabled assignment keeps it enabled and affects subsequent requests immediately, and the first iteration permits disable but not hard deletion.

The model does not contain `query_policy_config`, `mutation_policy_config`, per-table `allow_*`, `supports_*`, Policy configuration JSON, Auto Fill rule tables, history revisions, or optimistic-lock columns.

## Lifecycle and management

Query Policies and Mutation Policies follow `DRAFT -> ACTIVE -> DEPRECATED`. A Draft may be fully replaced or deleted but cannot be assigned; its Code is immutable from creation. An Active Policy is assignable and its execution semantics are immutable. A Deprecated Policy remains executable for existing assignments but cannot receive new assignments. Active and Deprecated Policies may only change display name and description and cannot be hard deleted or reactivated.

Codes are unique within their Query or Mutation namespace, use versioned domain language such as `standard_page_query_v1`, and must not expose a database implementation such as `mysql_`. Platform administrators manage definitions through dedicated protected APIs; the generic Managed Table capability cannot access any `rcc_*` catalog table. The management UI separates Query/Mutation Policy definition management from Table Policy assignment management.

`type_code` selects a code-owned, explicitly registered execution contract such as `page_query` or `single_table_mutation`; the database is authoritative for concrete Policy rules, while the Type registry is authoritative for executable capabilities. Unknown Types fail closed. All registered Mutation Types implement ADD, MODIFY, and DELETE; the selected Mutation Policy's `allow_*` values are the sole operation authorization source.

## Integrity and execution

The database enforces unique Codes, unique `table_name`, scalar `CHECK` constraints, and lookup indexes, but deliberately does not create foreign keys between Table Policy Codes and Policy definitions. Dedicated application services must reject missing Codes, Draft references, new Deprecated references, unknown Types, invalid lifecycle transitions, and definitions incompatible with the live table Schema. Existing Deprecated references remain valid. Runtime lookup failure always denies execution.

Each request reads the Table Policy, Query Policy, and Mutation Policy separately inside one `REPEATABLE READ` transaction rather than using a join or Policy cache. Query uses a read-only transaction; Mutation uses a read-write transaction that also contains the Managed Table change. External concurrent DDL cannot be frozen by this model and must fail safely if it races with live Schema validation.

Configurable page values cannot exceed code-owned platform safety limits for page size, conditions, membership values, offset, or request size. The first iteration accepts last-write-wins administration and defers optimistic concurrency control; a later concurrency mechanism must not be confused with historical Policy revisioning.

## Consequences

- Table Policy administration becomes simple selection and enablement; no strategy JSON is entered at assignment time.
- Reusable Policy definitions prevent per-table drift but require a new immutable Code for every behavior change.
- Keeping Query scalars and fixed Mutation audit slots in their respective main tables avoids one-to-one settings and child-rule tables, at the cost of supporting only the explicitly modeled Policy Types and standard Auto Fill shape.
- Omitting foreign keys and optimistic locking keeps the first implementation smaller but makes application validation, transactional lifecycle changes, fail-closed execution, and future concurrency work mandatory rather than optional.
