package rest

import (
	"net/http"
	"pricing-platform/internal/auth"
	"pricing-platform/internal/domain"
	"pricing-platform/internal/service"
	"strings"
	"time"
)

// router registers routes behind a per-route permission check.
type router struct {
	m     *http.ServeMux
	authn *service.Auth
}

func (rt router) handle(pattern string, perm domain.Permission, h http.HandlerFunc) {
	if rt.authn == nil || perm == domain.PermPublic {
		rt.m.HandleFunc(pattern, h)
		return
	}
	rt.m.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			reply(w, nil, domain.ErrUnauthorized)
			return
		}
		p, err := rt.authn.Authenticate(r.Context(), strings.TrimSpace(token))
		if err != nil {
			reply(w, nil, err)
			return
		}
		if !p.Allows(perm) {
			reply(w, nil, domain.ErrForbidden)
			return
		}
		h(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

func principal(r *http.Request) domain.Principal {
	p, _ := auth.FromContext[domain.Principal](r.Context())
	return p
}

func remoteHost(r *http.Request) string {
	if i := strings.LastIndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

// noStore keeps token responses out of shared and browser caches.
func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

func routeAuth(rt router, a *service.Auth) {
	rt.handle("POST /v1/auth/register", domain.PermPublic, func(w http.ResponseWriter, r *http.Request) {
		var q domain.Credentials
		if !decode(w, r, 4096, &q) {
			return
		}
		v, e := a.Register(r.Context(), q, remoteHost(r))
		replyStatus(w, http.StatusCreated, v, e)
	})
	rt.handle("POST /v1/auth/login", domain.PermPublic, func(w http.ResponseWriter, r *http.Request) {
		var q domain.Credentials
		if !decode(w, r, 4096, &q) {
			return
		}
		noStore(w)
		v, e := a.Login(r.Context(), q, r.UserAgent())
		reply(w, v, e)
	})
	rt.handle("POST /v1/auth/refresh", domain.PermPublic, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			RefreshToken string `json:"refresh_token"`
		}
		if !decode(w, r, 4096, &q) {
			return
		}
		noStore(w)
		v, e := a.Refresh(r.Context(), q.RefreshToken, r.UserAgent())
		reply(w, v, e)
	})
	rt.handle("POST /v1/auth/logout", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"status": "logged_out"}, a.Logout(r.Context(), principal(r)))
	})
	rt.handle("GET /v1/auth/me", domain.PermRead, func(w http.ResponseWriter, r *http.Request) {
		p := principal(r)
		v := map[string]string{"user_id": p.UserID, "email": p.Email, "role": p.Role, "scope": p.Scope, "api_key_id": p.APIKeyID}
		if p.APIKeyID == "" {
			v["credential"] = "session"
		} else {
			v["credential"] = "api_key"
		}
		reply(w, v, nil)
	})
	rt.handle("POST /v1/auth/api-keys", domain.PermAdmin, func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Name  string `json:"name"`
			Scope string `json:"scope"`
		}
		if !decode(w, r, 4096, &q) {
			return
		}
		noStore(w)
		v, e := a.CreateAPIKey(r.Context(), principal(r), q.Name, q.Scope)
		replyStatus(w, http.StatusCreated, v, e)
	})
	rt.handle("GET /v1/auth/api-keys", domain.PermAdmin, func(w http.ResponseWriter, r *http.Request) {
		v, e := a.APIKeys(r.Context())
		reply(w, v, e)
	})
	rt.handle("DELETE /v1/auth/api-keys/{id}", domain.PermAdmin, func(w http.ResponseWriter, r *http.Request) {
		e := a.RevokeAPIKey(r.Context(), r.PathValue("id"))
		reply(w, map[string]any{"status": "revoked", "revoked_at": time.Now().UTC()}, e)
	})
}
