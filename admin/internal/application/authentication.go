package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type PasswordHasher interface {
	Hash(context.Context, string) (string, error)
	Verify(context.Context, string, string) (bool, error)
}
type AuthenticationLimits struct {
	Registration  int
	LoginIP       int
	LoginFailures int
}
type Authentication struct {
	accounts  domain.AccountRepository
	passwords PasswordHasher
	now       func() time.Time
	rates     domain.AuthenticationRateStore
	limits    AuthenticationLimits
}

func NewAuthentication(accounts domain.AccountRepository, passwords PasswordHasher, now func() time.Time, rates domain.AuthenticationRateStore, limits AuthenticationLimits) *Authentication {
	if now == nil {
		now = time.Now
	}
	if limits.Registration < 1 {
		limits.Registration = 10
	}
	if limits.LoginIP < 1 {
		limits.LoginIP = 60
	}
	if limits.LoginFailures < 1 {
		limits.LoginFailures = 10
	}
	// Persisted account timestamps have microsecond precision. Use the same
	// clock for writes, returned deadlines and expiry checks; rounding upward
	// in storage must never extend a credential's lifetime.
	accountNow := func() time.Time { return now().UTC().Truncate(time.Microsecond) }
	return &Authentication{accounts: accounts, passwords: passwords, now: accountNow, rates: rates, limits: limits}
}

// Registration is the application input; HTTP does not import Domain models.
type Registration struct {
	Username    string
	Email       string
	Password    string
	DisplayName *string
}
type AccountIdentity struct {
	ID          string
	Username    string
	Email       string
	DisplayName string
	Roles       domain.AccountRoles
}
type SessionTiming struct {
	LastActiveAt time.Time
	ExpiresAt    time.Time
}
type AuthenticationResult struct {
	Account AccountIdentity
	Session SessionTiming
	Token   string
	CSRF    string
}
type LoginReservation = domain.LoginReservation

// These stable application errors preserve errors.Is while hiding repositories
// and their storage failures from interface adapters.
var (
	ErrAccountFields   = domain.ErrAccountFields
	ErrAccountConflict = domain.ErrAccountConflict
	ErrCredentials     = domain.ErrCredentials
	ErrCurrentPassword = domain.ErrCurrentPassword
	ErrSession         = domain.ErrSession
	ErrAccountDisabled = domain.ErrAccountDisabled
	ErrCSRF            = domain.ErrCSRF
	ErrAuthTimeout     = domain.ErrAuthTimeout
)

type AuthenticationRateLimited = domain.RateLimited

