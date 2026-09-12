package application

import (
	"context"
	"strconv"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Only explicit draft writes instantiate. Existing instances remain unchanged;
// removed tables leave the workflow, and unavailable config remains visible.
func saveDraftFlows(ctx context.Context, session ReleaseOrderSession, order *ReleaseOrder) error {
	existing := map[string]domain.ReleaseTableFlow{}
	for _, flow := range order.TableFlows {
		existing[flow.TableName] = flow
	}
	missing := []string{}
	for _, table := range order.TableNames {
		if _, found := existing[table]; !found {
			missing = append(missing, table)
		}
	}
	configurations, err := session.ReadReleaseFlowConfigurations(ctx, missing, order.ReleaseType)
	if err != nil {
		return err
	}
	for _, configuration := range configurations {
		id, err := newDetailID()
		if err != nil {
			return err
		}
		template := configuration.Template
		flow := domain.ReleaseTableFlow{InstanceID: id, TableName: configuration.TableName, ReleaseType: template.Type, TemplateCode: template.Code, TemplateName: template.Name, TemplateVersion: strconv.FormatUint(template.Version, 10), AssociationVersion: strconv.FormatUint(configuration.AssociationVersion, 10), InstantiatedAt: order.UpdatedAt, Nodes: []domain.ReleaseNodeInstance{}}
		for _, node := range template.Nodes {
			flow.Nodes = append(flow.Nodes, domain.ReleaseNodeInstance{ReleaseTemplateNode: node, State: "PENDING"})
		}
		existing[configuration.TableName] = flow
	}
	order.TableFlows = []domain.ReleaseTableFlow{}
	order.MissingFlowTables = []string{}
	for _, table := range order.TableNames {
		if flow, found := existing[table]; found {
			order.TableFlows = append(order.TableFlows, flow)
		} else {
			order.MissingFlowTables = append(order.MissingFlowTables, table)
		}
	}
	return nil
}

// Persist progress from real table decisions and order events in the same write
// as those facts. A stopped workflow never fabricates completion or an actor.
func advanceReleaseFlows(order *ReleaseOrder) {
	approvals := map[string]domain.ReleaseTableApproval{}
	for _, approval := range order.Approvals {
		approvals[approval.TableName] = approval
	}
	events := map[string]domain.ReleaseEvent{}
	for _, event := range order.History {
		events[event.Action] = event
	}
	for fi := range order.TableFlows {
		flow := &order.TableFlows[fi]
		for ni := range flow.Nodes {
			node := &flow.Nodes[ni]
			node.State, node.ActorID, node.At = "PENDING", "", ""
			switch node.Type {
			case "APPROVAL":
				approval := approvals[flow.TableName]
				if approval.Decision != nil {
					node.ActorID, node.At = approval.Decision.ActorID, approval.Decision.At
					if approval.State == "APPROVED" {
						node.State = "COMPLETED"
					} else if approval.State == "REJECTED" {
						node.State = "REJECTED"
					}
				} else if order.State == "PENDING_APPROVAL" {
					node.State = "ACTIVE"
				}
			case "PUBLICATION":
				if event, found := events["EXECUTE"]; found {
					node.State, node.ActorID, node.At = "COMPLETED", event.ActorID, event.At
				} else if order.State == "APPROVED" || order.State == "PENDING_PUBLICATION" {
					node.State = "ACTIVE"
				}
			case "COMPLETION":
				if event, found := events["COMPLETE"]; found {
					node.State, node.ActorID, node.At = "COMPLETED", event.ActorID, event.At
				} else if order.State == "SUCCEEDED" {
					node.State = "ACTIVE"
				}
			}
			if (order.State == "CANCELLED" || order.State == "REJECTED" || order.State == "ROLLED_BACK") && (node.State == "PENDING" || node.State == "ACTIVE") {
				node.State = "STOPPED"
			}
		}
	}
	// Restoration has one actual execution and ends the original order. The
	// template's completion definition is retained, without inventing that action.
	for fi := range order.RollbackTableFlows {
		for ni := range order.RollbackTableFlows[fi].Nodes {
			node := &order.RollbackTableFlows[fi].Nodes[ni]
			node.State, node.ActorID, node.At = "PENDING", "", ""
			if order.State == "SUCCEEDED" && node.Type == "PUBLICATION" {
				node.State = "ACTIVE"
			} else if order.State == "ROLLED_BACK" || order.State == "COMPLETED" {
				node.State = "STOPPED"
			}
			if event, found := events["QUICK_ROLLBACK"]; found && node.Type == "PUBLICATION" {
				node.State, node.ActorID, node.At = "COMPLETED", event.ActorID, event.At
			}
		}
	}
}
