# Frozen pre-Goose fixtures

`pre-goose-8b5cd859.sql` preserves the released control structure at main
`8b5cd8592f0fbf02dcd27bb72a8a306fa371993d`. It is used only to represent an
unmanaged historical database and as an independent physical-schema expectation.
It is never a current installation path and must not evolve with new migrations.

`pre-goose-8b5cd859-data.json` contains disposable data produced by T2 commit
`123365b8498141b830771e14325eb1fff4eb84e1` through its public account, independent
approval and publication workflow. Export used a temporary Go overlay without
modifying the delivered dependency source. SQL values are base64-encoded to
preserve binary record identities and JSON bytes; SQL NULL stays null. Generated
columns are omitted from insert inputs, SQL DATETIME values use MySQL literals,
and the expected public publication response remains JSON. At restore time only
Login Session expiry and last-active time move relative to the test clock, before preservation
snapshots. All IDs, roles, hashes, sessions, Policy data, version floors,
publication commands and history retain their frozen values.

Current test setup uses `startCurrentIntegrationMySQL` and Goose before loading
business fixtures. `startIntegrationMySQL` creates an empty database or explicitly
named historical SQL state for migration tests. Historical numbered upgrade SQL
retains its separate responsibilities under `deploy/mysql/migrations/README.md`.

After integration with main 4aeb54e, historical baseline tests explicitly apply
`deploy/mysql/migrations/014-table-field-policies.sql` after this unchanged SQL
snapshot. The composed historical structure independently checks Goose 00003;
the frozen files themselves must remain unchanged. The adoption test additionally
seeds a field-policy row before taking preservation snapshots.
