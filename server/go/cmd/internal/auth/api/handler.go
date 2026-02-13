package authapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"arc/cmd/identity"
	"arc/cmd/internal/auth/session"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler wires HTTP auth endpoints to identity/session services.
type Handler struct {
	log *slog.Logger
	cfg Config

	dbEnabled bool
	pool      *pgxpool.Pool

	identity *identity.PostgresStore
	sessions *session.Service
	sessCfg  session.Config

	emailSender EmailSender
	captcha     CaptchaVerifier

	dummyHash string
}

// HandlerOption configures optional auth handler dependencies.
type HandlerOption func(*Handler)

// WithEmailSender overrides the default no-op email sender.
func WithEmailSender(sender EmailSender) HandlerOption {
	return func(h *Handler) {
		if h == nil || sender == nil {
			return
		}
		h.emailSender = sender
	}
}

// WithCaptchaVerifier overrides the default no-op captcha verifier.
func WithCaptchaVerifier(verifier CaptchaVerifier) HandlerOption {
	return func(h *Handler) {
		if h == nil || verifier == nil {
			return
		}
		h.captcha = verifier
	}
}

// NewHandler constructs an auth Handler. If dbEnabled is false, handlers return 503.
func NewHandler(log *slog.Logger, pool *pgxpool.Pool, cfg Config, sessCfg session.Config, dbEnabled bool, opts ...HandlerOption) (*Handler, error) {
	if log == nil {
		log = slog.Default()
	}

	h := &Handler{
		log:         log,
		cfg:         cfg,
		dbEnabled:   dbEnabled,
		pool:        pool,
		sessCfg:     sessCfg,
		emailSender: NoopEmailSender{},
		captcha:     NoopCaptchaVerifier{},
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(h)
	}

	if !dbEnabled {
		return h, nil
	}
	if pool == nil {
		return nil, errors.New("auth: nil db pool")
	}

	idStore, err := identity.NewPostgresStore(pool)
	if err != nil {
		return nil, err
	}
	h.identity = idStore

	tokens, err := session.NewPasetoV4PublicManager(sessCfg)
	if err != nil {
		return nil, err
	}
	sessStore := session.NewPostgresStore(pool)
	h.sessions = session.NewService(sessCfg, pool, sessStore, tokens)

	// Dummy hash for timing-resistant login checks.
	if hash, err := identity.HashPassword("dummy-password-for-timing-only", identity.DefaultArgon2idParams()); err == nil {
		h.dummyHash = hash
	}

	return h, nil
}

// Register wires auth routes onto the provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	if h == nil || mux == nil {
		return
	}
	mux.HandleFunc("/auth/login", h.handleLogin)
	mux.HandleFunc("/auth/refresh", h.handleRefresh)
	mux.HandleFunc("/auth/logout", h.handleLogout)
	mux.HandleFunc("/auth/logout_all", h.handleLogoutAll)
	mux.HandleFunc("/auth/invites/create", h.handleInviteCreate)
	mux.HandleFunc("/auth/invites/consume", h.handleInviteConsume)
	mux.HandleFunc("/me", h.handleMe)
}

// SessionService returns the underlying session service (may be nil when DB is disabled).
func (h *Handler) SessionService() *session.Service {
	if h == nil {
		return nil
	}
	return h.sessions
}

