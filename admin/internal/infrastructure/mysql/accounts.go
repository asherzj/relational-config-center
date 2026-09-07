package mysql

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const accountTable = "rcc_accounts"
const sessionTable = "rcc_login_sessions"
const preauthTable = "rcc_preauth_credentials"
const rateTable = "rcc_auth_rate_limits"
const maxPreauth = 10000
const maxSessions = 100000
const maxRateBuckets = 100000

type storedAccount struct {
	domain.LocalAccount `gorm:"embedded"`
	CreatedAt           time.Time
}

func authError(err error) error {
	if err == nil {
		return nil
	}
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1205 {
		return domain.ErrAuthTimeout
	}
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrAccountConflict
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return domain.ErrAuthTimeout
	}
	for _, known := range []error{domain.ErrAccountNotFound, domain.ErrAccountConflict, domain.ErrCredentials, domain.ErrCurrentPassword, domain.ErrSession, domain.ErrAccountDisabled, domain.ErrCSRF, domain.ErrAuthUnavailable, domain.ErrAuthTimeout} {
		if errors.Is(err, known) {
			return known
		}
	}
	var limited *domain.RateLimited
	if errors.As(err, &limited) {
		return limited
	}
	return domain.ErrAuthUnavailable
}
func (a *Adapter) authTransaction(ctx context.Context, now time.Time, fn func(*gorm.DB) error) error {
	// Maintenance callers also trigger expiry cleanup without using the
	// Authentication clock. Keep their cleanup comparisons at storage precision.
	now = now.UTC().Truncate(time.Microsecond)
	return authError(a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lock struct{ ID int }
		if err := tx.Table("rcc_auth_control_lock").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = 1").Take(&lock).Error; err != nil {
			return err
		}
		if err := tx.Table(preauthTable).Where("expires_at <= ?", now).Delete(&domain.Preauth{}).Error; err != nil {
			return err
		}
		if err := tx.Table(sessionTable).Where("expires_at <= ? OR last_active_at <= ?", now, now.Add(-30*time.Minute)).Delete(&domain.LoginSession{}).Error; err != nil {
			return err
		}
		if err := tx.Table(rateTable).Where("expires_at <= ?", now).Delete(&rateBucket{}).Error; err != nil {
			return err
		}
		return fn(tx)
	}))
}
func capacity(tx *gorm.DB, table string, limit int) error {
	var count int64
	if err := tx.Table(table).Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(limit) {
		return domain.ErrAuthUnavailable
	}
	return nil
}
func (a *Adapter) Prepare(ctx context.Context, preauth domain.Preauth, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		if err := capacity(tx, preauthTable, maxPreauth); err != nil {
			return err
		}
		return tx.Table(preauthTable).Create(&preauth).Error
	})
}
func (a *Adapter) CheckPreauth(ctx context.Context, proof domain.CredentialProof, now time.Time) error {
	var found domain.Preauth
	err := a.gorm.WithContext(ctx).Table(preauthTable).Where("token_hash = ? AND csrf_hash = ? AND expires_at > ?", proof.TokenHash, proof.CSRFHash, now).Take(&found).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrCSRF
	}
	return authError(err)
}
func consumePreauth(tx *gorm.DB, token string, now time.Time) error {
	result := tx.Table(preauthTable).Where("token_hash = ? AND expires_at > ?", token, now).Delete(&domain.Preauth{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrCSRF
	}
	return nil
}
func saveSession(tx *gorm.DB, session domain.LoginSession, previous string) error {
	if err := capacity(tx, sessionTable, maxSessions); err != nil {
		return err
	}
	if err := tx.Table(sessionTable).Where("token_hash = ?", previous).Delete(&domain.LoginSession{}).Error; err != nil {
		return err
	}
	return tx.Table(sessionTable).Create(&session).Error
}
func (a *Adapter) Register(ctx context.Context, account domain.LocalAccount, session domain.LoginSession, admission domain.SessionAdmission, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		if err := consumePreauth(tx, admission.PreauthHash, now); err != nil {
			return err
		}
		row := storedAccount{LocalAccount: account, CreatedAt: now}
		if err := tx.Table(accountTable).Create(&row).Error; err != nil {
			return err
		}
		return saveSession(tx, session, admission.PreviousSessionHash)
	})
}
func (a *Adapter) AccountByUsername(ctx context.Context, username string) (domain.LocalAccount, error) {
	var account domain.LocalAccount
	err := a.gorm.WithContext(ctx).Table(accountTable).Where("username = ?", username).Take(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return account, domain.ErrCredentials
	}
	return account, authError(err)
}
func (a *Adapter) IssueSession(ctx context.Context, verified domain.LocalAccount, session domain.LoginSession, admission domain.SessionAdmission, reservation domain.LoginReservation, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var current domain.LocalAccount
		if err := tx.Table(accountTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", verified.ID).Take(&current).Error; err != nil {
			return err
		}
		if !current.Enabled || current.PasswordHash != verified.PasswordHash || current.PasswordVersion != verified.PasswordVersion || current.SessionVersion != verified.SessionVersion {
			return domain.ErrCredentials
		}
		if err := consumePreauth(tx, admission.PreauthHash, now); err != nil {
			return err
		}
		if err := settleLogin(tx, reservation, true, false); err != nil {
			return err
		}
		return saveSession(tx, session, admission.PreviousSessionHash)
	})
}
func currentSession(db *gorm.DB, token string, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	var row struct {
		domain.LocalAccount    `gorm:"embedded"`
		TokenHash              string
		CSRFHash               string
		SessionPasswordVersion uint64
		SessionSessionVersion  uint64
		CreatedAt              time.Time
		LastActiveAt           time.Time
		ExpiresAt              time.Time
	}
	// One statement observes account status and session versions consistently.
	// A valid credential for a disabled account is classified separately so Web
	// can destroy recoverable drafts without changing the uniform login failure.
	err := db.Table(sessionTable+" AS s").Select("a.*, s.token_hash, s.csrf_hash, s.password_version AS session_password_version, s.session_version AS session_session_version, s.created_at, s.last_active_at, s.expires_at").Joins("JOIN "+accountTable+" AS a ON a.id = s.account_id").Where("s.token_hash = ?", token).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.LocalAccount{}, domain.LoginSession{}, domain.ErrSession
	}
	if err != nil {
		return domain.LocalAccount{}, domain.LoginSession{}, authError(err)
	}
	if !row.Enabled {
		return domain.LocalAccount{}, domain.LoginSession{}, domain.ErrAccountDisabled
	}
	if row.PasswordVersion != row.SessionPasswordVersion || row.SessionVersion != row.SessionSessionVersion {
		return domain.LocalAccount{}, domain.LoginSession{}, domain.ErrSession
	}
	if !row.ExpiresAt.After(now) || !row.LastActiveAt.After(now.Add(-30*time.Minute)) {
		return domain.LocalAccount{}, domain.LoginSession{}, domain.ErrSession
	}
	return row.LocalAccount, domain.LoginSession{TokenHash: row.TokenHash, AccountID: row.ID, CSRFHash: row.CSRFHash, CreatedAt: row.CreatedAt, LastActiveAt: row.LastActiveAt, ExpiresAt: row.ExpiresAt}, nil
}
func (a *Adapter) CurrentSession(ctx context.Context, token string, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	return currentSession(a.gorm.WithContext(ctx), token, now)
}
func authenticatedSession(db *gorm.DB, proof domain.CredentialProof, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	account, session, err := currentSession(db, proof.TokenHash, now)
	if err != nil {
		return domain.LocalAccount{}, domain.LoginSession{}, err
	}
	if session.CSRFHash != proof.CSRFHash {
		return domain.LocalAccount{}, domain.LoginSession{}, domain.ErrCSRF
	}
	return account, session, nil
}
func (a *Adapter) AuthenticatedSession(ctx context.Context, proof domain.CredentialProof, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	return authenticatedSession(a.gorm.WithContext(ctx), proof, now)
}
func (a *Adapter) TouchSession(ctx context.Context, proof domain.CredentialProof, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	var account domain.LocalAccount
	var session domain.LoginSession
	err := a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var err error
		account, session, err = authenticatedSession(tx, proof, now)
		if err != nil {
			return err
		}
		result := tx.Table(sessionTable).
			Where("token_hash = ? AND expires_at > ? AND last_active_at > ?", proof.TokenHash, now, now.Add(-30*time.Minute)).
			Update("last_active_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrSession
		}
		session.LastActiveAt = now
		return nil
	})
	return account, session, err
}
func (a *Adapter) UpdateDisplayName(ctx context.Context, proof domain.CredentialProof, displayName string, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	var account domain.LocalAccount
	var session domain.LoginSession
	err := a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var err error
		account, session, err = authenticatedSession(tx, proof, now)
		if err != nil {
			return err
		}
		if err := tx.Table(accountTable).Where("id = ?", account.ID).Update("display_name", displayName).Error; err != nil {
			return err
		}
		account.DisplayName = displayName
		return nil
	})
	return account, session, err
}
func (a *Adapter) UpdateEmail(ctx context.Context, verified domain.LocalAccount, proof domain.CredentialProof, email string, now time.Time) (domain.LocalAccount, domain.LoginSession, error) {
	var account domain.LocalAccount
	var session domain.LoginSession
	err := a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var err error
		account, session, err = authenticatedSession(tx, proof, now)
		if err != nil {
			return err
		}
		if account.ID != verified.ID || account.PasswordHash != verified.PasswordHash || account.PasswordVersion != verified.PasswordVersion || account.SessionVersion != verified.SessionVersion {
			return domain.ErrCurrentPassword
		}
		if err := tx.Table(accountTable).Where("id = ?", account.ID).Update("email", email).Error; err != nil {
			return err
		}
		account.Email = email
		return nil
	})
	return account, session, err
}
func (a *Adapter) ChangePassword(ctx context.Context, verified domain.LocalAccount, proof domain.CredentialProof, passwordHash string, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		account, _, err := authenticatedSession(tx, proof, now)
		if err != nil {
			return err
		}
		if account.ID != verified.ID || account.PasswordHash != verified.PasswordHash || account.PasswordVersion != verified.PasswordVersion || account.SessionVersion != verified.SessionVersion {
			return domain.ErrCurrentPassword
		}
		if err := tx.Table(accountTable).Where("id = ?", account.ID).Updates(map[string]any{
			"password_hash":    passwordHash,
			"password_version": gorm.Expr("password_version + 1"),
			"session_version":  gorm.Expr("session_version + 1"),
		}).Error; err != nil {
			return err
		}
		return tx.Table(sessionTable).Where("account_id = ?", account.ID).Delete(&domain.LoginSession{}).Error
	})
}
func (a *Adapter) RevokeSession(ctx context.Context, proof domain.CredentialProof, now time.Time) error {
	_, session, err := a.CurrentSession(ctx, proof.TokenHash, now)
	if err != nil {
		return err
	}
	if session.CSRFHash != proof.CSRFHash {
		return domain.ErrCSRF
	}
	return authError(a.gorm.WithContext(ctx).Table(sessionTable).Where("token_hash = ?", proof.TokenHash).Delete(&domain.LoginSession{}).Error)
}
func (a *Adapter) RevokeAccountSessions(ctx context.Context, proof domain.CredentialProof, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		account, _, err := authenticatedSession(tx, proof, now)
		if err != nil {
			return err
		}
		if err := tx.Table(accountTable).Where("id = ?", account.ID).Update("session_version", gorm.Expr("session_version + 1")).Error; err != nil {
			return err
		}
		return tx.Table(sessionTable).Where("account_id = ?", account.ID).Delete(&domain.LoginSession{}).Error
	})
}

