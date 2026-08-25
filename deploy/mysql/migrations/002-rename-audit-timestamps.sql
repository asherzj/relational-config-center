-- Historical expand-stage bridge: the then-current Admin expected created_at
-- and updated_at. Migration 006 renames them back to the final ADR-0016 audit
-- names. Fresh deployments already receive gmt_created and gmt_modified from
-- mysql/init/001-schema.sql and do not replay this legacy sequence.

ALTER TABLE `rcc_table_policies`
  RENAME COLUMN `gmt_created` TO `created_at`,
  RENAME COLUMN `gmt_modified` TO `updated_at`;
