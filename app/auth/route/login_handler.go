package route

import (
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/idpsession"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

//go:embed templates/login.html
var loginTemplateFS embed.FS

type loginPageData struct {
	Error    string
	Username string
}

// loginHandler serves GET/POST /login for IdP resource owner authentication.
type loginHandler struct {
	authn         *service.AuthenticationService
	issuer        string
	secureCookies bool
	tmpl          *template.Template
}

func newLoginHandler(authn *service.AuthenticationService, issuer string, secureCookies bool) *loginHandler {
	tmpl := template.Must(template.ParseFS(loginTemplateFS, "templates/login.html"))
	return &loginHandler{authn: authn, issuer: issuer, secureCookies: secureCookies, tmpl: tmpl}
}

func (h *loginHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, loginPageData{})
}

func (h *loginHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	sess, err := h.authn.Login(r.Context(), username, password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			h.render(w, loginPageData{Error: "Invalid username or password.", Username: username})
			return
		}
		slog.Error("route: login", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}

	setSessionCookie(w, sess.ID, h.secureCookies)

	if pendingID := readPendingCookie(r); pendingID != "" {
		rawQuery, err := h.authn.ConsumePendingAuthorize(r.Context(), pendingID)
		clearPendingCookie(w, h.secureCookies)
		if err == nil {
			if location, ok := pendingAuthorizeLocation(h.issuer, rawQuery); ok {
				//nolint:gosec // G710: location is rebuilt from server-side pending store and whitelisted OAuth query keys only.
				http.Redirect(w, r, location, http.StatusFound)
				return
			}
		}
		if err != nil && !errors.Is(err, idpsession.ErrNotFound) {
			slog.Error("route: login: consume pending authorize", "error", err)
		}
	}

	http.Redirect(w, r, issuerPath(h.issuer, "/login?signed_in=1"), http.StatusFound)
}

func (h *loginHandler) render(w http.ResponseWriter, data loginPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := h.tmpl.Execute(w, data); err != nil {
		slog.Error("route: login: render template", "error", err)
	}
}
