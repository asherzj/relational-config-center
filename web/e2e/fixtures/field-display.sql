-- The history display journey publishes real changes without changing shared
-- query/editor seeds or competing with another script's unfinished draft.
CREATE TABLE field_display_browser_items LIKE stage1_acceptance_items;
INSERT INTO field_display_browser_items SELECT * FROM stage1_acceptance_items WHERE id=5;
INSERT INTO rcc_table_policies
  (table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier)
VALUES
  ('field_display_browser_items','notification_page_query_v1','stage1_mutation_v1',1,'browser-acceptance','browser-acceptance');
