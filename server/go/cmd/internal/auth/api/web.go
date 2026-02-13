package authapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"arc/cmd/internal/auth/session"
)

func (h *Handler) shouldUseWebCookieTransport(platform session.Platform) bool {
	return h != nil && h.cfg.WebRefreshCookieEnabled && platform == session.PlatformWeb
}

func (h *Handler) setWebSessionCookies(w http.ResponseWriter, refreshToken string, refreshExp time.Time) (string, error) {
	csrf, err := newOpaqueWebToken(32)
	if err != nil {
		return "", err
	}

	h.setRefreshCookie(w, refreshToken, refreshExp)
	h.setCSRFCookie(w, csrf, refreshExp)
	return csrf, nil
}

func (h *Handler) clearWebSessionCookies(w http.ResponseWriter) {
	if h == nil || w == nil || !h.cfg.WebRefreshCookieEnabled {
		return
	}
	h.expireCookie(w, h.cfg.RefreshCookieName, true)
	h.expireCookie(w, h.cfg.CSRFCookieName, false)
}

func (h *Handler) refreshTokenFromCookie(r *http.Request) (string, bool) {
	if h == nil || r == nil || !h.cfg.WebRefreshCookieEnabled {
		return "", false
	}
	c, err := r.Cookie(h.cfg.RefreshCookieName)
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(c.Value)
	if v == "" {
		return "", false
	}
	return v, true
}

func (h *Handler) csrfDoubleSubmitValid(r *http.Request) bool {
	if h == nil || r == nil || !h.cfg.WebRefreshCookieEnabled {
		return false
	}
	c, err := r.Cookie(h.cfg.CSRFCookieName)
	if err != nil {
		return false
	}
	cv := strings.TrimSpace(c.Value)
	hv := strings.TrimSpace(r.Header.Get(h.cfg.CSRFHeaderName))
	if cv == "" || hv == "" {
		return false
	}
	return secureStringEqual(cv, hv)
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, value string, exp time.Time) {
	if h == nil || w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.RefreshCookieName,
		Value:    value,
		Path:     h.cfg.CookiePath,
		Domain:   h.cfg.CookieDomain,
		Expires:  exp,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
	})
}

func (h *Handler) setCSRFCookie(w http.ResponseWriter, value string, exp time.Time) {
	if h == nil || w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CSRFCookieName,
		Value:    value,
		Path:     h.cfg.CookiePath,
		Domain:   h.cfg.CookieDomain,
		Expires:  exp,
		HttpOnly: false,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
	})
}

func (h *Handler) expireCookie(w http.ResponseWriter, name string, httpOnly bool) {
	if h == nil || w == nil || strings.TrimSpace(name) == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     h.cfg.CookiePath,
		Domain:   h.cfg.CookieDomain,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: httpOnly,
		Secure:   h.cfg.CookieSecure,
		SameSite: h.cfg.CookieSameSite,
	})
}

func newOpaqueWebToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 32
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func secureStringEqual(a, b string) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
