package http

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"math"
	"net"
	stdhttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"

	"github.com/gin-gonic/gin"
)

type AccountHTTPOptions struct {
	RequestTimeout    time.Duration
	PublicOrigin      string
	InsecureLocalHTTP bool
	TrustedProxies    []string
}
type accountHandler struct {
	options AccountHTTPOptions
}

func isAccountRequest(path string) bool {
	return path == "/api/v1/auth" || strings.HasPrefix(path, "/api/v1/auth/")
}
func (h accountHandler) cookieName(preauth bool) string {
	name := "rcc-session"
	if preauth {
		name = "rcc-preauth"
	}
	if !h.options.InsecureLocalHTTP {
		return "__Host-" + name
	}
	return name + "-dev"
}
func (h accountHandler) cookie(c *gin.Context, preauth bool) string {
	value, _ := c.Cookie(h.cookieName(preauth))
	return value
}
func (h accountHandler) setCookie(c *gin.Context, preauth bool, value string, maxAge int) {
	stdhttp.SetCookie(c.Writer, &stdhttp.Cookie{Name: h.cookieName(preauth), Value: value, Path: "/", HttpOnly: true, Secure: !h.options.InsecureLocalHTTP, SameSite: stdhttp.SameSiteLaxMode, MaxAge: maxAge})
}
func (h accountHandler) sameOrigin(c *gin.Context) bool {
	origin := c.GetHeader("Origin")
	if origin != "" {
		return origin == h.options.PublicOrigin
	}
	referer, err := url.Parse(c.GetHeader("Referer"))
	return err == nil && referer.User == nil && referer.Scheme+"://"+referer.Host == h.options.PublicOrigin
}
func registerAccountRoutes(router *gin.Engine, auth *application.Authentication, options AccountHTTPOptions) {
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 4 * time.Second
	}
	h := accountHandler{options: options}
	group := router.Group("/api/v1/auth")
	group.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ctx, cancel := stdcontext.WithTimeout(c.Request.Context(), options.RequestTimeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		if c.Request.Method != "GET" && !h.sameOrigin(c) {
			writeAuthError(c, application.ErrCSRF)
			c.Abort()
			return
		}
		c.Next()
	})
	group.GET("/csrf", func(c *gin.Context) {
		token, csrf, err := auth.Prepare(c.Request.Context())
		if err != nil {
			writeAuthError(c, err)
			return
		}
		h.setCookie(c, true, token, 600)
		c.JSON(200, gin.H{"csrf_token": csrf})
	})
	group.POST("/register", func(c *gin.Context) {
		preauth := h.cookie(c, true)
		if err := auth.CheckPreauth(c.Request.Context(), preauth, c.GetHeader("X-CSRF-Token")); err != nil {
			writeAuthError(c, err)
			return
		}
		if err := auth.AdmitRegistration(c.Request.Context(), h.sourceIP(c)); err != nil {
			writeAuthError(c, err)
			return
		}
		var body struct {
			Username    string          `json:"username"`
			Email       string          `json:"email"`
			Password    string          `json:"password"`
			DisplayName json.RawMessage `json:"display_name"`
		}
		if err := decodeRequest(c, &body); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		var displayName *string
		if body.DisplayName != nil {
			var value string
			if err := json.Unmarshal(body.DisplayName, &value); err != nil {
				writeRequestDecodeError(c, err)
				return
			}
			displayName = &value
		}
		result, err := auth.Register(c.Request.Context(), application.Registration{Username: body.Username, Email: body.Email, Password: body.Password, DisplayName: displayName}, preauth, h.cookie(c, false))
		if err != nil {
			writeAuthError(c, err)
			return
		}
		h.signedIn(c, result, 201)
	})
	group.POST("/login", func(c *gin.Context) {
		preauth := h.cookie(c, true)
		if err := auth.CheckPreauth(c.Request.Context(), preauth, c.GetHeader("X-CSRF-Token")); err != nil {
			writeAuthError(c, err)
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeRequest(c, &body); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		reservation, err := auth.AdmitLogin(c.Request.Context(), h.sourceIP(c), body.Username)
		if err != nil {
			writeAuthError(c, err)
			return
		}
		result, err := auth.LoginAttempt(c.Request.Context(), body.Username, body.Password, preauth, h.cookie(c, false), reservation)
		if err != nil {
			writeAuthError(c, err)
			return
		}
		h.signedIn(c, result, 200)
	})
	group.GET("/session", func(c *gin.Context) {
		result, err := auth.Current(c.Request.Context(), h.cookie(c, false))
		if err != nil {
			writeAuthError(c, err)
			return
		}
		c.JSON(200, identityResponse(result))
	})
	group.POST("/logout", func(c *gin.Context) {
		if err := auth.Logout(c.Request.Context(), h.cookie(c, false), c.GetHeader("X-CSRF-Token")); err != nil {
			writeAuthError(c, err)
			return
		}
		h.setCookie(c, false, "", -1)
		c.Status(204)
	})
}
func (h accountHandler) signedIn(c *gin.Context, result application.AuthenticationResult, status int) {
	h.setCookie(c, false, result.Token, 8*60*60)
	h.setCookie(c, true, "", -1)
	c.JSON(status, identityResponse(result))
}
func identityResponse(result application.AuthenticationResult) gin.H {
	account := result.Account
	return gin.H{"account": gin.H{"id": account.ID, "username": account.Username, "display_name": account.DisplayName, "email": account.Email, "email_verified": false, "status": "enabled"}, "csrf_token": result.CSRF, "idle_expires_at": result.Session.LastActiveAt.Add(30 * time.Minute), "expires_at": result.Session.ExpiresAt}
}
func writeAuthError(c *gin.Context, err error) {
	var rate *application.AuthenticationRateLimited
	switch {
	case errors.As(err, &rate):
		c.Header("Retry-After", formatSeconds(rate.RetryAfter))
		writeError(c, 429, "auth_rate_limited", "too many attempts; wait before trying again")
	case errors.Is(err, application.ErrAccountFields):
		writeError(c, 400, "invalid_account_fields", "account fields do not meet the required format or length")
	case errors.Is(err, application.ErrAccountConflict):
		writeError(c, 409, "account_conflict", "username or email is already in use")
	case errors.Is(err, application.ErrCredentials):
		writeError(c, 401, "invalid_credentials", "username or password is incorrect")
	case errors.Is(err, application.ErrSession):
		writeError(c, 401, "session_invalid", "a valid login session is required")
	case errors.Is(err, application.ErrCSRF):
		writeError(c, 403, "csrf_invalid", "valid same-origin CSRF credentials are required")
	case errors.Is(err, application.ErrAuthTimeout):
		writeError(c, 504, "auth_timeout", "authentication service timed out")
	default:
		writeError(c, 503, "auth_unavailable", "authentication service is unavailable")
	}
}
func formatSeconds(duration time.Duration) string {
	return strconv.Itoa(int(math.Max(1, math.Ceil(duration.Seconds()))))
}

// Client IP starts at the socket peer. Forwarded headers are accepted only from
// explicitly trusted proxies, scanning the chain back toward the first untrusted hop.
func (h accountHandler) sourceIP(c *gin.Context) string {
	peer, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		peer = c.Request.RemoteAddr
	}
	trusted := func(value string) bool {
		ip := net.ParseIP(value)
		if ip == nil {
			return false
		}
		for _, cidr := range h.options.TrustedProxies {
			_, network, err := net.ParseCIDR(cidr)
			if err == nil && network.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer
	}
	chain := strings.Split(c.GetHeader("X-Forwarded-For"), ",")
	for index := len(chain) - 1; index >= 0; index-- {
		candidate := strings.TrimSpace(chain[index])
		if net.ParseIP(candidate) == nil {
			return peer
		}
		peer = candidate
		if !trusted(peer) {
			break
		}
	}
	return peer
}