type rateBucket struct {
	BucketKey string
	Attempts  uint
	InFlight  uint
	ExpiresAt time.Time
}

var _ domain.AccountRepository = (*Adapter)(nil)

func (a *Adapter) Admit(ctx context.Context, key string, limit int, window time.Duration, now time.Time) error {
	_, err := a.admitRate(ctx, key, limit, window, now, false)
	return err
}
func (a *Adapter) ReserveLogin(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (domain.LoginReservation, error) {
	return a.admitRate(ctx, key, limit, window, now, true)
}
func (a *Adapter) admitRate(ctx context.Context, key string, limit int, window time.Duration, now time.Time, reserve bool) (domain.LoginReservation, error) {
	var reservation domain.LoginReservation
	err := a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var bucket rateBucket
		err := tx.Table(rateTable).Where("bucket_key = ?", key).Take(&bucket).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := capacity(tx, rateTable, maxRateBuckets); err != nil {
				return err
			}
			bucket = rateBucket{BucketKey: key, ExpiresAt: now.Add(window).UTC().Truncate(time.Microsecond)}
			if reserve {
				bucket.InFlight = 1
			} else {
				bucket.Attempts = 1
			}
			reservation = domain.LoginReservation{Key: key, ExpiresAt: bucket.ExpiresAt}
			return tx.Table(rateTable).Create(&bucket).Error
		}
		if err != nil {
			return err
		}
		if uint64(bucket.Attempts)+uint64(bucket.InFlight) >= uint64(limit) {
			return &domain.RateLimited{RetryAfter: bucket.ExpiresAt.Sub(now)}
		}
		reservation = domain.LoginReservation{Key: key, ExpiresAt: bucket.ExpiresAt}
		column := "attempts"
		if reserve {
			column = "in_flight"
		}
		return tx.Table(rateTable).Where("bucket_key = ?", key).Update(column, gorm.Expr(column+" + 1")).Error
	})
	return reservation, err
}
func settleLogin(tx *gorm.DB, reservation domain.LoginReservation, success, credentialFailure bool) error {
	var bucket rateBucket
	err := tx.Table(rateTable).Where("bucket_key = ? AND expires_at = ?", reservation.Key, reservation.ExpiresAt).Take(&bucket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if bucket.InFlight == 0 {
		return domain.ErrAuthUnavailable
	}
	bucket.InFlight--
	if success {
		bucket.Attempts = 0
	} else if credentialFailure {
		bucket.Attempts++
	}
	return tx.Table(rateTable).Where("bucket_key = ? AND expires_at = ?", reservation.Key, reservation.ExpiresAt).Updates(map[string]any{"attempts": bucket.Attempts, "in_flight": bucket.InFlight}).Error
}
func (a *Adapter) RejectLogin(ctx context.Context, reservation domain.LoginReservation, credentialFailure bool, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error { return settleLogin(tx, reservation, false, credentialFailure) })
}
