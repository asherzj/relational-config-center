CREATE TABLE `query_policy_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `score` bigint NOT NULL,
  `name` varchar(64) NOT NULL,
  `nullable_value` varchar(32) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Complete Query Policy fixture';

INSERT INTO `query_policy_items` (`id`, `score`, `name`, `nullable_value`) VALUES
  (1, 10, 'alpha%_!needle', 'present'),
  (2, 20, 'alphabet', 'present'),
  (3, 30, '', NULL),
  (4, 40, 'omega', '');

CREATE TABLE `query_type_values` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `signed_tiny` tinyint NOT NULL,
  `unsigned_int` int unsigned NOT NULL,
  `signed_big` bigint NOT NULL,
  `unsigned_big` bigint unsigned NOT NULL,
  `decimal_value` decimal(65,30) NOT NULL,
  `float_value` float NOT NULL,
  `double_value` double NOT NULL,
  `char_value` char(8) NOT NULL,
  `varchar_value` varchar(64) NOT NULL,
  `text_value` text NOT NULL,
  `enum_value` enum('alpha','beta') NOT NULL,
  `boolean_value` tinyint(1) NOT NULL,
  `date_value` date NOT NULL,
  `time_value` time(6) NOT NULL,
  `datetime_value` datetime(6) NOT NULL,
  `timestamp_value` timestamp(6) NOT NULL,
  `json_value` json NOT NULL,
  `nullable_value` varchar(32) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Supported live Query types';

INSERT INTO `query_type_values` (
  `id`, `signed_tiny`, `unsigned_int`, `signed_big`, `unsigned_big`,
  `decimal_value`, `float_value`, `double_value`, `char_value`,
  `varchar_value`, `text_value`, `enum_value`, `boolean_value`,
  `date_value`, `time_value`, `datetime_value`, `timestamp_value`,
  `json_value`, `nullable_value`
) VALUES (
  1, -128, 4294967295, -9223372036854775808, 18446744073709551615,
  123456789012345678901234567890.123456789012345678901234567890,
  1.5, 1.5, 'char', 'varchar', 'text', 'alpha', 1,
  '2024-02-29', '23:59:58.123456', '2024-02-29 23:59:58.123456',
  '2024-02-29 23:59:58.123456',
  '{"nested":{"n":9007199254740993},"ok":true}', NULL
);
