CREATE TABLE `policy_alpha` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `value` varchar(64) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Policy Alpha';

CREATE TABLE `policy_beta` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `value` varchar(64) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Policy Beta';

CREATE TABLE `policy_incompatible` (
  `code` varchar(64) NOT NULL,
  PRIMARY KEY (`code`)
) ENGINE=InnoDB COMMENT='Incompatible Policy target';

CREATE VIEW `policy_view` AS SELECT `id`, `value` FROM `policy_alpha`;

CREATE TABLE `rcc_policy_shadow` (
  `id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Protected Policy target';
