#!/bin/sh
cd /private/tmp/rcc-template-notification-integration/admin
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache go test -tags=integration ./cmd/admin -run '^(TestSchema|TestReleaseTemplateSchema|TestReleaseSchemaStages|TestBaselineGoose|TestApprovalRoleSchema|TestTableApprovalSchema|TestApprovalNotificationsSchema|TestTemplateApprovalSchema|TestTableReleaseSchema|TestAccountUpgradeFromLegacy|TestLegacyApprover)' -count=1 -v -timeout=30m
