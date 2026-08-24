package main

import (
	"context"
	"fmt"
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
	registry, err := application.NewStrategyRegistry(
		[]application.QueryRegistration{
			{ID: application.MySQLPageQueryV1, Constructor: application.NewMySQLPageQueryStrategy},
		},
		[]application.MutationRegistration{
			{ID: application.MySQLSingleTableMutationV1, Constructor: application.NewMySQLSingleTableMutationStrategy},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("strategy registration: %w", err)
	}

	mysql, err := mysqladapter.Open(ctx, settings.MySQL)
	if err != nil {
		return nil, err
	}

	discovery := application.NewDatabaseTableDiscovery(mysql)
	policies := application.NewTablePolicyManagement(mysql, mysql, registry, settings.Operator)
	queries := application.NewManagedTableQuery(mysql, mysql, registry, mysql)
	mutations := application.NewManagedTableMutation(mysql, mysql, registry, application.NewFixedOperatorProvider(settings.Operator), mysql)
	return &adminApplication{
		handler: httpinterface.NewRouter(discovery, mysql, policies, queries, mutations, httpinterface.RouterOptions{
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
