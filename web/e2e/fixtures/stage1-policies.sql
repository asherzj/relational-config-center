-- Browser acceptance policy state. Run only in the disposable E2E database.

INSERT INTO `rcc_query_policies` (
  `code`, `name`, `description`, `type_code`, `default_order_field`,
  `default_order_direction`, `default_page_size`, `max_page_size`, `status`,
  `creator`, `modifier`
) VALUES (
  'stage1_query_v1',
  'Stage 1 query',
  'Disposable browser acceptance query policy',
  'page_query',
  'id',
  'DESC',
  20,
  200,
  'ACTIVE',
  'browser-acceptance',
  'browser-acceptance'
);

INSERT INTO `rcc_mutation_policies` (
  `code`, `name`, `description`, `type_code`,
  `allow_add`, `allow_modify`, `allow_delete`,
  `create_operator_field`, `create_time_field`,
  `modify_operator_field`, `modify_time_field`,
  `status`, `creator`, `modifier`
) VALUES (
  'stage1_mutation_v1',
  'Stage 1 mutation',
  'Disposable browser acceptance mutation policy',
  'single_table_mutation',
  1,
  1,
  1,
  'created_by',
  'created_at',
  'updated_by',
  'updated_at',
  'DEPRECATED',
  'browser-acceptance',
  'browser-acceptance'
);

INSERT INTO `rcc_table_policies` (
  `table_name`, `query_policy_code`, `mutation_policy_code`, `enabled`,
  `creator`, `modifier`
) VALUES (
  'stage1_acceptance_items',
  'notification_page_query_v1',
  'stage1_mutation_v1',
  1,
  'browser-acceptance',
  'browser-acceptance'
);
