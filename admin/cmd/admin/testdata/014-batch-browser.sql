-- Isolated browser acceptance data; policies are registered through public HTTP.
CREATE TABLE batch_browser_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  code VARCHAR(32) NOT NULL,
  label VARCHAR(64) NOT NULL,
  UNIQUE KEY uk_batch_browser_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO batch_browser_items(id,code,label) VALUES
  (1,'seed-one','original one'),
  (2,'seed-two','original two'),
  (3,'seed-three','original three');
