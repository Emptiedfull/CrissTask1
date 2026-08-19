package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var secret = []byte("yo")

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   MemRole   `json:"role"`
	jwt.RegisteredClaims
}

func GenJWT(id uuid.UUID, role MemRole) (string, error) {
	claims := Claims{
		UserID: id,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(48 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func ValJWT(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

func AuthMiddleware(up http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing/invalid token")
			return
		}

		tokenstring := strings.TrimPrefix(header, "Bearer ")
		claims, err := ValJWT(tokenstring)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), "claims", claims)
		up(w, r.WithContext(ctx))
	}
}

func RequireRole(role MemRole, up http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := getClaims(r)
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "just rlly bad")
			return
		}
		if claims.Role != role {
			writeError(w, http.StatusUnauthorized, "Insufficient permissions")
			return
		}

		up(w, r)

	}
}

func getClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value("claims").(*Claims)
	return claims
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
