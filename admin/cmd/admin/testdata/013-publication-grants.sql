-- Explicit publication metadata capability for the test deployment. Production
-- grants are managed by the deployment owner, never by the control schema.
GRANT PROCESS ON *.* TO 'rcc_admin'@'%';
