-- Stop old writers before upgrade. This migration is restartable and does not touch business rows.
CREATE TABLE IF NOT EXISTS rcc_release_orders (
 id varbinary(32) NOT NULL PRIMARY KEY,
 table_name varbinary(256) NOT NULL,
 applicant_id varbinary(36) NOT NULL,
 state varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version bigint unsigned NOT NULL,
 document json NOT NULL,
 KEY release_table(table_name,id),
 KEY release_applicant(applicant_id,id),
 KEY release_state(state,id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS rcc_release_requests (
 actor_id varbinary(36) NOT NULL,
 operation varbinary(96) NOT NULL,
 request_key varbinary(64) NOT NULL,
 digest binary(32) NOT NULL,
 result json NULL,
 PRIMARY KEY(actor_id,operation,request_key)
) ENGINE=InnoDB;
