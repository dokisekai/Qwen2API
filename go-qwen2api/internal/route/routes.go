package route

import (
	"net/http"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/middleware"
)

func Setup() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/cli/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.ApiKeyVerify(http.HandlerFunc(handleCliMiddleware)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/getAllAccounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleGetAllAccounts)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/setAccount", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleSetAccount)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/deleteAccount", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleDeleteAccount)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/setAccounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleSetAccounts)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/refreshAccount", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleRefreshAccount)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/refreshAllAccounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleRefreshAllAccounts)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/forceRefreshAllAccounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleForceRefreshAll)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/getFailureRecords", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleGetFailureRecords)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/clearFailureRecords", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleClearFailureRecords)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/exportFailureAccounts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleExportFailureAccounts)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleGetSettings)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/addRegularKey", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleAddRegularKey)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/deleteRegularKey", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleDeleteRegularKey)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/retryCliInit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleRetryCliInit)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/initAllCli", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleInitAllCli)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/initFailedCli", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleInitFailedCli)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/wafStatus", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.AdminKeyVerify(http.HandlerFunc(handleWafStatus)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		middleware.ApiKeyVerify(http.HandlerFunc(handleGetStats)).ServeHTTP(w, r)
	})

	mux.HandleFunc("/verify", handleVerify)

	fs := http.FileServer(http.Dir("web"))
	mux.Handle("/", fs)

	return mux
}
