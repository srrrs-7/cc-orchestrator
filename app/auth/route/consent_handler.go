package route

import (
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/idpsession"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

//go:embed templates/consent.html
var consentTemplateFS embed.FS

type consentPageData struct {
	ClientID string
	Username string
	Scopes   []string
}

type consentHandler struct {
	svc           *service.AuthorizationService
	authn         *service.AuthenticationService
	consent       *service.ConsentService
	issuer        string
	secureCookies bool
	tmpl          *template.Template
}

func newConsentHandler(
	svc *service.AuthorizationService,
	authn *service.AuthenticationService,
	consentSvc *service.ConsentService,
	issuer string,
	secureCookies bool,
) *consentHandler {
	tmpl := template.Must(template.ParseFS(consentTemplateFS, "templates/consent.html"))
	return &consentHandler{
		svc:           svc,
		authn:         authn,
		consent:       consentSvc,
		issuer:        issuer,
		secureCookies: secureCookies,
		tmpl:          tmpl,
	}
}

func (h *consentHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	owner, req, ok := h.loadPendingConsentRequest(w, r)
	if !ok {
		return
	}
	scopes, err := scopeLabels(req.Scope)
	if err != nil {
		slog.Error("route: consent: parse scope", "error", err)
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_scope"})
		return
	}
	h.render(w, consentPageData{
		ClientID: req.ClientID,
		Username: owner.Username().String(),
		Scopes:   scopes,
	})
}

func (h *consentHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return
	}
	owner, req, ok := h.loadPendingConsentRequest(w, r)
	if !ok {
		return
	}

	switch r.FormValue("action") {
	case "accept":
		if err := h.consent.Grant(r.Context(), owner.ID(), req.ClientID, req.Scope); err != nil {
			slog.Error("route: consent: grant", "error", err)
			writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
			return
		}
		h.resumeAuthorize(w, r)
	case "deny":
		verified, err := h.svc.ValidateAuthorize(r.Context(), req)
		h.clearPending(w, r)
		if err != nil {
			writeAuthorizeError(w, r, verified.RedirectURI, req.State, err)
			return
		}
		writeAuthorizeError(w, r, verified.RedirectURI, req.State, service.ErrAccessDenied)
	default:
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
	}
}

func (h *consentHandler) loadPendingConsentRequest(w http.ResponseWriter, r *http.Request) (*user.User, service.AuthorizeRequest, bool) {
	owner, err := h.authn.UserFromSession(r.Context(), readSessionCookie(r))
	if err != nil {
		if errors.Is(err, idpsession.ErrNotFound) {
			http.Redirect(w, r, issuerPath(h.issuer, "/login"), http.StatusFound)
			return nil, service.AuthorizeRequest{}, false
		}
		slog.Error("route: consent: session lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return nil, service.AuthorizeRequest{}, false
	}

	rawQuery, ok := h.readPendingQuery(w, r)
	if !ok {
		return nil, service.AuthorizeRequest{}, false
	}
	req, err := authorizeRequestFromRawQuery(rawQuery)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return nil, service.AuthorizeRequest{}, false
	}
	if _, err := h.svc.ValidateAuthorize(r.Context(), req); err != nil {
		writeAuthorizeError(w, r, "", req.State, err)
		return nil, service.AuthorizeRequest{}, false
	}
	return owner, req, true
}

func (h *consentHandler) readPendingQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	pendingID := readPendingCookie(r)
	if pendingID == "" {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request", ErrorDescription: "missing pending authorization"})
		return "", false
	}
	rawQuery, err := h.authn.PeekPendingAuthorize(r.Context(), pendingID)
	if err != nil {
		if errors.Is(err, idpsession.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request", ErrorDescription: "authorization request expired"})
			return "", false
		}
		slog.Error("route: consent: load pending", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return "", false
	}
	return rawQuery, true
}

func (h *consentHandler) resumeAuthorize(w http.ResponseWriter, r *http.Request) {
	pendingID := readPendingCookie(r)
	if pendingID == "" {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return
	}
	rawQuery, err := h.authn.ConsumePendingAuthorize(r.Context(), pendingID)
	clearPendingCookie(w, h.secureCookies)
	if err != nil {
		slog.Error("route: consent: consume pending", "error", err)
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return
	}
	if location, ok := pendingAuthorizeLocation(h.issuer, rawQuery); ok {
		//nolint:gosec // G710: location is rebuilt from server-side pending store and whitelisted OAuth query keys only.
		http.Redirect(w, r, location, http.StatusFound)
		return
	}
	writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
}

func (h *consentHandler) clearPending(w http.ResponseWriter, r *http.Request) {
	if pendingID := readPendingCookie(r); pendingID != "" {
		_, _ = h.authn.ConsumePendingAuthorize(r.Context(), pendingID)
		clearPendingCookie(w, h.secureCookies)
	}
}

func (h *consentHandler) render(w http.ResponseWriter, data consentPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := h.tmpl.Execute(w, data); err != nil {
		slog.Error("route: consent: render template", "error", err)
	}
}

func scopeLabels(scopeRaw string) ([]string, error) {
	scope, err := authcode.ParseScope(scopeRaw)
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(scope.Values()))
	for _, v := range scope.Values() {
		switch v {
		case authcode.ScopeOpenID:
			labels = append(labels, "Verify your identity (openid)")
		case "profile":
			labels = append(labels, "View your profile name")
		case "email":
			labels = append(labels, "View your email address")
		default:
			labels = append(labels, v)
		}
	}
	return labels, nil
}

func authorizeRequestFromRawQuery(rawQuery string) (service.AuthorizeRequest, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return service.AuthorizeRequest{}, err
	}
	return service.AuthorizeRequest{
		ResponseType:        values.Get("response_type"),
		ClientID:            values.Get("client_id"),
		RedirectURI:         values.Get("redirect_uri"),
		Scope:               values.Get("scope"),
		State:               values.Get("state"),
		Nonce:               values.Get("nonce"),
		CodeChallenge:       values.Get("code_challenge"),
		CodeChallengeMethod: values.Get("code_challenge_method"),
		LoginHint:           values.Get("login_hint"),
	}, nil
}
