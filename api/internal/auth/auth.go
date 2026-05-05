package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const userCtxKey ctxKey = "user_id"

type Service struct {
	secret      []byte
	workerToken string
	tokenTTL    time.Duration
}

func New(secret, workerToken string) *Service {
	return &Service{
		secret:      []byte(secret),
		workerToken: workerToken,
		tokenTTL:    24 * time.Hour,
	}
}

func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(h), err
}

func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func (s *Service) Issue(userID int64) (string, time.Time, error) {
	exp := time.Now().Add(s.tokenTTL)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": exp.Unix(),
		"iat": time.Now().Unix(),
	})
	signed, err := tok.SignedString(s.secret)
	return signed, exp, err
}

func (s *Service) parse(token string) (int64, error) {
	tok, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("bad signing method")
		}
		return s.secret, nil
	})
	if err != nil || !tok.Valid {
		return 0, errors.New("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("bad claims")
	}
	switch v := claims["sub"].(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	}
	return 0, errors.New("bad sub")
}

func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		uid, err := s.parse(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) RequireWorker(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Worker-Token") != s.workerToken {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserID(ctx context.Context) int64 {
	v, _ := ctx.Value(userCtxKey).(int64)
	return v
}
