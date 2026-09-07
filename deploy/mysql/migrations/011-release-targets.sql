-- Apply after 010. Reservations have no TTL and are released only by workflow.
CREATE TABLE IF NOT EXISTS rcc_release_targets (
 table_name varbinary(256) NOT NULL,
 record_key binary(32) NOT NULL,
 order_id varbinary(32) NOT NULL,
 PRIMARY KEY (table_name,record_key),
 KEY release_target_order(order_id)
) ENGINE=InnoDB;
