package api

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"
)

func (h *Handler) auditLoginFailed(ctx context.Context, userID *string, ip net.IP, ua string, identifier string, reason string) {
	h.insertAudit(ctx, "auth.login.failed", userID, nil, ip, ua, map[string]any{
		"identifier": identifier,
		"reason":     reason,
	})
}

func (h *Handler) auditLoginSuccess(ctx context.Context, userID *string, sessionID string, ip net.IP, ua string, identifier string) {
	h.insertAudit(ctx, "auth.login.success", userID, &sessionID, ip, ua, map[string]any{
		"identifier": identifier,
	})
}

func (h *Handler) auditLoginRateLimited(ctx context.Context, userID *string, ip net.IP, ua string, identifier string, retryAfter time.Duration) {
	h.insertAudit(ctx, "auth.login.rate_limited", userID, nil, ip, ua, map[string]any{
		"identifier":    identifier,
		"retry_after_s": int64(retryAfter.Seconds()),
	})
}

func (h *Handler) auditRefreshSuccess(ctx context.Context, sessionID string, ip net.IP, ua string) {
	h.insertAudit(ctx, "auth.refresh.success", nil, &sessionID, ip, ua, nil)
}

func (h *Handler) auditRefreshReuse(ctx context.Context, ip net.IP, ua string) {
	h.insertAudit(ctx, "auth.refresh.reuse_detected", nil, nil, ip, ua, nil)
}

func (h *Handler) auditLogout(ctx context.Context, userID string, sessionID string, ip net.IP, ua string) {
	h.insertAudit(ctx, "auth.logout", &userID, &sessionID, ip, ua, nil)
}

func (h *Handler) auditLogoutAll(ctx context.Context, userID string, ip net.IP, ua string) {
	h.insertAudit(ctx, "auth.logout_all", &userID, nil, ip, ua, nil)
}

func (h *Handler) auditInviteCreated(ctx context.Context, userID string, inviteID string, ip net.IP, ua string) {
	h.insertAudit(ctx, "auth.invite.created", &userID, nil, ip, ua, map[string]any{
		"invite_id": inviteID,
	})
}

func (h *Handler) auditInviteConsumed(ctx context.Context, userID string, inviteID string, ip net.IP, ua string) {
	h.insertAudit(ctx, "auth.invite.consumed", &userID, nil, ip, ua, map[string]any{
		"invite_id": inviteID,
	})
}

func (h *Handler) insertAudit(ctx context.Context, action string, userID *string, sessionID *string, ip net.IP, ua string, meta map[string]any) {
	if h == nil || h.pool == nil || !h.dbEnabled {
		return
	}

	action = strings.TrimSpace(action)
	if action == "" {
		return
	}

	var ipVal any
	if ip != nil {
		ipVal = ip.String()
	}

	var metaVal *string
	if len(meta) > 0 {
		if b, err := json.Marshal(meta); err == nil {
			s := string(b)
			metaVal = &s
		}
	}

	_, err := h.pool.Exec(ctx, `
		INSERT INTO arc.audit_log (
			user_id, session_id, action, created_at, ip, user_agent, meta
		) VALUES ($1, $2, $3, now(), $4, $5, $6::jsonb)
	`, userID, sessionID, action, ipVal, trimOrNil(ua), metaVal)
	if err != nil {
		h.log.Error("auth.audit.insert.fail", "err", err, "action", action)
	}
}

func trimOrNil(s string) any {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return v
}