// ---- handlers ----

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	var req loginRequest
	if err := decodeJSON(w, r, h.cfg.MaxBodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}

	username, email, password, platform, rememberMe, ok := normalizeLoginRequest(req)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "username/email and password are required")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	ip := clientIP(r, h.cfg.TrustProxy)
	ua := strings.TrimSpace(r.UserAgent())
	identifier := loginIdentifier(username, email)

	// IP-based throttling before DB lookup.
	if blocked, retryAfter, err := h.checkLoginIPThrottle(ctx, ip, now); err != nil {
		h.log.Error("auth.login.throttle_ip.fail", "err", err)
		writeError(w, http.StatusServiceUnavailable, "server_busy", "please retry later")
		return
	} else if blocked {
		h.auditLoginRateLimited(ctx, nil, ip, ua, identifier, retryAfter)
		writeRateLimited(w, retryAfter)
		return
	}
	// Identifier-based throttling before DB lookup to avoid extra auth DB load.
	if blocked, retryAfter, err := h.checkLoginIdentifierThrottle(ctx, identifier, now); err != nil {
		h.log.Error("auth.login.throttle_identifier.fail", "err", err)
		writeError(w, http.StatusServiceUnavailable, "server_busy", "please retry later")
		return
	} else if blocked {
		h.auditLoginRateLimited(ctx, nil, ip, ua, identifier, retryAfter)
		writeRateLimited(w, retryAfter)
		return
	}
	if err := h.enforceCaptcha(ctx, req.Captcha, ip); err != nil {
		h.auditLoginFailed(ctx, nil, ip, ua, identifier, "captcha_invalid")
		switch {
		case errors.Is(err, ErrCaptchaRequired), errors.Is(err, ErrCaptchaInvalid):
			writeError(w, http.StatusForbidden, "captcha_invalid", "captcha verification failed")
		default:
			h.log.Error("auth.login.captcha.fail", "err", err)
			writeError(w, http.StatusServiceUnavailable, "server_busy", "please retry later")
		}
		return
	}

	userAuth, err := h.lookupUserForLogin(ctx, username, email)
	if err != nil {
		// Timing resistance: perform a dummy verify when user is missing.
		if h.dummyHash != "" {
			_, _ = identity.VerifyPassword(password, h.dummyHash)
		}
		h.auditLoginFailed(ctx, nil, ip, ua, identifier, "not_found")
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
		return
	}

	okPw, err := identity.VerifyPassword(password, userAuth.PasswordHash)
	if err != nil || !okPw {
		h.auditLoginFailed(ctx, &userAuth.User.ID, ip, ua, identifier, "bad_password")
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
		return
	}
	if err := h.enforceEmailVerified(userAuth.User); err != nil {
		h.auditLoginFailed(ctx, &userAuth.User.ID, ip, ua, identifier, "email_not_verified")
		writeError(w, http.StatusForbidden, "email_not_verified", "email verification required")
		return
	}

	dev := session.DeviceContext{
		Platform:   platform,
		RememberMe: rememberMe,
		UserAgent:  ua,
		IP:         ip,
	}

	issued, err := h.sessions.IssueSession(ctx, now, userAuth.User.ID, dev)
	if err != nil {
		h.log.Error("auth.login.issue_session.fail", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	h.auditLoginSuccess(ctx, &userAuth.User.ID, issued.SessionID, ip, ua, identifier)

	respSession := toSessionResponse(issued)
	if h.shouldUseWebCookieTransport(platform) {
		if _, err := h.setWebSessionCookies(w, issued.RefreshToken, issued.RefreshExp); err != nil {
			h.log.Error("auth.login.web_cookie.fail", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "internal error")
			return
		}
		respSession.RefreshToken = ""
	}

	writeJSON(w, http.StatusOK, loginResponse{
		User:    toUserResponse(userAuth.User),
		Session: respSession,
	})
}

func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	var req refreshRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, h.cfg.MaxBodyBytes, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
			return
		}
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	fromCookie := false
	if cookieToken, ok := h.refreshTokenFromCookie(r); ok {
		fromCookie = true
		if refreshToken == "" {
			refreshToken = cookieToken
		}
	}
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	if fromCookie && !h.csrfDoubleSubmitValid(r) {
		writeError(w, http.StatusForbidden, "csrf_invalid", "missing or invalid csrf token")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	ip := clientIP(r, h.cfg.TrustProxy)
	ua := strings.TrimSpace(r.UserAgent())

	dev := session.DeviceContext{
		Platform:   normalizePlatform(req.Platform),
		RememberMe: req.RememberMe,
		UserAgent:  ua,
		IP:         ip,
	}

	issued, err := h.sessions.RotateRefresh(ctx, now, refreshToken, dev)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrRefreshRateLimited):
			var rlErr session.RefreshRateLimitError
			if errors.As(err, &rlErr) {
				h.auditRefreshRateLimited(ctx, rlErr.SessionID, ip, ua, rlErr.RetryAfter)
				writeRateLimitedError(w, rlErr.RetryAfter, "refresh_rate_limited", "refresh attempted too frequently")
				return
			}
			h.auditRefreshRateLimited(ctx, "", ip, ua, 0)
			writeRateLimitedError(w, 0, "refresh_rate_limited", "refresh attempted too frequently")
			return
		case errors.Is(err, session.ErrRefreshReuseDetected):
			h.auditRefreshReuse(ctx, ip, ua)
			writeError(w, http.StatusUnauthorized, "refresh_reuse_detected", "refresh token reuse detected")
		case errors.Is(err, session.ErrSessionExpired), errors.Is(err, session.ErrSessionRevoked), errors.Is(err, session.ErrSessionNotFound):
			writeError(w, http.StatusUnauthorized, "session_not_active", "session not active")
		default:
			h.log.Error("auth.refresh.fail", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		}
		return
	}

	h.auditRefreshSuccess(ctx, issued.SessionID, ip, ua)

	respSession := toSessionResponse(issued)
	if fromCookie || h.shouldUseWebCookieTransport(dev.Platform) {
		if _, err := h.setWebSessionCookies(w, issued.RefreshToken, issued.RefreshExp); err != nil {
			h.log.Error("auth.refresh.web_cookie.fail", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "internal error")
			return
		}
		respSession.RefreshToken = ""
	}

	writeJSON(w, http.StatusOK, refreshResponse{
		Session: respSession,
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	claims, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	if err := h.sessions.RevokeSession(ctx, now, claims.SessionID); err != nil {
		h.log.Error("auth.logout.fail", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	h.auditLogout(ctx, claims.UserID, claims.SessionID, clientIP(r, h.cfg.TrustProxy), strings.TrimSpace(r.UserAgent()))
	h.clearWebSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleLogoutAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	claims, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	if err := h.sessions.RevokeAll(ctx, now, claims.UserID); err != nil {
		h.log.Error("auth.logout_all.fail", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	h.auditLogoutAll(ctx, claims.UserID, clientIP(r, h.cfg.TrustProxy), strings.TrimSpace(r.UserAgent()))
	h.clearWebSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	claims, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	u, err := h.identity.GetUserByID(ctx, claims.UserID)
	if err != nil {
		if identity.IsNotFound(err) {
			writeError(w, http.StatusUnauthorized, "not_found", "user not found")
			return
		}
		h.log.Error("auth.me.fail", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{User: toUserResponse(u)})
}

func (h *Handler) handleInviteCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	claims, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	var req inviteCreateRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, h.cfg.MaxBodyBytes, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
			return
		}
	}

	ttl := h.cfg.InviteTTL
	if req.ExpiresInSeconds > 0 {
		ttl = time.Duration(req.ExpiresInSeconds) * time.Second
	}
	if ttl > h.cfg.InviteMaxTTL {
		ttl = h.cfg.InviteMaxTTL
	}
	if ttl <= 0 {
		ttl = h.cfg.InviteTTL
	}
	maxUses := h.cfg.InviteMaxUses
	if req.MaxUses > 0 {
		maxUses = req.MaxUses
	}
	if maxUses > h.cfg.InviteMaxUsesMax {
		maxUses = h.cfg.InviteMaxUsesMax
	}
	note := trimPtr(req.Note)
	if note != nil && len(*note) > 512 {
		writeError(w, http.StatusBadRequest, "invalid_request", "note is too long")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	res, err := h.identity.CreateInvite(ctx, identity.CreateInviteInput{
		CreatedBy: &claims.UserID,
		TTL:       ttl,
		MaxUses:   maxUses,
		Note:      note,
		Now:       now,
	})
	if err != nil {
		h.log.Error("auth.invite.create.fail", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	h.auditInviteCreated(ctx, claims.UserID, res.Invite.ID, clientIP(r, h.cfg.TrustProxy), strings.TrimSpace(r.UserAgent()))

	writeJSON(w, http.StatusOK, inviteCreateResponse{
		InviteID:    res.Invite.ID,
		InviteToken: res.Token,
		ExpiresAt:   res.Invite.ExpiresAt,
	})
}

func (h *Handler) handleInviteConsume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !h.dbEnabled {
		writeError(w, http.StatusServiceUnavailable, "db_unavailable", "database not configured")
		return
	}

	var req inviteConsumeRequest
	if err := decodeJSON(w, r, h.cfg.MaxBodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}

	if h.cfg.InviteOnly && strings.TrimSpace(req.InviteToken) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "invite_token is required")
		return
	}

	username := trimPtr(req.Username)
	email := trimPtr(req.Email)
	if username == nil && email == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "username or email is required")
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "password is required")
		return
	}

	platform := normalizePlatform(req.Platform)
	rememberMe := req.RememberMe
	ttl := refreshTTL(h.sessCfg, platform, rememberMe)

	ctx := r.Context()
	now := time.Now().UTC()
	ip := clientIP(r, h.cfg.TrustProxy)
	if err := h.enforceCaptcha(ctx, req.Captcha, ip); err != nil {
		switch {
		case errors.Is(err, ErrCaptchaRequired), errors.Is(err, ErrCaptchaInvalid):
			writeError(w, http.StatusForbidden, "captcha_invalid", "captcha verification failed")
		default:
			h.log.Error("auth.invite.consume.captcha.fail", "err", err)
			writeError(w, http.StatusServiceUnavailable, "server_busy", "please retry later")
		}
		return
	}
	ua := strings.TrimSpace(r.UserAgent())
	var uaPtr *string
	if ua != "" {
		uaPtr = &ua
	}
	var ipPtr *net.IP
	if ip != nil {
		ipCopy := ip
		ipPtr = &ipCopy
	}

	res, err := h.identity.ConsumeInviteAndCreateUser(ctx, identity.ConsumeInviteInput{
		Token:      strings.TrimSpace(req.InviteToken),
		Username:   username,
		Email:      email,
		Password:   req.Password,
		Now:        now,
		SessionTTL: ttl,
		Platform:   string(platform),
		UserAgent:  uaPtr,
		IP:         ipPtr,
	})
	if err != nil {
		switch {
		case identity.IsConflict(err):
			writeError(w, http.StatusConflict, "conflict", "username or email already exists")
		case identity.IsInvalidInput(err):
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid input")
		case identity.IsNotActive(err) || identity.IsNotFound(err):
			writeError(w, http.StatusBadRequest, "invalid_invite", "invalid or expired invite")
		default:
			h.log.Error("auth.invite.consume.fail", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		}
		return
	}

	accessToken, accessExp, err := h.sessions.IssueAccessToken(res.User.ID, res.Session.ID, now)
	if err != nil {
		h.log.Error("auth.invite.consume.token.fail", "err", err)
		writeError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	if res.Invite.ID != "" {
		h.auditInviteConsumed(ctx, res.User.ID, res.Invite.ID, ip, ua)
	} else {
		h.insertAudit(ctx, "auth.signup", &res.User.ID, &res.Session.ID, ip, ua, nil)
	}
	h.maybeSendVerificationEmail(ctx, res.User)

	respSession := sessionResponse{
		SessionID:        res.Session.ID,
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExp,
		RefreshToken:     res.RefreshToken,
		RefreshExpiresAt: res.Session.ExpiresAt,
	}
	if h.shouldUseWebCookieTransport(platform) {
		if _, err := h.setWebSessionCookies(w, res.RefreshToken, res.Session.ExpiresAt); err != nil {
			h.log.Error("auth.invite.consume.web_cookie.fail", "err", err)
			writeError(w, http.StatusInternalServerError, "server_error", "internal error")
			return
		}
		respSession.RefreshToken = ""
	}

	writeJSON(w, http.StatusOK, inviteConsumeResponse{
		User:     toUserResponse(res.User),
		Session:  respSession,
		InviteID: res.Invite.ID,
	})
}

// ---- helpers ----

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) (session.AccessClaims, bool) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return session.AccessClaims{}, false
	}
	claims, err := h.sessions.ValidateAccessToken(r.Context(), token, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
		return session.AccessClaims{}, false
	}
	return claims, true
}

