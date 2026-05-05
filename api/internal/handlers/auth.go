package handlers

import (
	"net/http"
	"time"

	"github.com/erhan/falcon/api/internal/auth"
	"github.com/erhan/falcon/api/internal/models"
	"github.com/jmoiron/sqlx"
)

type AuthHandler struct {
	DB   *sqlx.DB
	Auth *auth.Service
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
}

// Login authenticates an admin user with email + password and returns a JWT.
//
//	@Summary		Sign in with email + password
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		loginReq	true	"credentials"
//	@Success		200		{object}	loginResp
//	@Failure		400		{object}	errorResp
//	@Failure		401		{object}	errorResp
//	@Router			/api/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(r, &req); err != nil || req.Email == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "email and password required")
		return
	}
	var u models.User
	if err := h.DB.GetContext(r.Context(), &u,
		`SELECT id,email,password_hash,created_at FROM users WHERE lower(email)=lower($1)`, req.Email); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	tok, exp, err := h.Auth.Issue(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	resp := loginResp{Token: tok, ExpiresAt: exp}
	resp.User.ID = u.ID
	resp.User.Email = u.Email
	writeJSON(w, http.StatusOK, resp)
}

// Me returns the authenticated user's profile.
//
//	@Summary		Get current user
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	models.User
//	@Failure		401	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	var u models.User
	if err := h.DB.GetContext(r.Context(), &u,
		`SELECT id,email,password_hash,created_at FROM users WHERE id=$1`, uid); err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
