const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { authenticatedRequest } = require('./local-account.cjs');

async function approvalFixtureRequest(context, base, method, path, data, status = 200) {
  const response = await authenticatedRequest(context, base, path, { method, data, headers: { 'Idempotency-Key': randomUUID() } });
  assert.equal(response.status(), status, `${method} ${path}: ${await response.text()}`);
  return response.json();
}
async function bindFixtureApprovalRoles(context, base, table, roleIDs) {
  const path = `/api/v1/table-policies/${encodeURIComponent(table)}/approval-roles`;
  const current = await approvalFixtureRequest(context, base, 'GET', path);
  return approvalFixtureRequest(context, base, 'PUT', path, { expected_version: current.version, role_ids: roleIDs });
}
async function createFixtureApprovalRole(context, base, name, memberIDs, tables = []) {
  const role = await approvalFixtureRequest(context, base, 'POST', '/api/v1/approval-roles', { name, description: '隔离验收角色', enabled: true, member_ids: memberIDs }, 201);
  for (const table of tables) await bindFixtureApprovalRoles(context, base, table, [role.id]);
  return role;
}
// Read as the actual reviewer when preparing a new request. The caller keeps
// its reviewed order version and reason; retries must reuse the resulting body.
async function fixtureApprovalInput(context, base, orderID, input) {
  const order = await approvalFixtureRequest(context, base, 'GET', `/api/v1/release-orders/${orderID}`);
  assert.equal(typeof order.approval_context?.revision, 'string', 'reviewer response omitted the approval revision');
  assert.ok(Array.isArray(order.approval_context.approvable_tables), 'reviewer response omitted the approval scope');
  return {
    ...input,
    confirmed_tables: [...order.approval_context.approvable_tables],
    expected_approval_revision: order.approval_context.revision,
  };
}
module.exports = { approvalFixtureRequest, bindFixtureApprovalRoles, createFixtureApprovalRole, fixtureApprovalInput };