func bearerToken(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func normalizePlatform(p string) session.Platform {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "web":
		return session.PlatformWeb
	case "ios":
		return session.PlatformIOS
	case "android":
		return session.PlatformAndroid
	case "desktop":
		return session.PlatformDesktop
	default:
		return session.PlatformUnknown
	}
}

func refreshTTL(cfg session.Config, platform session.Platform, rememberMe bool) time.Duration {
	switch platform {
	case session.PlatformWeb:
		return cfg.RefreshTTLWeb
	case session.PlatformIOS, session.PlatformAndroid, session.PlatformDesktop:
		if rememberMe {
			return cfg.RefreshTTLNative
		}
		return cfg.RefreshTTLNativeShort
	default:
		return cfg.RefreshTTLWeb
	}
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func normalizeLoginRequest(req loginRequest) (username *string, email *string, password string, platform session.Platform, rememberMe bool, ok bool) {
	username = trimPtr(req.Username)
	email = trimPtr(req.Email)
	password = strings.TrimSpace(req.Password)
	if password == "" {
		return nil, nil, "", session.PlatformUnknown, false, false
	}
	if (username == nil && email == nil) || (username != nil && email != nil) {
		return nil, nil, "", session.PlatformUnknown, false, false
	}
	platform = normalizePlatform(req.Platform)
	return username, email, password, platform, req.RememberMe, true
}

func loginIdentifier(username, email *string) string {
	if username != nil {
		return identity.NormalizeUsername(*username)
	}
	if email != nil {
		return identity.NormalizeEmail(*email)
	}
	return ""
}

func (h *Handler) lookupUserForLogin(ctx context.Context, username, email *string) (identity.UserAuth, error) {
	if h.identity == nil {
		return identity.UserAuth{}, identity.OpError{Op: "auth.lookupUser", Kind: identity.ErrNotFound}
	}
	if username != nil {
		return h.identity.GetUserAuthByUsername(ctx, *username)
	}
	if email != nil {
		return h.identity.GetUserAuthByEmail(ctx, *email)
	}
	return identity.UserAuth{}, identity.OpError{Op: "auth.lookupUser", Kind: identity.ErrInvalidInput}
}

func (h *Handler) enforceCaptcha(ctx context.Context, token string, ip net.IP) error {
	if h == nil || !h.cfg.EnableCaptcha {
		return nil
	}
	token = normalizeCaptchaToken(token)
	if token == "" {
		return ErrCaptchaRequired
	}
	if h.captcha == nil {
		return errors.New("captcha verifier not configured")
	}
	if err := h.captcha.Verify(ctx, token, ip); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrCaptchaInvalid
	}
	return nil
}

