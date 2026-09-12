# Release template Web verification

Verification date: 2026-09-11

Source hashes:

- `web/src/features/release-templates/ReleaseTemplatesPage.tsx`: `e93055f0ba3138566b7c180c9e6eb5531d336f3956d2b58dedca19ca8c1bc239`
- `web/src/features/release-templates/ReleaseTemplatesPage.test.tsx`: `fb158deeeb02e7db643c74651c308694d2897e8621811a4df155e3a6f568ea42`

Commands run from `web/`:

```text
pnpm exec vitest run src/features/release-templates/ReleaseTemplatesPage.test.tsx
PASS: 1 file, 7 tests, 3.92s

pnpm exec tsc --noEmit
PASS: exit 0

pnpm run build
PASS: 2145 modules transformed, production bundle built

pnpm exec vitest run src/features/accounts/WorkspaceAccess.test.tsx
PASS: 1 file, 15 tests, 4.24s
```

Raw output is retained in `component.log`, `typecheck.log`, `build.log`, and `workspace-access.log` in this directory.

The component scenarios cover constrained creation, exact retry after an unknown write result, stale-version input retention, destructive keyboard confirmation, binding an unknown lifecycle request to its original action and target, retaining the request ID, and clearing a failed operation when moving to another template.

The sibling `browser/pre-final-source/` directory retains the earlier successful browser run. `browser/final-source/` is the complete rerun against the source hashes above and includes the raw Go/testcontainers/Chromium log, result JSON, and screenshots.
