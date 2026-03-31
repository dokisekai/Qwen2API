package route

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/account"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/config"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/logger"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/middleware"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/proxy"
)

type AccountResp struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Token    string  `json:"token"`
	Expires  float64 `json:"expires"`
	CliReady bool    `json:"cli_ready"`
	TokenOk  bool    `json:"token_ok"`
}

func handleCliMiddleware(w http.ResponseWriter, r *http.Request) {
	accessToken, email := account.M.GetCliAccessToken()
	if accessToken == "" {
		http.Error(w, `{"error":"no available CLI account"}`, http.StatusServiceUnavailable)
		return
	}
	account.M.IncrementCliRequest(email)
	r.Header.Set("X-Cli-Access-Token", accessToken)
	r.Header.Set("X-Cli-Email", email)
	CliChatCompletion(w, r)
}

func CliChatCompletion(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	preprocessRequestBody(body)

	isStream := false
	if s, ok := body["stream"].(bool); ok {
		isStream = s
	}
	model := ""
	if m, ok := body["model"].(string); ok {
		model = m
	}

	accessToken := r.Header.Get("X-Cli-Access-Token")
	email := r.Header.Get("X-Cli-Email")
	if accessToken == "" {
		http.Error(w, `{"error":"no cli access token"}`, http.StatusServiceUnavailable)
		return
	}

	logger.Info("CLI", "request using account[%s] model=%s", email, model)

	baseURL := proxy.CliBaseURL()
	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", baseURL+"/v1/chat/completions", strings.NewReader(string(data)))
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	if isStream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", "QwenCode/0.10.3 (darwin; arm64)")
	req.Header.Set("X-Dashscope-Useragent", "QwenCode/0.10.3 (darwin; arm64)")
	req.Header.Set("X-Stainless-Runtime-Version", "v22.17.0")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("X-Stainless-Lang", "js")
	req.Header.Set("X-Stainless-Arch", "arm64")
	req.Header.Set("X-Stainless-Package-Version", "5.11.0")
	req.Header.Set("X-Dashscope-Cachecontrol", "enable")
	req.Header.Set("X-Stainless-Retry-Count", "0")
	req.Header.Set("X-Stainless-Os", "MacOS")
	req.Header.Set("X-Dashscope-Authtype", "qwen-oauth")
	req.Header.Set("X-Stainless-Runtime", "node")

	client := proxy.Client()
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("CLI", "request failed for [%s]: %v", email, err)
		account.M.RecordFailure(email, "connection error")
		account.RecordCall(false)
		http.Error(w, `{"error":{"message":"connection_error","type":"connection_error","code":503}}`, http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		logger.Error("CLI", "request failed for [%s] status=%d", email, resp.StatusCode)
		account.M.RecordFailure(email, "api_error")
		account.RecordCall(false)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "api_error",
				"type":    "api_error",
				"code":    resp.StatusCode,
				"details": errBody,
			},
		})
		return
	}

	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				text := string(buf[:n])
				lines := strings.Split(text, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" || !strings.HasPrefix(line, "data:") {
						continue
					}
					w.Write([]byte(line + "\n\n"))
					if canFlush {
						flusher.Flush()
					}
				}
			}
			if readErr != nil {
				break
			}
		}
		logger.Info("CLI", "request success for [%s] (stream)", email)
		account.M.ResetFailures(email)
		account.RecordCall(true)
	} else {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result == nil {
			result = make(map[string]interface{})
		}
		if _, ok := result["object"]; !ok {
			result["object"] = "chat.completion"
		}
		if _, ok := result["model"]; !ok {
			result["model"] = model
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		logger.Info("CLI", "request success for [%s] (json)", email)
		account.M.ResetFailures(email)
		account.RecordCall(true)
	}
}

func preprocessRequestBody(body map[string]interface{}) {
	var modelRedirect = map[string]string{
		"qwen3.5-plus": "coder-model",
	}
	if model, ok := body["model"].(string); ok {
		if redirect, found := modelRedirect[model]; found {
			body["model"] = redirect
		}
	}

	if stream, ok := body["stream"].(bool); ok && stream {
		tools, _ := body["tools"].([]interface{})
		if len(tools) == 0 {
			body["tools"] = []interface{}{
				map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "do_not_call_me",
						"description": "Do not call this tool.",
						"parameters": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{"operation": map[string]interface{}{"type": "number", "description": "placeholder"}},
							"required":   []string{"operation"},
						},
					},
				},
			}
		}
		so, _ := body["stream_options"].(map[string]interface{})
		if so == nil {
			so = make(map[string]interface{})
		}
		so["include_usage"] = true
		body["stream_options"] = so
	}
}

func handleGetAllAccounts(w http.ResponseWriter, r *http.Request) {
	var resp []AccountResp
	for _, a := range account.M.AllAccounts() {
		tokenOk := a.Token != "" && a.Expires > float64(time.Now().Unix())
		resp = append(resp, AccountResp{
			Email:    a.Email,
			Password: a.Password,
			Token:    a.Token,
			Expires:  a.Expires,
			CliReady: a.CliInfo != nil && a.CliInfo.AccessToken != "",
			TokenOk:  tokenOk,
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total": len(resp),
		"data":  resp,
	})
}

func handleSetAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}
	if account.M.AddAccount(body.Email, body.Password) {
		json.NewEncoder(w).Encode(map[string]interface{}{"email": body.Email, "message": "account created"})
	} else {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
	}
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}
	if account.M.RemoveAccount(body.Email) {
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "account deleted"})
	} else {
		http.Error(w, `{"error":"account not found"}`, http.StatusNotFound)
	}
}

func handleSetAccounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Accounts string `json:"accounts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	body.Accounts = strings.ReplaceAll(body.Accounts, "\r", "\n")
	lines := strings.Split(body.Accounts, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		account.M.AddAccount(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "batch add submitted"})
}

func handleRefreshAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}
	if account.M.RefreshToken(body.Email) {
		config.WriteSuccess(w, "account refreshed")
	} else {
		config.WriteError(w, http.StatusInternalServerError, "refresh failed")
	}
}

func handleRefreshAllAccounts(w http.ResponseWriter, r *http.Request) {
	go func() {
		success, failed := account.M.RefreshAllTokens()
		logger.Info("API", "refreshAllAccounts completed: success=%d failed=%d", success, failed)
	}()
	config.WriteJSON(w, map[string]interface{}{"message": "batch refresh started", "note": "running in background"})
}

func handleForceRefreshAll(w http.ResponseWriter, r *http.Request) {
	go func() {
		success, failed := account.M.ForceRefreshAllWithCli()
		logger.Info("API", "forceRefreshAll completed: success=%d failed=%d", success, failed)
	}()
	config.WriteJSON(w, map[string]interface{}{"message": "force refresh started", "note": "running in background"})
}

func handleExportFailureAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := account.M.ExportFailureAccounts()
	content := strings.Join(accounts, "\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"failure_accounts.txt\"")
	w.Write([]byte(content))
}

func handleGetFailureRecords(w http.ResponseWriter, r *http.Request) {
	records := account.M.GetFailureRecords()
	config.WriteJSON(w, map[string]interface{}{
		"total": len(records),
		"data":  records,
	})
}

func handleClearFailureRecords(w http.ResponseWriter, r *http.Request) {
	account.M.ClearFailureRecords()
	config.WriteSuccess(w, "failure records cleared")
}

func handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		APIKey string `json:"apiKey"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	valid, isAdmin := middleware.ValidateAPIKey(body.APIKey)
	if !valid {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 401, "message": "Unauthorized"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": 200, "message": "success", "isAdmin": isAdmin})
}

func handleAddRegularKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" {
		config.WriteError(w, http.StatusBadRequest, "API Key cannot be empty")
		return
	}
	if config.AddAPIKey(body.APIKey) {
		config.WriteJSON(w, map[string]string{"message": "API Key added"})
	} else {
		config.WriteError(w, http.StatusConflict, "API Key already exists")
	}
}

func handleDeleteRegularKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" {
		config.WriteError(w, http.StatusBadRequest, "API Key cannot be empty")
		return
	}
	if config.RemoveAPIKey(body.APIKey) {
		config.WriteJSON(w, map[string]string{"message": "API Key deleted"})
	} else {
		config.WriteError(w, http.StatusNotFound, "API Key not found")
	}
}

func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	var regularKeys []string
	for _, k := range config.C.APIKeys {
		if k != config.C.AdminKey {
			regularKeys = append(regularKeys, k)
		}
	}
	config.WriteJSON(w, map[string]interface{}{
		"adminKey":            config.C.AdminKey,
		"regularKeys":         regularKeys,
		"autoRefresh":         config.C.AutoRefresh,
		"autoRefreshInterval": config.C.AutoRefreshInterval,
	})
}

func handleRetryCliInit(w http.ResponseWriter, r *http.Request) {
	go func() {
		success := account.M.RetryInitCli()
		logger.Info("API", "retryCliInit completed: %d accounts initialized", success)
	}()
	config.WriteJSON(w, map[string]interface{}{
		"message": "CLI re-initialization started",
		"note":    "running in background",
	})
}

func handleInitAllCli(w http.ResponseWriter, r *http.Request) {
	go func() {
		success := account.M.InitAllCli()
		logger.Info("API", "initAllCli completed: %d accounts initialized", success)
	}()
	config.WriteJSON(w, map[string]interface{}{
		"message": "CLI initialization started for all accounts",
		"note":    "running in background",
	})
}

func handleInitFailedCli(w http.ResponseWriter, r *http.Request) {
	go func() {
		success := account.M.InitFailedCli()
		logger.Info("API", "initFailedCli completed: %d accounts initialized", success)
	}()
	config.WriteJSON(w, map[string]interface{}{
		"message": "CLI initialization started for failed accounts",
		"note":    "running in background",
	})
}

func handleWafStatus(w http.ResponseWriter, r *http.Request) {
	wafBlocked, wafCount := account.M.GetWafStatus()
	config.WriteJSON(w, map[string]interface{}{
		"waf_blocked":     wafBlocked,
		"waf_block_count": wafCount,
		"verify_url":      config.C.QwenChatProxyURL,
	})
}

func handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := account.GetStatsSummary()
	config.WriteJSON(w, stats)
}
