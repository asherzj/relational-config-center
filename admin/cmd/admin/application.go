package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

type adminApplication struct {
	handler http.Handler
	mysql   *mysqladapter.Adapter
}

func newApplication(ctx context.Context, settings config.Config) (*adminApplication, error) {
	return newApplicationWithClock(ctx, settings, nil)
}

func newApplicationWithClock(ctx context.Context, settings config.Config, now func() time.Time) (*adminApplication, error) {
	mysql, err := mysqladapter.Open(ctx, settings.MySQL)
	if err != nil {
		return nil, err
	}

	discovery := application.NewDatabaseTableDiscovery(mysql)
	queryPolicies := application.NewQueryPolicyManagement(mysql, application.NewQueryPolicyTypeRegistry())
	mutationPolicies := application.NewMutationPolicyManagement(mysql, application.NewMutationPolicyTypeRegistry())
	policies := application.NewTablePolicyManagement(mysql, mysql, queryPolicies, mutationPolicies)
	queries := application.NewManagedTableQuery(mysql, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry())
	return &adminApplication{
		handler: httpinterface.NewRouter(discovery, mysql, queryPolicies, mutationPolicies, policies, queries, httpinterface.RouterOptions{
			FieldPolicies:      application.NewTableFieldPolicyManagement(mysql, mysql, mysql, mysql),
			Authentication:     application.NewAuthentication(mysql, passwordadapter.NewArgon2id(), now, mysql, application.AuthenticationLimits{Registration: settings.AccountRegisterLimit, LoginIP: settings.AccountLoginIPLimit, LoginFailures: settings.AccountLoginFailureLimit}),
			AccountRoles:       application.NewAccountRoleManagement(mysql),
			ReleaseOrders:      application.NewReleaseOrders(mysql),
			ReleaseTemplates:   application.NewReleaseTemplateManagement(mysql),
			PublicationTimeout: accountRequestTimeout(settings.MySQL),
			AccountHTTP:        httpinterface.AccountHTTPOptions{RequestTimeout: accountRequestTimeout(settings.MySQL), PublicOrigin: settings.AccountPublicOrigin, InsecureLocalHTTP: settings.AccountInsecureHTTP, TrustedProxies: settings.AccountTrustedProxies},
			AccessLog:          os.Stdout,
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

// Leave time for a stable HTTP error before any driver socket/HTTP deadline.
func accountRequestTimeout(mysql config.MySQL) time.Duration {
	timeout := 8 * time.Second
	for _, value := range []time.Duration{mysql.ConnectTimeout, mysql.ReadTimeout, mysql.WriteTimeout} {
		if value > 0 && value < timeout {
			timeout = value
		}
	}
	return timeout * 4 / 5
}
