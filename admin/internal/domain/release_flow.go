package domain

// ReleaseTableFlow is the workflow definition acknowledged for one table by a
// saved draft. Template edits and later approval assignments cannot replace it.
type ReleaseTableFlow struct {
	InstanceID         string                `json:"instance_id"`
	TableName          string                `json:"table_name"`
	ReleaseType        ReleaseType           `json:"release_type"`
	TemplateCode       string                `json:"template_code"`
	TemplateName       string                `json:"template_name"`
	TemplateVersion    string                `json:"template_version"`
	AssociationVersion string                `json:"association_version"`
	InstantiatedAt     string                `json:"instantiated_at"`
	Nodes              []ReleaseNodeInstance `json:"node_list"`
}

type ReleaseNodeInstance struct {
	ReleaseTemplateNode
	State   string `json:"state"`
	ActorID string `json:"actor_id,omitempty"`
	At      string `json:"at,omitempty"`
}

// ReleaseFlowConfiguration is a complete source definition read consistently
// with the other draft inputs. Its absence means explicitly unavailable config.
type ReleaseFlowConfiguration struct {
	TableName          string
	Template           ReleaseTemplate
	AssociationVersion uint64
}

// HasCompleteFlows checks every participating table; an empty draft or one
// missing configuration can never acquire submission or publication authority.
func (order ReleaseOrder) HasCompleteFlows() bool {
	if len(order.TableNames) == 0 || len(order.MissingFlowTables) != 0 || len(order.TableFlows) != len(order.TableNames) {
		return false
	}
	flows := map[string]bool{}
	for _, flow := range order.TableFlows {
		if flows[flow.TableName] || flow.ReleaseType != order.ReleaseType || flow.InstanceID == "" || flow.TemplateCode == "" || flow.TemplateName == "" || flow.TemplateVersion == "" || flow.AssociationVersion == "" || flow.InstantiatedAt == "" {
			return false
		}
		nodes := make([]ReleaseTemplateNode, len(flow.Nodes))
		for index, node := range flow.Nodes {
			nodes[index] = node.ReleaseTemplateNode
		}
		if ValidateReleaseTemplateNodes(flow.ReleaseType, nodes) != nil {
			return false
		}
		flows[flow.TableName] = true
	}
	for _, table := range order.TableNames {
		if !flows[table] {
			return false
		}
	}
	return true
}
