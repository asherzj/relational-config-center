package mysql

import "gorm.io/gorm"

// information_schema hides triggers without TRIGGER privilege. Require an
// explicit effective direct grant rather than mistaking invisibility for none.
// Legacy privilege tables use case-insensitive string columns, so apply the
// server's filesystem-name case rules explicitly before accepting a grant.
func directTriggerGrant(database *gorm.DB, table string) (bool, error) {
	var grants int
	err := database.Raw(`SELECT COUNT(*) FROM (
 SELECT GRANTEE,PRIVILEGE_TYPE FROM information_schema.USER_PRIVILEGES WHERE NOT @@global.partial_revokes
 UNION ALL SELECT GRANTEE,PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES
 WHERE IF(@@global.partial_revokes,
   IF(@@lower_case_table_names=0,BINARY TABLE_SCHEMA=BINARY DATABASE(),BINARY LOWER(TABLE_SCHEMA)=BINARY LOWER(DATABASE())),
   IF(@@lower_case_table_names=0,BINARY DATABASE() LIKE BINARY TABLE_SCHEMA ESCAPE X'5C',BINARY LOWER(DATABASE()) LIKE BINARY LOWER(TABLE_SCHEMA) ESCAPE X'5C'))
 UNION ALL SELECT GRANTEE,PRIVILEGE_TYPE FROM information_schema.TABLE_PRIVILEGES
 WHERE IF(@@lower_case_table_names=0,BINARY TABLE_SCHEMA=BINARY DATABASE(),BINARY LOWER(TABLE_SCHEMA)=BINARY LOWER(DATABASE()))
 AND IF(@@lower_case_table_names=0,BINARY TABLE_NAME=BINARY ?,BINARY LOWER(TABLE_NAME)=BINARY LOWER(?))
 ) p WHERE PRIVILEGE_TYPE='TRIGGER' AND BINARY GRANTEE=BINARY CONCAT(
 CHAR(39),LEFT(CURRENT_USER(),CHAR_LENGTH(CURRENT_USER())-CHAR_LENGTH(SUBSTRING_INDEX(CURRENT_USER(),'@',-1))-1),
 CHAR(39),'@',CHAR(39),SUBSTRING_INDEX(CURRENT_USER(),'@',-1),CHAR(39))`, table, table).Row().Scan(&grants)
	return grants > 0, err
}
