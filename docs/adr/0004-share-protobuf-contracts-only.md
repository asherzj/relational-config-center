# Share Protobuf contracts only, never Go domain types

Services never share Go domain packages; the only cross-service artifacts are Protobuf IDL files and their generated code, which live in the `shared` module. Each service maps wire types — for example the Query Specification — to its own domain types at its edge, so the same contract legitimately has several service-local definitions. grpc-go and Protobuf dependencies enter the workspace only when the Server iteration starts; the Admin iteration adds none.

## Consequences

- `shared` is reserved for Protobuf contracts and generated code; putting Go domain types there is a boundary violation.
- A future reader will find several Query Specification definitions by design; the alternative — one shared domain package — would couple service evolution and is rejected.
