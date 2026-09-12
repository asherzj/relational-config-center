# Historical delayed-notification probe

This probe was temporarily inserted into `web/e2e/release-rollbacks.cjs` and removed before final verification. It did four things:

1. timestamped `pageerror` and failed `/api/v1/approval-notifications` requests;
2. immediately performed `route.fetch()` for the first notification GET after the quick-rollback reload and recorded the real HTTP status;
3. delayed only `route.fulfill({ response })` for five seconds;
4. asserted that this successful request was not reported as `Load request cancelled`.

The red variant retained the original hard `page.goto` to the competing order. The green variant changed only that navigation to the existing release-list link followed by the current order's title link. Both used real Admin, MySQL, accounts, roles, approval, publication, rollback and competition. No response status, body, headers, credential, role, timeout or retry policy changed.

The exact captured event streams are in the two `rollback-navigation-probe.json` files. The source listener, route, delay and diagnostic assertion are absent from the final candidate; the connection preload in this directory is inert unless explicitly passed through `NODE_OPTIONS`.
