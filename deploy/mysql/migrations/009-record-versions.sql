-- The empty key is the table maintenance generation floor; 32-byte keys are record identities.
CREATE TABLE IF NOT EXISTS rcc_record_versions (
 table_name VARBINARY(256) NOT NULL,
 record_key VARBINARY(32) NOT NULL,
 lock_version BIGINT UNSIGNED NOT NULL,
 PRIMARY KEY (table_name, record_key)
) ENGINE=InnoDB;
