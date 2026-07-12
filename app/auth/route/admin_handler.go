package route

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// adminError is the JSON body returned for admin API failures.
type adminError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// adminHandler handles /admin/clients and /admin/users management routes.
// All routes are protected by a static API key (ADMIN_API_KEY env)
// checked via requireAdminAuth.
type adminHandler struct {
	svc    *service.AdminService
	apiKey string
}

// createClientRequest is the JSON body for POST /admin/clients.
type createClientRequest struct {
	ClientID      string   `json:"client_id"`
	RedirectURIs  []string `json:"redirect_uris"`
	AllowedScopes []string `json:"allowed_scopes"`
	ResponseTypes []string `json:"response_types"`
	GrantTypes    []string `json:"grant_types"`
	// ClientSecret is optional. Providing it creates a confidential
	// client (bcrypt-hashed); omitting it creates a public client.
	ClientSecret string `json:"client_secret,omitempty"`
}

// createClientResponse is the JSON body returned on success for
// POST /admin/clients.
type createClientResponse struct {
	ClientID       string `json:"client_id"`
	IsConfidential bool   `json:"is_confidential"`
}

// createUserRequest is the JSON body for POST /admin/users.
type createUserRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

// createUserResponse is the JSON body returned on success for
// POST /admin/users.
type createUserResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type listUserItem struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

type listUsersResponse struct {
	Users []listUserItem `json:"users"`
}

type listClientItem struct {
	ClientID       string   `json:"client_id"`
	RedirectURIs   []string `json:"redirect_uris"`
	AllowedScopes  []string `json:"allowed_scopes"`
	ResponseTypes  []string `json:"response_types"`
	GrantTypes     []string `json:"grant_types"`
	IsConfidential bool     `json:"is_confidential"`
}

type listClientsResponse struct {
	Clients []listClientItem `json:"clients"`
}

