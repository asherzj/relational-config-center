//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Existing whole-transaction assertions collect the result from each original
// detail in execution order. Production reads use bounded detail pages.
func executionCommands(order domain.ReleaseOrder, kind string) []domain.PublicationCommand {
	commands := make([]domain.PublicationCommand, 0, len(order.Items))
	for index := range order.Items {
		position := index
		if kind == "ROLLBACK" {
			position = len(order.Items) - 1 - index
		}
		command := order.Items[position].Publication
		if kind == "ROLLBACK" {
			command = order.Items[position].Rollback
		}
		if command != nil {
			commands = append(commands, *command)
		}
	}
	return commands
}

func singleExecutionTableVersion(execution domain.ReleaseExecution) string {
	if len(execution.TableVersions) != 1 {
		panic("single-table assertion used for a multitable execution")
	}
	for _, version := range execution.TableVersions {
		return version
	}
	return ""
}

func applicationItems(order domain.ReleaseOrder) []domain.ReleaseItem {
	items := append([]domain.ReleaseItem(nil), order.Items...)
	for index := range items {
		items[index].Publication, items[index].Rollback = nil, nil
	}
	return items
}

// Existing whole-order assertions deliberately assemble the public header and
// version-bound detail pages. Raw HTTP paging tests bypass this helper.
func releaseReadAllDetails(t *testing.T, app *adminApplication, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	session := integrationAdminSession(t, app)
	return releaseActorReadAllDetails(t, app, session, method, path, body, key)
}
func releaseActorReadAllDetails(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	response := releaseActorRequest(t, app, actor, method, path, body, key)
	if method != "GET" || response.Code != 200 {
		return response
	}
	var header domain.ReleaseHeader
	var shape map[string]json.RawMessage
	if json.Unmarshal(response.Body.Bytes(), &shape) != nil || shape["item_count"] == nil || shape["history"] == nil {
		return response
	}
	if err := json.Unmarshal(response.Body.Bytes(), &header); err != nil {
		t.Fatal(err)
	}
	order := header.Workflow()
	order.Items = []domain.ReleaseItem{}
	for offset := 0; offset < header.ItemCount; {
		pageResponse := releaseActorRequest(t, app, actor, "GET", fmt.Sprintf("%s/details?expected_version=%s&offset=%d&limit=100", path, header.Version, offset), "", "")
		if pageResponse.Code != 200 {
			return pageResponse
		}
		var page domain.ReleaseDetailPage
		if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Version != header.Version || page.OrderID != header.ID || page.Offset != offset || len(page.Items) == 0 {
			t.Fatal("inconsistent public detail page", pageResponse.Body)
		}
		order.Items = append(order.Items, page.Items...)
		offset += len(page.Items)
	}
	var actions []string
	if err := json.Unmarshal(shape["allowed_actions"], &actions); err != nil {
		t.Fatal(err)
	}
	result := struct {
		domain.ReleaseOrder
		ItemCount       int            `json:"item_count"`
		OperationCounts map[string]int `json:"operation_counts"`
		AllowedActions  []string       `json:"allowed_actions"`
	}{order, header.ItemCount, header.OperationCounts, actions}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Reset()
	response.Body.Write(encoded)
	return response
}
