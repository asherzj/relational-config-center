-- Historical expand-stage bridge: the then-current Admin expected created_at
-- and updated_at. Contraction later used gmt_created/gmt_modified; migration 013
-- establishes the current created_at/updated_at names. Fresh deployments use
-- schema-migrate up and do not replay this legacy sequence.

ALTER TABLE `rcc_table_policies`
  RENAME COLUMN `gmt_created` TO `created_at`,
  RENAME COLUMN `gmt_modified` TO `updated_at`;
