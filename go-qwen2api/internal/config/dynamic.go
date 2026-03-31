package config

import (
	"encoding/json"
	"net/http"
	"sync"
)

var mu sync.RWMutex

func SetAutoRefresh(v bool, interval int) {
	mu.Lock()
	defer mu.Unlock()
	C.AutoRefresh = v
	if interval > 0 {
		C.AutoRefreshInterval = interval
	}
}

func AddAPIKey(key string) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, k := range C.APIKeys {
		if k == key {
			return false
		}
	}
	C.APIKeys = append(C.APIKeys, key)
	return true
}

func RemoveAPIKey(key string) bool {
	mu.Lock()
	defer mu.Unlock()
	if key == C.AdminKey {
		return false
	}
	for i, k := range C.APIKeys {
		if k == key {
			C.APIKeys = append(C.APIKeys[:i], C.APIKeys[i+1:]...)
			return true
		}
	}
	return false
}

func WriteJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func WriteSuccess(w http.ResponseWriter, msg string) {
	WriteJSON(w, map[string]interface{}{"status": true, "message": msg})
}
