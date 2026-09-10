package application

import (
	"errors"
	"testing"
)

func TestReleaseTemplateManagementAcceptsOnlyExecutableNodeSequences(t *testing.T) {
	tests := []struct {
		name      string
		typeCode  string
		nodes     []PutReleaseTemplateNode
		wantError error
	}{
		{name: "standard", typeCode: "STANDARD", nodes: standardTemplateNodes()},
		{name: "emergency", typeCode: "EMERGENCY", nodes: emergencyTemplateNodes()},
		{name: "standard cannot omit approval", typeCode: "STANDARD", nodes: emergencyTemplateNodes(), wantError: ErrInvalidReleaseTemplateNodes},
		{name: "emergency cannot include approval", typeCode: "EMERGENCY", nodes: standardTemplateNodes(), wantError: ErrInvalidReleaseTemplateNodes},
		{name: "unknown type", typeCode: "CANARY", nodes: emergencyTemplateNodes(), wantError: ErrUnknownReleaseType},
		{name: "duplicate node code", typeCode: "EMERGENCY", nodes: []PutReleaseTemplateNode{{Code: "step", Type: "PUBLICATION", Name: "发布", RequiredRole: "PUBLISHER"}, {Code: "step", Type: "COMPLETION", Name: "完结", RequiredRole: "PUBLISHER"}}, wantError: ErrInvalidReleaseTemplateNodes},
		{name: "invalid permission", typeCode: "STANDARD", nodes: []PutReleaseTemplateNode{{Code: "approval", Type: "APPROVAL", Name: "审批", RequiredRole: "APPROVER"}, {Code: "publication", Type: "PUBLICATION", Name: "发布", RequiredRole: "PUBLISHER"}, {Code: "completion", Type: "COMPLETION", Name: "完结", RequiredRole: "PUBLISHER"}}, wantError: ErrInvalidReleaseTemplateNodes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			template, err := validatedReleaseTemplate(PutReleaseTemplate{Code: "release_flow_v1", Name: "发布流程", Type: test.typeCode, Nodes: test.nodes})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("validation error = %v, want %v", err, test.wantError)
			}
			if test.wantError == nil && len(template.MonitorList) != 0 {
				t.Fatalf("monitor_list must remain empty: %#v", template.MonitorList)
			}
		})
	}
}

func standardTemplateNodes() []PutReleaseTemplateNode {
	return []PutReleaseTemplateNode{
		{Code: "approval", Type: "APPROVAL", Name: "按表审批", RequiredRole: "TABLE_APPROVER"},
		{Code: "publication", Type: "PUBLICATION", Name: "发布", RequiredRole: "PUBLISHER"},
		{Code: "completion", Type: "COMPLETION", Name: "完结", RequiredRole: "PUBLISHER"},
	}
}

func emergencyTemplateNodes() []PutReleaseTemplateNode {
	return []PutReleaseTemplateNode{
		{Code: "publication", Type: "PUBLICATION", Name: "应急发布", RequiredRole: "PUBLISHER"},
		{Code: "completion", Type: "COMPLETION", Name: "完结", RequiredRole: "PUBLISHER"},
	}
}