func (h *Handler) enforceEmailVerified(user identity.User) error {
	if h == nil || !h.cfg.RequireEmailVerified {
		return nil
	}
	if user.Email == nil || strings.TrimSpace(*user.Email) == "" {
		return ErrEmailNotVerified
	}
	if user.EmailVerifiedAt == nil {
		return ErrEmailNotVerified
	}
	return nil
}

func (h *Handler) maybeSendVerificationEmail(ctx context.Context, user identity.User) {
	if h == nil || h.emailSender == nil {
		return
	}
	if user.EmailVerifiedAt != nil || user.Email == nil {
		return
	}
	email := strings.TrimSpace(*user.Email)
	if email == "" {
		return
	}

	if err := h.emailSender.SendEmailVerification(ctx, EmailVerificationMessage{
		UserID: user.ID,
		Email:  email,
	}); err != nil {
		h.log.Error("auth.email_verification.send.fail", "err", err, "user_id", user.ID)
	}
}

func clientIP(r *http.Request, trustProxy bool) net.IP {
	if trustProxy {
		if ip := parseForwardedIP(r.Header.Get("X-Forwarded-For")); ip != nil {
			return ip
		}
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip
		}
	}
	return nil
}

func parseForwardedIP(raw string) net.IP {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		if ip := net.ParseIP(strings.TrimSpace(p)); ip != nil {
			return ip
		}
	}
	return nil
}
