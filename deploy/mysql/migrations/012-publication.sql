-- Apply after 011. These immutable records describe this Admin single data source.
CREATE TABLE IF NOT EXISTS rcc_table_publications (
 table_name varbinary(256) NOT NULL,
 table_version bigint unsigned NOT NULL,
 command_cursor bigint unsigned NOT NULL,
 PRIMARY KEY(table_name)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS rcc_publication_commands (
 table_name varbinary(256) NOT NULL,
 sequence bigint unsigned NOT NULL,
 order_id varbinary(32) NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(table_name,sequence),
 KEY publication_order(order_id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS rcc_refresh_notifications (
 order_id varbinary(32) NOT NULL,
 table_name varbinary(256) NOT NULL,
 table_version bigint unsigned NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(order_id)
) ENGINE=InnoDB;
