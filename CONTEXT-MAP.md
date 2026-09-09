# Context Map

Project-wide language is defined in the [root glossary](./CONTEXT.md).

## Contexts

- [Admin](./admin/CONTEXT.md): governs which existing relational configuration tables may be managed and how their rows may be queried or changed
- **Server**: serves configuration data to runtime consumers; it is outside the first Admin iteration
- **Client**: consumes configuration data from Server; its domain design is deferred until after the first Admin iteration

## Relationships

- **Admin → Managed Data Source**: Admin reads and writes policies and managed data through the single database selected by deployment configuration
- **Server → Managed Data Source**: Server will read configuration data with read-only database authority in a later iteration
- **Server → Client**: Server will provide Client with configuration-reading capabilities in a later iteration
- **Admin / Server / Client**: each context owns its domain language and model; transport contracts do not constitute a shared domain model
