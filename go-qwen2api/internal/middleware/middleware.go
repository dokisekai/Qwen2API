package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/config"
)

type contextKey string

const (
	IsAdminKey contextKey = "isAdmin"
	APIKeyKey  contextKey = "apiKey"
)

func ApiKeyVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		if key == "" {
			key = r.Header.Get("X-Api-Key")
		}
		if key == "" {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		clean := strings.TrimPrefix(key, "Bearer ")
		valid := false
		isAdmin := false
		for _, k := range config.C.APIKeys {
			if k == clean {
				valid = true
				isAdmin = clean == config.C.AdminKey
				break
			}
		}
		if !valid {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, IsAdminKey, isAdmin)
		ctx = context.WithValue(ctx, APIKeyKey, clean)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AdminKeyVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		if key == "" {
			key = r.Header.Get("X-Api-Key")
		}
		clean := strings.TrimPrefix(key, "Bearer ")
		isAdmin := clean == config.C.AdminKey
		if !isAdmin {
			http.Error(w, `{"error":"Admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ValidateAPIKey(key string) (valid bool, isAdmin bool) {
	clean := strings.TrimPrefix(key, "Bearer ")
	for _, k := range config.C.APIKeys {
		if k == clean {
			return true, clean == config.C.AdminKey
		}
	}
	return false, false
}
