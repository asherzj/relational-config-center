-- One isolated rollback target per Playwright engine. Each engine can prove the
-- complete forward/reverse history without rewriting another engine's fixture.
CREATE TABLE stage1_rollback_chromium_items LIKE stage1_acceptance_items;
CREATE TABLE stage1_rollback_firefox_items LIKE stage1_acceptance_items;
CREATE TABLE stage1_rollback_webkit_items LIKE stage1_acceptance_items;

INSERT INTO stage1_rollback_chromium_items VALUES
  (1,'Rollback seed',NULL,'engine chromium','active',1,'fixture',NOW(6),'fixture',NOW(6));
INSERT INTO stage1_rollback_firefox_items VALUES
  (1,'Rollback seed',NULL,'engine firefox','active',1,'fixture',NOW(6),'fixture',NOW(6));
INSERT INTO stage1_rollback_webkit_items VALUES
  (1,'Rollback seed',NULL,'engine webkit','active',1,'fixture',NOW(6),'fixture',NOW(6));

INSERT INTO rcc_table_policies
  (table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier)
VALUES
  ('stage1_rollback_chromium_items','notification_page_query_v1','stage1_mutation_v1',1,'browser-acceptance','browser-acceptance'),
  ('stage1_rollback_firefox_items','notification_page_query_v1','stage1_mutation_v1',1,'browser-acceptance','browser-acceptance'),
  ('stage1_rollback_webkit_items','notification_page_query_v1','stage1_mutation_v1',1,'browser-acceptance','browser-acceptance');
