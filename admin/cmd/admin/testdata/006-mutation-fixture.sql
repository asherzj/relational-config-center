CREATE TABLE mutation_add_items (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  code varchar(32) NOT NULL,
  label varchar(64) NOT NULL,
  defaulted_value varchar(64) NOT NULL DEFAULT 'database-default',
  nullable_value varchar(64) NULL,
  lifecycle enum('active', 'paused') NOT NULL DEFAULT 'active',
  quantity int NULL,
  metadata json NULL,
  generated_value varchar(128) GENERATED ALWAYS AS (concat(code, ':generated')) STORED,
  PRIMARY KEY (id),
  UNIQUE KEY uk_mutation_add_items_code (code),
  CONSTRAINT chk_mutation_add_items_label CHECK (label <> 'rollback')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE mutation_auto_fill_items (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  code varchar(32) NOT NULL,
  creator varchar(64) NOT NULL,
  occurred_at datetime(6) NOT NULL,
  status varchar(32) NOT NULL,
  quantity int NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_mutation_auto_fill_items_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE mutation_supplied_id_items (
  id varchar(32) NOT NULL,
  label varchar(64) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE mutation_delete_parents (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  code varchar(32) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_mutation_delete_parents_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE mutation_delete_children (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  parent_id bigint unsigned NOT NULL,
  PRIMARY KEY (id),
  CONSTRAINT fk_mutation_delete_children_parent
    FOREIGN KEY (parent_id) REFERENCES mutation_delete_parents (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO mutation_delete_parents (id, code) VALUES (1, 'delete-rollback');
INSERT INTO mutation_delete_children (parent_id) VALUES (1);
