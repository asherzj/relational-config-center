package domain_test

import (
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"strings"
	"testing"
)

func TestRegistrationFieldBoundaries(t *testing.T) {
	name := strings.Repeat("界", 64)
	baseline := domain.AccountRegistration{Username: "Abc", Email: "a+tag@example.com", Password: strings.Repeat("🔐", 15), DisplayName: &name}
	accepted, err := domain.NormalizeRegistration(baseline)
	if err != nil || accepted.Username != "abc" || accepted.DisplayName != name || accepted.Email != "a+tag@example.com" {
		t.Fatalf("legal Unicode bounds: %#v %v", accepted, err)
	}
	for _, test := range []struct {
		name string
		edit func(*domain.AccountRegistration)
	}{
		{"ASCII username only", func(v *domain.AccountRegistration) { v.Username = "Kelvin" }},
		{"ASCII email only", func(v *domain.AccountRegistration) { v.Email = "Kelvin@example.com" }},
		{"username short", func(v *domain.AccountRegistration) { v.Username = "ab" }},
		{"username long", func(v *domain.AccountRegistration) { v.Username = strings.Repeat("a", 33) }},
		{"username first letter", func(v *domain.AccountRegistration) { v.Username = "1ab" }},
		{"email display address", func(v *domain.AccountRegistration) { v.Email = "Alice <alice@example.com>" }},
		{"empty email", func(v *domain.AccountRegistration) { v.Email = " " }},
		{"email invalid", func(v *domain.AccountRegistration) { v.Email = "a@@example.com" }},
		{"short password", func(v *domain.AccountRegistration) { v.Password = strings.Repeat("🔐", 14) }},
		{"long password", func(v *domain.AccountRegistration) { v.Password = strings.Repeat("🔐", 129) }},
		{"empty display name", func(v *domain.AccountRegistration) { empty := "  "; v.DisplayName = &empty }},
		{"display name control", func(v *domain.AccountRegistration) { control := "a\nb"; v.DisplayName = &control }},
		{"display name long", func(v *domain.AccountRegistration) { long := strings.Repeat("界", 65); v.DisplayName = &long }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := baseline
			test.edit(&input)
			if _, err := domain.NormalizeRegistration(input); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	for _, size := range []int{15, 128} {
		input := baseline
		input.Password = strings.Repeat(" ", size)
		if _, err := domain.NormalizeRegistration(input); err != nil {
			t.Fatalf("password spaces of size %d rejected", size)
		}
	}
	input := baseline
	input.Username = strings.Repeat("a", 32)
	input.DisplayName = nil
	if result, err := domain.NormalizeRegistration(input); err != nil || result.DisplayName != input.Username {
		t.Fatal("username length/default boundary")
	}
}