// requireAdminAuth is middleware that enforces admin authentication.
// It accepts the API key as either:
//   - Authorization: Bearer <key>  (RFC 6750 §2.1)
//   - X-Admin-Key: <key>
//
// The check is fail-closed: if apiKey is empty (ADMIN_API_KEY not
// set), every request is rejected with 401. Comparison uses
// crypto/subtle.ConstantTimeCompare to avoid timing leaks.
func requireAdminAuth(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := extractAdminKey(r)
		if apiKey == "" ||
			len(presented) != len(apiKey) ||
			subtle.ConstantTimeCompare([]byte(presented), []byte(apiKey)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="admin"`)
			writeJSON(w, http.StatusUnauthorized, adminError{
				Error:            "unauthorized",
				ErrorDescription: "valid ADMIN_API_KEY required",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractAdminKey reads the presented API key from either the
// Authorization: Bearer header or the X-Admin-Key header.
// Returns an empty string when neither is present.
func extractAdminKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if rest, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return rest
		}
	}
	return r.Header.Get("X-Admin-Key")
}

// handleCreateClient handles POST /admin/clients.
// It parses the JSON body, builds a domain Client (public or
// confidential), and delegates to AdminService.CreateClient.
func (h *adminHandler) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req createClientRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	cid, err := client.ParseClientID(req.ClientID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, adminError{
			Error:            "invalid_request",
			ErrorDescription: "client_id is missing or invalid",
		})
		return
	}

	redirectURIs := make([]client.RedirectURI, 0, len(req.RedirectURIs))
	for _, raw := range req.RedirectURIs {
		uri, err := client.NewRedirectURI(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, adminError{
				Error:            "invalid_request",
				ErrorDescription: "redirect_uri " + raw + " is not a valid absolute URI",
			})
			return
		}
		redirectURIs = append(redirectURIs, uri)
	}

	var c *client.Client
	if req.ClientSecret != "" {
		c, err = client.NewConfidential(cid, redirectURIs, req.AllowedScopes, req.ResponseTypes, req.GrantTypes, req.ClientSecret)
		if err != nil {
			slog.Error("route: admin: hash client secret", "error", err)
			writeJSON(w, http.StatusInternalServerError, adminError{Error: "server_error"})
			return
		}
	} else {
		c = client.New(cid, redirectURIs, req.AllowedScopes, req.ResponseTypes, req.GrantTypes)
	}

	if err := h.svc.CreateClient(r.Context(), c); err != nil {
		slog.Error("route: admin: create client", "client_id", req.ClientID, "error", err)
		writeJSON(w, http.StatusInternalServerError, adminError{Error: "server_error"})
		return
	}

	writeJSON(w, http.StatusCreated, createClientResponse{
		ClientID:       c.ID().String(),
		IsConfidential: c.IsConfidential(),
	})
}

// handleCreateUser handles POST /admin/users.
// It parses the JSON body, builds a domain User (with bcrypt-hashed
// password via user.New), and delegates to AdminService.CreateUser.
func (h *adminHandler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	uid, err := user.ParseUserID(req.UserID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, adminError{
			Error:            "invalid_request",
			ErrorDescription: "user_id is missing or invalid",
		})
		return
	}

	username, err := user.NewUsername(req.Username)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, adminError{
			Error:            "invalid_request",
			ErrorDescription: "username is missing or invalid",
		})
		return
	}

	if req.Password == "" {
		writeJSON(w, http.StatusBadRequest, adminError{
			Error:            "invalid_request",
			ErrorDescription: "password is required",
		})
		return
	}

	profile, err := user.NewProfile(req.Name, req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, adminError{
			Error:            "invalid_request",
			ErrorDescription: "name and email are required",
		})
		return
	}

	u, err := user.New(uid, username, req.Password, profile)
	if err != nil {
		slog.Error("route: admin: build user", "error", err)
		writeJSON(w, http.StatusInternalServerError, adminError{Error: "server_error"})
		return
	}

	if err := h.svc.CreateUser(r.Context(), u); err != nil {
		slog.Error("route: admin: create user", "user_id", req.UserID, "error", err)
		writeJSON(w, http.StatusInternalServerError, adminError{Error: "server_error"})
		return
	}

	writeJSON(w, http.StatusCreated, createUserResponse{
		UserID:   u.ID().String(),
		Username: u.Username().String(),
	})
}

// handleListUsers handles GET /admin/users.
func (h *adminHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		slog.Error("route: admin: list users", "error", err)
		writeJSON(w, http.StatusInternalServerError, adminError{Error: "server_error"})
		return
	}

	items := make([]listUserItem, 0, len(users))
	for _, u := range users {
		items = append(items, listUserItem{
			UserID:   u.ID().String(),
			Username: u.Username().String(),
			Name:     u.Profile().Name(),
			Email:    u.Profile().Email(),
		})
	}

	writeJSON(w, http.StatusOK, listUsersResponse{Users: items})
}

// handleListClients handles GET /admin/clients.
func (h *adminHandler) handleListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.svc.ListClients(r.Context())
	if err != nil {
		slog.Error("route: admin: list clients", "error", err)
		writeJSON(w, http.StatusInternalServerError, adminError{Error: "server_error"})
		return
	}

	items := make([]listClientItem, 0, len(clients))
	for _, c := range clients {
		redirectURIs := make([]string, 0, len(c.RedirectURIs()))
		for _, uri := range c.RedirectURIs() {
			redirectURIs = append(redirectURIs, uri.String())
		}
		items = append(items, listClientItem{
			ClientID:       c.ID().String(),
			RedirectURIs:   redirectURIs,
			AllowedScopes:  c.AllowedScopes(),
			ResponseTypes:  c.ResponseTypes(),
			GrantTypes:     c.GrantTypes(),
			IsConfidential: c.IsConfidential(),
		})
	}

	writeJSON(w, http.StatusOK, listClientsResponse{Clients: items})
}