func credential() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", domain.ErrAuthUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func TokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
func csrfFor(token string) string { return TokenDigest("rcc-csrf:" + token) }
func currentIdentity(account domain.LocalAccount, session domain.LoginSession, token string) AuthenticationResult {
	return AuthenticationResult{Account: AccountIdentity{ID: account.ID, Username: account.Username, Email: account.Email, DisplayName: account.DisplayName, Roles: account.Roles}, Session: SessionTiming{LastActiveAt: session.LastActiveAt, ExpiresAt: session.ExpiresAt}, CSRF: csrfFor(token)}
}
func (a *Authentication) Prepare(ctx context.Context) (string, string, error) {
	token, err := credential()
	if err != nil {
		return "", "", err
	}
	csrf := csrfFor(token)
	err = a.accounts.Prepare(ctx, domain.Preauth{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf), ExpiresAt: a.now().Add(10 * time.Minute)}, a.now())
	return token, csrf, err
}
func (a *Authentication) CheckPreauth(ctx context.Context, token, csrf string) error {
	if token == "" || csrf == "" {
		return domain.ErrCSRF
	}
	return a.accounts.CheckPreauth(ctx, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, a.now())
}
func (a *Authentication) Register(ctx context.Context, input Registration, preauth, previous string) (AuthenticationResult, error) {
	account, err := domain.NormalizeRegistration(domain.AccountRegistration{Username: input.Username, Email: input.Email, Password: input.Password, DisplayName: input.DisplayName})
	if err != nil {
		return AuthenticationResult{}, err
	}
	account.Roles, account.RoleVersion = domain.RoleViewer, 1
	account.PasswordHash, err = a.passwords.Hash(ctx, input.Password)
	if err != nil {
		return AuthenticationResult{}, err
	}
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return AuthenticationResult{}, domain.ErrAuthUnavailable
	}
	raw[6] = (raw[6] & 15) | 64
	raw[8] = (raw[8] & 63) | 128
	account.ID = fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
	session, token, err := a.newSession(account)
	if err != nil {
		return AuthenticationResult{}, err
	}
	admission := domain.SessionAdmission{PreauthHash: TokenDigest(preauth), PreviousSessionHash: TokenDigest(previous)}
	if err := a.accounts.Register(ctx, account, session, admission, a.now()); err != nil {
		return AuthenticationResult{}, err
	}
	result := currentIdentity(account, session, token)
	result.Token = token
	return result, nil
}
func (a *Authentication) newSession(account domain.LocalAccount) (domain.LoginSession, string, error) {
	token, err := credential()
	if err != nil {
		return domain.LoginSession{}, "", err
	}
	now := a.now()
	return domain.LoginSession{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrfFor(token)), AccountID: account.ID, PasswordVersion: account.PasswordVersion, SessionVersion: account.SessionVersion, CreatedAt: now, LastActiveAt: now, ExpiresAt: now.Add(8 * time.Hour)}, token, nil
}
func (a *Authentication) Current(ctx context.Context, token string) (AuthenticationResult, error) {
	if token == "" {
		return AuthenticationResult{}, domain.ErrSession
	}
	account, session, err := a.accounts.CurrentSession(ctx, TokenDigest(token), a.now())
	return currentIdentity(account, session, token), err
}
func (a *Authentication) Logout(ctx context.Context, token, csrf string) error {
	if token == "" {
		return domain.ErrSession
	}
	return a.accounts.RevokeSession(ctx, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, a.now())
}
func (a *Authentication) LogoutAll(ctx context.Context, token, csrf string) error {
	if token == "" {
		return domain.ErrSession
	}
	return a.accounts.RevokeAccountSessions(ctx, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, a.now())
}
func (a *Authentication) Activity(ctx context.Context, token, csrf string) (AuthenticationResult, error) {
	if token == "" {
		return AuthenticationResult{}, domain.ErrSession
	}
	account, session, err := a.accounts.TouchSession(ctx, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, a.now())
	return currentIdentity(account, session, token), err
}
func (a *Authentication) authorizeChange(ctx context.Context, token, csrf string) (domain.LocalAccount, error) {
	if token == "" {
		return domain.LocalAccount{}, domain.ErrSession
	}
	account, _, err := a.accounts.AuthenticatedSession(ctx, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, a.now())
	return account, err
}

