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

	dummyHash string
}

// NewHandler constructs an auth Handler. If dbEnabled is false, handlers return 503.
func NewHandler(log *slog.Logger, pool *pgxpool.Pool, cfg Config, sessCfg session.Config, dbEnabled bool) (*Handler, error) {
	if log == nil {
		log = slog.Default()
	}

	h := &Handler{
		log:       log,
		cfg:       cfg,
		dbEnabled: dbEnabled,
		pool:      pool,
		sessCfg:   sessCfg,
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
	if blocked, retryAfter, err := h.checkLoginIPThrottle(ctx, ip, now); err == nil && blocked {
		h.auditLoginRateLimited(ctx, nil, ip, ua, identifier, retryAfter)
		writeRateLimited(w, retryAfter)
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

	if blocked, retryAfter, err := h.checkLoginUserThrottle(ctx, userAuth.User.ID, now); err == nil && blocked {
		h.auditLoginRateLimited(ctx, &userAuth.User.ID, ip, ua, identifier, retryAfter)
		writeRateLimited(w, retryAfter)
		return
	}

	okPw, err := identity.VerifyPassword(password, userAuth.PasswordHash)
	if err != nil || !okPw {
		h.auditLoginFailed(ctx, &userAuth.User.ID, ip, ua, identifier, "bad_password")
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
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

	writeJSON(w, http.StatusOK, loginResponse{
		User:    toUserResponse(userAuth.User),
		Session: toSessionResponse(issued),
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
	if err := decodeJSON(w, r, h.cfg.MaxBodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
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

	writeJSON(w, http.StatusOK, refreshResponse{
		Session: toSessionResponse(issued),
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

	writeJSON(w, http.StatusOK, inviteConsumeResponse{
		User: toUserResponse(res.User),
		Session: sessionResponse{
			SessionID:        res.Session.ID,
			AccessToken:      accessToken,
			AccessExpiresAt:  accessExp,
			RefreshToken:     res.RefreshToken,
			RefreshExpiresAt: res.Session.ExpiresAt,
		},
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
	if platform == session.PlatformUnknown {
		platform = session.PlatformWeb
	}
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
