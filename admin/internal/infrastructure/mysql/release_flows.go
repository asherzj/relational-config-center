package mysql

import (
	"context"
	"encoding/json"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// A joined consistent read shares the draft transaction's REPEATABLE READ
// snapshot. Concurrent configuration writes cannot mix association and template
// versions, nor can later tables observe a different database snapshot.
func (session *releaseOrderSession) ReadReleaseFlowConfigurations(ctx context.Context, tables []string, kind domain.ReleaseType) ([]domain.ReleaseFlowConfiguration, error) {
	if err := session.available(); err != nil {
		return nil, err
	}
	result := []domain.ReleaseFlowConfiguration{}
	if len(tables) == 0 {
		return result, nil
	}
	var rows []struct {
		TableName, Code, Name       string
		Type                        domain.ReleaseType
		Version, AssociationVersion uint64
		Nodes, Monitors             []byte
	}
	err := session.database.WithContext(ctx).Raw(`SELECT p.table_name,t.code,t.name,t.release_type AS type,t.version,a.version AS association_version,t.node_list AS nodes,t.monitor_list AS monitors
		FROM rcc_table_policies p JOIN rcc_table_release_templates a ON a.table_policy_id=p.id
		JOIN rcc_release_templates t ON t.id=a.template_id AND t.release_type=a.release_type
		WHERE p.table_name IN ? AND a.release_type=? AND p.enabled=1 AND a.enabled=1 AND t.enabled=1 ORDER BY p.table_name`, tables, kind).Scan(&rows).Error
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	for _, row := range rows {
		template := domain.ReleaseTemplate{Code: row.Code, Name: row.Name, Type: row.Type, Version: row.Version, Enabled: true}
		if json.Unmarshal(row.Nodes, &template.Nodes) != nil || json.Unmarshal(row.Monitors, &template.MonitorList) != nil || len(template.MonitorList) != 0 || domain.ValidateReleaseTemplateNodes(template.Type, template.Nodes) != nil || row.Version == 0 || row.AssociationVersion == 0 {
			return nil, application.ErrReleaseUnavailable
		}
		result = append(result, domain.ReleaseFlowConfiguration{TableName: row.TableName, Template: template, AssociationVersion: row.AssociationVersion})
	}
	return result, nil
}