// AuthorizeChange lets an interface reject unauthenticated state-changing
// requests before it parses or validates account input. Each operation still
// rechecks the same proof at its atomic repository transition.
func (a *Authentication) AuthorizeChange(ctx context.Context, token, csrf string) error {
	_, err := a.authorizeChange(ctx, token, csrf)
	return err
}
func (a *Authentication) UpdateDisplayName(ctx context.Context, token, csrf, displayName string) (AuthenticationResult, error) {
	if _, err := a.authorizeChange(ctx, token, csrf); err != nil {
		return AuthenticationResult{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if !domain.ValidDisplayName(displayName) {
		return AuthenticationResult{}, domain.ErrAccountFields
	}
	account, session, err := a.accounts.UpdateDisplayName(ctx, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, displayName, a.now())
	return currentIdentity(account, session, token), err
}
func (a *Authentication) UpdateEmail(ctx context.Context, token, csrf, email, currentPassword string) (AuthenticationResult, error) {
	account, err := a.authorizeChange(ctx, token, csrf)
	if err != nil {
		return AuthenticationResult{}, err
	}
	email = domain.NormalizeEmail(email)
	if !domain.ValidEmail(email) {
		return AuthenticationResult{}, domain.ErrAccountFields
	}
	valid, err := a.passwords.Verify(ctx, currentPassword, account.PasswordHash)
	if err != nil {
		return AuthenticationResult{}, err
	}
	if !valid {
		return AuthenticationResult{}, domain.ErrCurrentPassword
	}
	account, session, err := a.accounts.UpdateEmail(ctx, account, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, email, a.now())
	return currentIdentity(account, session, token), err
}
func (a *Authentication) ChangePassword(ctx context.Context, token, csrf, currentPassword, newPassword string) error {
	account, err := a.authorizeChange(ctx, token, csrf)
	if err != nil {
		return err
	}
	if !domain.ValidPassword(newPassword) {
		return domain.ErrAccountFields
	}
	valid, err := a.passwords.Verify(ctx, currentPassword, account.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return domain.ErrCurrentPassword
	}
	passwordHash, err := a.passwords.Hash(ctx, newPassword)
	if err != nil {
		return err
	}
	return a.accounts.ChangePassword(ctx, account, domain.CredentialProof{TokenHash: TokenDigest(token), CSRFHash: TokenDigest(csrf)}, passwordHash, a.now())
}
func (a *Authentication) AdmitRegistration(ctx context.Context, ip string) error {
	return a.rates.Admit(ctx, "register:"+TokenDigest(ip), a.limits.Registration, time.Hour, a.now())
}
func (a *Authentication) AdmitLogin(ctx context.Context, ip, username string) (LoginReservation, error) {
	if err := a.rates.Admit(ctx, "login-ip:"+TokenDigest(ip), a.limits.LoginIP, time.Minute, a.now()); err != nil {
		return LoginReservation{}, err
	}
	return a.rates.ReserveLogin(ctx, "login-user:"+TokenDigest(domain.NormalizeUsername(username)), a.limits.LoginFailures, 15*time.Minute, a.now())
}
func (a *Authentication) LoginAttempt(ctx context.Context, username, password, preauth, previous string, reservation LoginReservation) (AuthenticationResult, error) {
	result, err := a.login(ctx, username, password, preauth, previous, reservation)
	if err != nil {
		// Successful issuance settles inside the same account/session transaction.
		// Every rejected path releases its reservation, counting credential failures.
		settlementContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if settleErr := a.rates.RejectLogin(settlementContext, reservation, errors.Is(err, domain.ErrCredentials), a.now()); settleErr != nil {
			if errors.Is(err, domain.ErrAuthTimeout) {
				return AuthenticationResult{}, err
			}
			return AuthenticationResult{}, settleErr
		}
	}
	return result, err
}
func (a *Authentication) login(ctx context.Context, username, password, preauth, previous string, reservation LoginReservation) (AuthenticationResult, error) {
	if !domain.ValidPassword(password) {
		return AuthenticationResult{}, domain.ErrCredentials
	}
	account, err := a.accounts.AccountByUsername(ctx, domain.NormalizeUsername(username))
	if err != nil && !errors.Is(err, domain.ErrCredentials) {
		return AuthenticationResult{}, err
	}
	encoded := account.PasswordHash
	if errors.Is(err, domain.ErrCredentials) {
		encoded = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	valid, err := a.passwords.Verify(ctx, password, encoded)
	if err != nil {
		return AuthenticationResult{}, err
	}
	if !valid || !account.Enabled {
		return AuthenticationResult{}, domain.ErrCredentials
	}
	session, token, err := a.newSession(account)
	if err != nil {
		return AuthenticationResult{}, err
	}
	admission := domain.SessionAdmission{PreauthHash: TokenDigest(preauth), PreviousSessionHash: TokenDigest(previous)}
	if err := a.accounts.IssueSession(ctx, account, session, admission, reservation, a.now()); err != nil {
		return AuthenticationResult{}, err
	}
	result := currentIdentity(account, session, token)
	result.Token = token
	return result, nil
}
