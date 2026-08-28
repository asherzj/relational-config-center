CREATE TABLE `notification_templates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `template_key` varchar(100) NOT NULL,
  `channel` enum('EMAIL', 'SMS', 'PUSH') NOT NULL,
  `subject` varchar(200) NULL,
  `body` text NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `priority` int NOT NULL DEFAULT 100,
  `retry_interval_seconds` decimal(8,2) NOT NULL DEFAULT 30.00,
  `active_from` date NULL,
  `delivery_window_start` time NULL,
  `metadata` json NULL,
  `creator` varchar(64) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  PRIMARY KEY (`template_key`),
  UNIQUE KEY `uk_incompatible_notification_id` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
