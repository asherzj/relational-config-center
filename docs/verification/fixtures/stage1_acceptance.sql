-- Stage 1 browser acceptance fixture.
-- Run only in an isolated database; this resets the table of the same name.

DROP TABLE IF EXISTS stage1_acceptance_items;

CREATE TABLE stage1_acceptance_items (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  name varchar(64) NOT NULL,
  category varchar(32) NULL,
  note varchar(255) NULL,
  state enum('draft', 'active', 'paused') NOT NULL DEFAULT 'draft',
  priority int NOT NULL DEFAULT 0,
  created_by varchar(64) NOT NULL,
  created_at datetime(6) NOT NULL,
  updated_by varchar(64) NOT NULL,
  updated_at datetime(6) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_0900_ai_ci
  COMMENT='Stage 1 isolated acceptance items';

SET @stage1_fixture_time = NOW(6);

INSERT INTO stage1_acceptance_items (
  name,
  category,
  note,
  state,
  priority,
  created_by,
  created_at,
  updated_by,
  updated_at
) VALUES
  ('Alpha', 'notice', NULL, 'active', 10, 'fixture', @stage1_fixture_time, 'fixture', @stage1_fixture_time),
  ('Beta', 'alert', '', 'paused', 20, 'fixture', @stage1_fixture_time, 'fixture', @stage1_fixture_time),
  ('Gamma', 'notice', 'contains 100% literal', 'draft', 30, 'fixture', @stage1_fixture_time, 'fixture', @stage1_fixture_time),
  ('Delta', NULL, 'range candidate', 'active', 40, 'fixture', @stage1_fixture_time, 'fixture', @stage1_fixture_time),
  ('Epsilon', 'digest', 'final seed', 'draft', 50, 'fixture', @stage1_fixture_time, 'fixture', @stage1_fixture_time);
