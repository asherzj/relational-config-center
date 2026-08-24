CREATE TABLE `query_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `category` varchar(32) NOT NULL,
  `label` varchar(64) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Queryable items';

INSERT INTO `query_items` (`id`, `category`, `label`) VALUES
  (1, 'alpha', 'first'),
  (2, 'beta', 'second'),
  (3, 'alpha', 'third'),
  (4, 'alpha', NULL);
