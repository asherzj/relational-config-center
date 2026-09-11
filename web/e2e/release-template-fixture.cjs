const assert = require('node:assert/strict');
const {randomUUID} = require('node:crypto');
const {authenticatedRequest} = require('./local-account.cjs');

// Explicit opt-in for named disposable fixtures, never part of application reads.
async function configureFixtureReleaseTemplates(context, origin, tables) {
  const api = async (path, options = {}) => {
    const response = await authenticatedRequest(context, origin, path, options);
    assert.equal(response.status(), 200, `${path}: ${await response.text()}`);
    return response.json();
  };
  const templates = (await api('/api/v1/release-templates')).templates;
  const defaults = ['STANDARD', 'EMERGENCY'].map(type => {
    const template = templates.find(item => item.code === `default_${type.toLowerCase()}_v1`);
    assert.ok(template && template.type === type && template.enabled);
    return template;
  });
  const policies = (await api('/api/v1/table-policies')).policies.filter(policy => tables.includes(policy.table_name));
  const saved = [];
  for (const policy of policies) {
    const path = `/api/v1/table-policies/${encodeURIComponent(policy.table_name)}/release-templates`;
    const existing = (await api(path)).associations;
    for (const template of defaults) {
      if (existing.some(item => item.type === template.type)) continue;
      saved.push(await api(`${path}/${template.type}`, {method:'PUT',
        headers:{'Idempotency-Key':randomUUID()},
        data:{template_code:template.code,enabled:true,expected_version:'0'}}));
    }
  }
  return saved;
}

module.exports = {configureFixtureReleaseTemplates};
