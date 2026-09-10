CREATE TABLE multitable_browser_a(id INT PRIMARY KEY,label VARCHAR(80)) ENGINE=InnoDB;
CREATE TABLE multitable_browser_b(id INT PRIMARY KEY,label VARCHAR(80)) ENGINE=InnoDB;
CREATE TABLE multitable_browser_large(id INT PRIMARY KEY,payload LONGTEXT) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO multitable_browser_a SELECT n,CONCAT('before-',n) FROM JSON_TABLE('[1,2,3,4,5,6,7,8,9,10,11,12,13]','$[*]' COLUMNS(n INT PATH '$')) numbers;
INSERT INTO multitable_browser_b SELECT * FROM multitable_browser_a;

CREATE TABLE multitable_browser_a_firefox LIKE multitable_browser_a;
INSERT INTO multitable_browser_a_firefox SELECT * FROM multitable_browser_a;

CREATE TABLE multitable_browser_b_firefox LIKE multitable_browser_b;
INSERT INTO multitable_browser_b_firefox SELECT * FROM multitable_browser_b;

CREATE TABLE multitable_browser_large_firefox LIKE multitable_browser_large;

CREATE TABLE multitable_browser_a_webkit LIKE multitable_browser_a;
INSERT INTO multitable_browser_a_webkit SELECT * FROM multitable_browser_a;

CREATE TABLE multitable_browser_b_webkit LIKE multitable_browser_b;
INSERT INTO multitable_browser_b_webkit SELECT * FROM multitable_browser_b;

CREATE TABLE multitable_browser_large_webkit LIKE multitable_browser_large;
