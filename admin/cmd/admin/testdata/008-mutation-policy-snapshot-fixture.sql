CREATE TABLE mutation_snapshot_items (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  code varchar(64) NOT NULL,
  label varchar(64) NOT NULL,
  creator varchar(64) NOT NULL,
  created_at datetime(6) NOT NULL,
  modifier varchar(64) NOT NULL,
  updated_at datetime(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_mutation_snapshot_items_code (code),
  CONSTRAINT chk_mutation_snapshot_items_label CHECK (label <> 'rollback')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
