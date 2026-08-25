# Policy Catalog migrations

The reusable Policy model uses an expand-contract rollout. Apply migrations
001 through 005 in order, deploy the transactional Policy Snapshot Admin, then
run the contraction command below. A fresh installation uses the already-final
`mysql/init/001-schema.sql` and does not replay these legacy migrations.

## Table Policy Code expansion

After `005-expand-table-policy-code-references.sql`, run the set-wide preflight
and backfill from the `admin` module:

```bash
go run ./cmd/policy-migrate
```

The command uses the same `MYSQL_*` and `ADMIN_OPERATOR` environment settings as
Admin. It first locks and backfills the legacy set only when every row is
representable. It then repeats a contraction preflight that requires both
non-null, valid technology-neutral Codes, existing executable
Active/Deprecated definitions of registered Types, valid relational scalar
rules, and representable legacy configuration. A failed preflight prints sorted
table-scoped diagnostics and drops no column. On success it applies the final
single Table Policy ALTER.

Representable legacy behavior is normalized as follows:

- omitted page-query values become `id DESC`, default page size `20`, and max
  page size `200`, matching the legacy constructor;
- explicit valid page-query scalars are preserved;
- `allow_add`, `allow_modify`, and `allow_delete` move into the reusable
  Mutation Policy;
- an ADD-only `operator`/`now` target maps to a Create operator/time slot;
- a MODIFY `operator`/`now` target must also appear identically in ADD and maps
  to a Modify operator/time slot, preserving ADD and MODIFY behavior.

Policy Codes are deterministic, technology-neutral hashes of normalized
semantics and end in `_v1`, so identical legacy rules reuse one Active Policy.

The preflight rejects unknown strategy/configuration fields, invalid query
scalars, literal or unknown Auto Fill sources, multiple targets for one fixed
slot, non-mirrored MODIFY rules, and partial Code backfills. Diagnostics include
the affected `table_name`. On failure the transaction rolls back every Policy
insert and Table Policy update; the nullable expand columns and indexes remain
available for a corrected rerun.

## Standalone contract migration

[`006-contract-legacy-table-policy-columns.sql`](./006-contract-legacy-table-policy-columns.sql)
is provided for DBA-controlled migration runners. Its stored-procedure guard
rechecks Codes, definition existence/lifecycle/registered Types, relational
scalars, and legacy Query/Mutation/Auto Fill representability before reaching
its only destructive `ALTER TABLE`. Any assertion raises `SQLSTATE 45000`; all
legacy columns remain intact. Prefer the Go command because its diagnostics are
more precise, but direct execution of 006 is independently fail closed.

The final `rcc_table_policies` columns are exactly `id`, `table_name`, both
Policy Codes, `enabled`, `creator`, `modifier`, `gmt_created`, and
`gmt_modified`. No foreign keys or optimistic-lock columns are added.
