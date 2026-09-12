import json
from pathlib import Path
import re

root = Path('/private/tmp/rcc-issue-106-release-template-final/docs/verification/2026-09-11-release-template-final')
index = json.loads((root / 'ac-index.json').read_text())
results = json.loads((root / 'integration-results.json').read_text())
patterns = {
 1: r'TestReleaseTemplateHTTP(RejectsNonAdministratorManagement|ChecksCurrentAuthorizationBeforeReplayingAllWrites)',
 2: r'TestReleaseTemplateHTTPMaintainsMultipleTypesWithIdempotencyAndVersions',
 3: r'TestReleaseTemplateHTTPProtectsEmergencyAndRejectsInvalidNodes',
 4: r'TestReleaseTemplateHTTP(ProtectsEmergencyAndRejectsInvalidNodes|StandardDeletionKeepsReadOnlyReadiness)',
 5: r'TestReleaseTemplateHTTP(ConcurrentRequestsRetainOneResultAndRejectStaleAdministrator|ReplaysCommittedReplacementAfterLaterChanges|RequiresRequestIdentityAndRejectsChangedTargets)',
 6: r'TestTableReleaseTemplateHTTP(AC006AndAC008SelectShareProtectAndDisable|RejectsMissingTableAndNonAdmin)',
 7: r'Test(TableReleaseTemplateHTTPAC007AtomicCreateEnableAndRequestResultFailure|TablePolicyHTTPConcurrentCreateAndEnablePreserveEmergency|TableReleaseSchemaMigratesExistingTablesAndPreservesSelections)',
 8: r'TestTableReleaseTemplateHTTP(AC006AndAC008SelectShareProtectAndDisable|ConcurrentSwitchAndOriginalReplay)',
 9: r'TestReleaseFlowAC009PersistsIndependentTablesBeforeFirstRead',
 10: r'TestReleaseFlowAC010MissingConfigurationRequiresExplicitSave',
 11: r'TestReleaseFlowAC011DraftEditsPreserveAndReplaceOnlyParticipatingTables',
 12: r'TestReleaseEmergencyAC012.*',
 13: r'TestReleaseFlowAC013DerivedDraftUsesCurrentConfiguration',
 14: r'Test(ReleaseFlowAC014SeparatesInstancesFromSubmissionApprovals|LegacyApproverFormalCutover|LegacyApproverCurrentInputRejectsRetiredGrants|LegacyApproverCutoverPreservesLiveResponsibilityAndCompletedApproval)',
 15: r'TestReleaseEmergency(AC015SubmitNeedsReasonAndDoesNotPublish|ReasonOnlyAcceptedBySubmit)',
 16: r'TestRelease(EmergencyAC016PublisherPermissionIsCurrent|FlowCurrentAuthorizationAfterWaitingForMaintenance)',
 17: r'TestRelease(EmergencyAC017AC018WholeOrderExecutionAndCompletion|MixedBatchPublication)',
 18: r'TestRelease(EmergencyAC017AC018WholeOrderExecutionAndCompletion|CompletionPermanentlyClosesRollbackWindow|CompletionReleasesWithoutRepublishing|CompletionRolesConcurrencyAndReplay)',
 19: r'TestRollbackFlow(AC019PersistsCurrentEmergencyInstancesBeforeDisplay|PreviewDoesNotExtendRecoveryOrFreezeBusinessValues)',
 20: r'Test(RollbackFlowAC020EndsOriginalAndRecordsOnlyTrueExecution|QuickRollbackCompetesWithCompletionAndOtherRollbacks|ReleaseCompletionPermanentlyClosesRollbackWindow)',
 21: r'Test(RollbackFlowAC021EmergencyCancellationUsesRealSubmission|ReleaseEmergencyPendingConsumersAndDerivedDrafts|ReleaseNotifications.*)',
 22: r'TestReleaseFlowAC022ConcurrentTemplateAndAssociationSnapshots',
 23: r'Test(ReleaseFlowStorageFailuresKeepDraftAndOriginalResultAtomic|ReleaseEmergencyAC023SwitchFailureAndOriginalResults|RollbackFlowPreviewStorageFailuresKeepOriginalAtomic|ReleaseTemplateHTTPResultStorageFailureRollsBackEveryWrite)',
 24: r'Test(QuickRollbackPersistenceFailuresPreserveValuesVersionsHistoryAndTargets|QuickRollbackConstraintFailureRollsBackEarlierItems|QuickRollbackCompetesWithCompletionAndOtherRollbacks|RollbackFlowPreviewStorageFailuresKeepOriginalAtomic)',
 25: r'TestReleaseWorkflowPublicRoutes',
 26: r'Test(Schema.*|ReleaseTemplateSchemaMigrationUpgradesVersionFive|TableReleaseSchema.*|TemplateApprovalSchema.*)',
 27: r'Test(LegacyApproverFormalCutover|LegacyApproverCurrentInputRejectsRetiredGrants|ReleaseWorkflowPublicRoutes|ReleaseFlowAC010MissingConfigurationRequiresExplicitSave|RollbackFlowAC020EndsOriginalAndRecordsOnlyTrueExecution|ReleaseCompletionPermanentlyClosesRollbackWindow)',
}
browser = {
 **{n: ['TestReleaseTemplateBrowserSystemPath'] for n in range(1, 6)},
 **{n: ['TestTableReleaseTemplateBrowserSystemPath'] for n in range(6, 9)},
 **{n: ['TestReleaseFlowBrowserSystemPath'] for n in [9, 10, 11, 13, 14]},
 **{n: ['TestReleaseEmergencyBrowserSystemPath'] for n in [12, 15, 16, 17, 18]},
 **{n: ['TestRollbackFlowBrowserSystemPath'] for n in [19, 20, 24]},
 21: ['TestApprovalNotificationsBrowserSystemPath', 'TestReleaseNotificationsBrowserSystemPath', 'TestRollbackFlowBrowserSystemPath'],
 22: [], 23: ['TestReleaseFlowBrowserSystemPath', 'TestReleaseEmergencyBrowserSystemPath', 'TestRollbackFlowBrowserSystemPath'],
 25: json.loads(Path('/tmp/rcc-106-root-required-browser-matrix.json').read_text())['go_browser_targets'],
 26: ['TestReleaseTemplateBrowserSystemPath', 'TestTableReleaseTemplateBrowserSystemPath'],
 27: ['TestApprovalFinalBrowserSystemPath', 'TestRollbackFlowBrowserSystemPath', 'TestReleaseFlowBrowserSystemPath'],
}
for row in index['rows']:
    number = int(row['ac'].split('-')[1])
    matches = [r for r in results['rows'] if re.fullmatch(patterns[number], r['test'])]
    assert matches, number
    row['current_integration'] = [{'package': r['package'], 'test': r['test'], 'effective': r['effective']} for r in matches]
    row['current_browser_targets'] = browser[number]
    row['applicability'] = 'Original primary delivery remains immutable. Current product contracts are retained; this ticket reruns the named public/real-MySQL tests and relevant browser paths. Unchanged detailed module evidence remains attributable to the primary issue, alongside this run of all Go/Web module tests. The final effective execution status is authoritative, never the original issue status alone.'
    row['state'] = 'implementation acceptance passed; root delivery gate pending' if results['summary'] == {'PASS': results['expected_test_count']} else 'awaiting final combined result audit'
    if number == 25:
        row['additional_evidence'] = ['raw/terminal-phase-red.log', 'raw/terminal-phase-green-02.log', 'raw/web-failure-recheck.log', 'browser-matrix.json', 'visual-inspection.json', 'go-browser/TestReleaseFlowBrowserSystemPath-02/release-instances-browser-evidence.json']
    elif number == 26:
        row['additional_evidence'] = ['inherited-boundary-check.json', 'browser-runner-integrity.json', 'compose-migrations-02/results.json']
        row['scope_note'] = 'Formal current11 new-install/upgrade/read-only/partial-recovery checks are separate from the historical Contract+016 equivalence at prefix5; immutable1..11 SQL/manifests are retained.'
    elif number == 27:
        row['additional_evidence'] = ['browser-matrix.json', 'integration-results.json', 'inherited-boundary-check.json', 'reviews/']
        row['project_record'] = 'Root owns the post-gate commit/push/remote verification and Notion overall update before announcing feature completion or closing106. Parent100 remains open.'
index['purpose'] = 'Per-AC final applicability working index; effective statuses and pending delivery gate remain explicit.'
(root / 'ac-index.json').write_text(json.dumps(index, ensure_ascii=False, indent=2) + '\n')
print('updated', len(index['rows']), 'AC rows')
