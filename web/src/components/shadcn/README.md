# shadcn/ui primitives

Generated with the official shadcn CLI 4.21.0 using `new-york` / `neutral` on 2026-09-07. Sources: https://ui.shadcn.com/docs and https://github.com/shadcn-ui/ui (MIT).

These files are owned and maintained by this project. Keep the following deliberate adaptations when updating:

- `dialog.tsx`, `sheet.tsx`: optional inline rendering for workspace-owned surfaces; Chinese close labels. Business wrappers retain the existing guarded focus and dismissal behavior during authentication interruption.
- `table.tsx`: compact metadata headers, readable cell padding, wrapping for long values.
- `native-select.tsx`: full-width wrapper for labeled fields.
- `sonner.tsx`: the app's light theme without an unused theme provider.

Shared product compositions live in `../ui/`; the design contract is `web/DESIGN.md`.
