CREATE TABLE multitable_browser_a(id INT PRIMARY KEY,label VARCHAR(80)) ENGINE=InnoDB;
CREATE TABLE multitable_browser_b(id INT PRIMARY KEY,label VARCHAR(80)) ENGINE=InnoDB;
CREATE TABLE multitable_browser_large(id INT PRIMARY KEY,payload LONGTEXT) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO multitable_browser_a SELECT n,CONCAT('before-',n) FROM JSON_TABLE('[1,2,3,4,5,6,7,8,9,10,11,12,13]','$[*]' COLUMNS(n INT PATH '$')) numbers;
INSERT INTO multitable_browser_b SELECT * FROM multitable_browser_a;
