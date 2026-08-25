package main

import (
	"context"
	"net/http"
	"os"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

type adminApplication struct {
	handler http.Handler
	mysql   *mysqladapter.Adapter
}

func newApplication(ctx context.Context, settings config.Config) (*adminApplication, error) {
	mysql, err := mysqladapter.Open(ctx, settings.MySQL)
	if err != nil {
		return nil, err
	}

	discovery := application.NewDatabaseTableDiscovery(mysql)
	queryPolicies := application.NewQueryPolicyManagement(mysql, application.NewQueryPolicyTypeRegistry(), settings.Operator)
	mutationPolicies := application.NewMutationPolicyManagement(mysql, application.NewMutationPolicyTypeRegistry(), settings.Operator)
	policies := application.NewTablePolicyManagement(mysql, mysql, queryPolicies, mutationPolicies, settings.Operator)
	queries := application.NewManagedTableQuery(mysql, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry())
	mutations := application.NewManagedTableMutation(mysql, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry(), application.NewFixedOperatorProvider(settings.Operator))
	return &adminApplication{
		handler: httpinterface.NewRouter(discovery, mysql, queryPolicies, mutationPolicies, policies, queries, mutations, httpinterface.RouterOptions{
			APIToken:     settings.APIToken,
			AuthDisabled: settings.AuthDisabled,
			CORSOrigins:  settings.CORSOrigins,
			AccessLog:    os.Stdout,
		}),
		mysql: mysql,
	}, nil
}

func (application *adminApplication) Handler() http.Handler {
	return application.handler
}

func (application *adminApplication) Close() error {
	return application.mysql.Close()
}
