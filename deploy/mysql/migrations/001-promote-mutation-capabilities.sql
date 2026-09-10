-- Apply once to an existing first-iteration Policy Catalog created before
-- allow_add, allow_modify, and allow_delete became Table Policy columns.
-- Current fresh deployments use schema-migrate up with capabilities stored in
-- Mutation Policies; they do not replay this intermediate Table Policy shape.

ALTER TABLE `rcc_table_policies`
  ADD COLUMN `allow_add` tinyint(1) NOT NULL DEFAULT 0 AFTER `mutation_policy_config`,
  ADD COLUMN `allow_modify` tinyint(1) NOT NULL DEFAULT 0 AFTER `allow_add`,
  ADD COLUMN `allow_delete` tinyint(1) NOT NULL DEFAULT 0 AFTER `allow_modify`;

UPDATE `rcc_table_policies`
SET
  `allow_add` = CASE JSON_UNQUOTE(JSON_EXTRACT(`mutation_policy_config`, '$.allow_add'))
    WHEN 'true' THEN 1 ELSE 0 END,
  `allow_modify` = CASE JSON_UNQUOTE(JSON_EXTRACT(`mutation_policy_config`, '$.allow_modify'))
    WHEN 'true' THEN 1 ELSE 0 END,
  `allow_delete` = CASE JSON_UNQUOTE(JSON_EXTRACT(`mutation_policy_config`, '$.allow_delete'))
    WHEN 'true' THEN 1 ELSE 0 END,
  `mutation_policy_config` = JSON_REMOVE(
    `mutation_policy_config`,
    '$.allow_add',
    '$.allow_modify',
    '$.allow_delete'
  );
