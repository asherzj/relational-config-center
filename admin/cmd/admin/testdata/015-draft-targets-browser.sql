CREATE TABLE draft_browser_items (
 id INT AUTO_INCREMENT PRIMARY KEY,
 code VARCHAR(80) NOT NULL,
 label VARCHAR(80) NOT NULL,
 generated_label VARCHAR(80) GENERATED ALWAYS AS (label) STORED,
 `long_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` VARCHAR(80),
 stamp DATETIME
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO draft_browser_items(id,code,label)
SELECT n,CONCAT('key-',n),CONCAT('label-',n)
FROM JSON_TABLE('[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25]','$[*]' COLUMNS(n INT PATH '$')) numbers;
