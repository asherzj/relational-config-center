package domain

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrAccountFields   = errors.New("invalid account fields")
	ErrAccountConflict = errors.New("username or email already occupied")
	ErrCredentials     = errors.New("invalid credentials")
	ErrCurrentPassword = errors.New("current password invalid")
	ErrSession         = errors.New("invalid session")
	ErrCSRF            = errors.New("invalid csrf credentials")
	ErrAuthUnavailable = errors.New("authentication unavailable")
	ErrAuthTimeout     = errors.New("authentication timed out")
)

type RateLimited struct{ RetryAfter time.Duration }

func (e *RateLimited) Error() string { return "authentication rate limited" }

type LocalAccount struct {
	ID              string
	Username        string
	Email           string
	DisplayName     string
	PasswordHash    string
	Enabled         bool
	PasswordVersion uint64
	SessionVersion  uint64
}

type LoginSession struct {
	TokenHash       string
	AccountID       string
	CSRFHash        string
	PasswordVersion uint64
	SessionVersion  uint64
	CreatedAt       time.Time
	LastActiveAt    time.Time
	ExpiresAt       time.Time
}

type Preauth struct {
	TokenHash string
	CSRFHash  string
	ExpiresAt time.Time
}

type AccountRegistration struct {
	Username    string
	Email       string
	DisplayName *string
	Password    string
}

var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,31}$`)

func NormalizeUsername(value string) string {
	return strings.Map(func(c rune) rune {
		if c >= 'A' && c <= 'Z' {
			return c + ('a' - 'A')
		}
		return c
	}, strings.TrimSpace(value))
}
func NormalizeEmail(value string) string { return NormalizeUsername(value) }
func NormalizeRegistration(input AccountRegistration) (LocalAccount, error) {
	username := NormalizeUsername(input.Username)
	email := NormalizeEmail(input.Email)
	if !usernamePattern.MatchString(username) || !ValidEmail(email) || !ValidPassword(input.Password) {
		return LocalAccount{}, ErrAccountFields
	}
	displayName := username
	if input.DisplayName != nil {
		displayName = strings.TrimSpace(*input.DisplayName)
	}
	if !ValidDisplayName(displayName) {
		return LocalAccount{}, ErrAccountFields
	}
	return LocalAccount{Username: username, Email: email, DisplayName: displayName, Enabled: true, PasswordVersion: 1, SessionVersion: 1}, nil
}
func ValidPassword(value string) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && n >= 15 && n <= 128
}
func ValidDisplayName(value string) bool {
	n := utf8.RuneCountInString(value)
	if !utf8.ValidString(value) || n < 1 || n > 64 {
		return false
	}
	for _, c := range value {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
func ValidEmail(value string) bool {
	if value == "" || len(value) > 254 {
		return false
	}
	for _, c := range value {
		if c > 127 || unicode.IsSpace(c) || unicode.IsControl(c) {
			return false
		}
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && !strings.ContainsAny(value, "<>\"")
}

// AccountRepository commits account and credential transitions atomically.
// No transport, SQL or password hashing details cross this boundary.
type AccountRepository interface {
	Prepare(context.Context, Preauth, time.Time) error
	CheckPreauth(context.Context, CredentialProof, time.Time) error
	Register(context.Context, LocalAccount, LoginSession, SessionAdmission, time.Time) error
	AccountByUsername(context.Context, string) (LocalAccount, error)
	IssueSession(context.Context, LocalAccount, LoginSession, SessionAdmission, LoginReservation, time.Time) error
	CurrentSession(context.Context, string, time.Time) (LocalAccount, LoginSession, error)
	AuthenticatedSession(context.Context, CredentialProof, time.Time) (LocalAccount, LoginSession, error)
	TouchSession(context.Context, CredentialProof, time.Time) (LocalAccount, LoginSession, error)
	UpdateDisplayName(context.Context, CredentialProof, string, time.Time) (LocalAccount, LoginSession, error)
	UpdateEmail(context.Context, LocalAccount, CredentialProof, string, time.Time) (LocalAccount, LoginSession, error)
	ChangePassword(context.Context, LocalAccount, CredentialProof, string, time.Time) error
	RevokeSession(context.Context, CredentialProof, time.Time) error
	RevokeAccountSessions(context.Context, CredentialProof, time.Time) error
}

// Rate counters are persistent authentication control state, never account status.
type AuthenticationRateStore interface {
	Admit(context.Context, string, int, time.Duration, time.Time) error
	ReserveLogin(context.Context, string, int, time.Duration, time.Time) (LoginReservation, error)
	RejectLogin(context.Context, LoginReservation, bool, time.Time) error
}

type SessionAdmission struct {
	PreauthHash         string
	PreviousSessionHash string
}
type LoginReservation struct {
	Key       string
	ExpiresAt time.Time
}

type CredentialProof struct {
	TokenHash string
	CSRFHash  string
}
