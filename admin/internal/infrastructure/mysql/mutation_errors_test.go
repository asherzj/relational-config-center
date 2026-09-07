package mysql

import (
	"errors"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	driver "github.com/go-sql-driver/mysql"
)

func TestMutationStorageValueErrorsAreEditableButOtherDatabaseFailuresRemainUnavailable(t *testing.T) {
	for _, test := range []struct {
		number uint16
		want   error
	}{
		{1062, application.ErrDuplicateKey},
		{1265, application.ErrInvalidMutation},
		{1406, application.ErrInvalidMutation},
		{1264, application.ErrInvalidMutation},
		{1146, application.ErrMutationUnavailable},
		{1213, application.ErrMutationUnavailable},
		{1644, application.ErrMutationUnavailable},
	} {
		if got := classifyMutationError(&driver.MySQLError{Number: test.number}, nil); !errors.Is(got, test.want) {
			t.Errorf("MySQL %d: got %v, want %v", test.number, got, test.want)
		}
	}
}
