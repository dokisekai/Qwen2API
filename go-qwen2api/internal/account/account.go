package account

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/config"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/logger"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/proxy"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/tools"
)

type AccountInfo struct {
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Token    string   `json:"token"`
	Expires  float64  `json:"expires"`
	CliInfo  *CliInfo `json:"cli_info,omitempty"`
}

type FailureInfo struct {
	Email     string `json:"email"`
	Reason    string `json:"reason"`
	Timestamp int64  `json:"timestamp"`
	ErrorCode string `json:"error_code,omitempty"`
}

type Manager struct {
	accounts       []AccountInfo
	cliIndex       int
	mu             sync.RWMutex
	lastUsed       map[string]int64
	failures       map[string]int
	failureRecords []FailureInfo
	WafBlocked     bool
	WafBlockCount  int
}

var M *Manager

func Init() {
	M = &Manager{
		lastUsed:       make(map[string]int64),
		failures:       make(map[string]int),
		failureRecords: make([]FailureInfo, 0),
	}
	M.load()

	go M.initCliAccounts()
	go M.dailyResetCliRequests()
	go initStatsResetLoop()

	if config.C.AutoRefresh {
		go func() {
			for {
				time.Sleep(time.Duration(config.C.AutoRefreshInterval) * time.Second)
				M.autoRefresh()
			}
		}()
	}
}

func (m *Manager) load() {
	switch config.C.DataSaveMode {
	case "file":
		m.loadFromFile()
	case "none":
		m.loadFromEnv()
	}
	logger.Info("ACCOUNT", "loaded %d accounts", len(m.accounts))
}

func (m *Manager) loadFromFile() {
	data, err := os.ReadFile("data/data.json")
	if err != nil {
		return
	}
	var fd struct {
		Accounts []AccountInfo `json:"accounts"`
	}
	if err := json.Unmarshal(data, &fd); err != nil {
		return
	}
	m.accounts = fd.Accounts
}

func (m *Manager) loadFromEnv() {
	env := os.Getenv("ACCOUNTS")
	if env == "" {
		return
	}
	for _, item := range strings.Split(env, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		email, password := parts[0], parts[1]
		token, expires := loginAndGetToken(email, password)
		m.accounts = append(m.accounts, AccountInfo{
			Email: email, Password: password, Token: token, Expires: expires,
		})
	}
}

func (m *Manager) GetCliAccessToken() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.accounts) == 0 {
		return "", ""
	}
	for i := 0; i < len(m.accounts); i++ {
		idx := (m.cliIndex + i) % len(m.accounts)
		acc := m.accounts[idx]
		if acc.CliInfo == nil || acc.CliInfo.AccessToken == "" {
			continue
		}
		if acc.CliInfo.RequestNumber >= 1000 {
			continue
		}
		if !m.isAvailable(acc.Email) {
			continue
		}
		m.cliIndex = (idx + 1) % len(m.accounts)
		m.lastUsed[acc.Email] = time.Now().UnixMilli()
		return acc.CliInfo.AccessToken, acc.Email
	}
	return "", ""
}

func (m *Manager) isAvailable(email string) bool {
	f := m.failures[email]
	if f >= 3 {
		last, ok := m.lastUsed[email]
		if ok && time.Now().UnixMilli()-last < 5*60*1000 {
			return false
		}
		delete(m.failures, email)
	}
	return true
}

func (m *Manager) IncrementCliRequest(email string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.accounts {
		if a.Email == email && a.CliInfo != nil {
			m.accounts[i].CliInfo.RequestNumber++
			break
		}
	}
}

func (m *Manager) RecordFailure(email, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures[email]++
	m.failureRecords = append(m.failureRecords, FailureInfo{
		Email: email, Reason: reason, Timestamp: time.Now().Unix(),
	})
	logger.Error("ACCOUNT", "recorded failure for %s: %s", email, reason)
}

func (m *Manager) ResetFailures(email string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.failures, email)
}

func (m *Manager) AllAccounts() []AccountInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]AccountInfo, len(m.accounts))
	copy(result, m.accounts)
	return result
}

func (m *Manager) AddAccount(email, password string) bool {
	token, expires := loginAndGetToken(email, password)
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.accounts {
		if a.Email == email {
			return false
		}
	}
	m.accounts = append(m.accounts, AccountInfo{
		Email: email, Password: password, Token: token, Expires: expires,
	})
	m.saveToFileLocked()

	go func() {
		cli, err := InitCliForToken(token)
		if err != nil {
			logger.Error("CLI", "init cli for %s failed: %v", email, err)
			return
		}
		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].Email == email {
				m.accounts[i].CliInfo = cli
				m.saveToFileLocked()
				break
			}
		}
		m.mu.Unlock()
		logger.Info("CLI", "initialized cli for %s", email)
	}()

	return true
}

func (m *Manager) RemoveAccount(email string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.accounts {
		if a.Email == email {
			m.accounts = append(m.accounts[:i], m.accounts[i+1:]...)
			m.saveToFileLocked()
			return true
		}
	}
	return false
}

func (m *Manager) RefreshToken(email string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.accounts {
		if a.Email == email {
			token, expires := loginAndGetToken(a.Email, a.Password)
			if token == "" {
				return false
			}
			m.accounts[i].Token = token
			m.accounts[i].Expires = expires
			m.accounts[i].CliInfo = nil
			m.failures[email] = 0
			m.saveToFileLocked()
			go m.reInitCli(email, token)
			return true
		}
	}
	return false
}

func (m *Manager) RefreshAllTokens() (int, int) {
	m.mu.RLock()
	accounts := make([]AccountInfo, len(m.accounts))
	copy(accounts, m.accounts)
	m.mu.RUnlock()

	success, failed := 0, 0
	for _, a := range accounts {
		token, expires := loginAndGetToken(a.Email, a.Password)
		if token == "" {
			failed++
			continue
		}
		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].Email == a.Email {
				m.accounts[i].Token = token
				m.accounts[i].Expires = expires
				m.accounts[i].CliInfo = nil
				m.failures[a.Email] = 0
				break
			}
		}
		m.mu.Unlock()
		go m.reInitCli(a.Email, token)
		success++
		time.Sleep(1 * time.Second)
	}
	m.mu.Lock()
	m.saveToFileLocked()
	m.mu.Unlock()
	return success, failed
}

func (m *Manager) ForceRefreshAllWithCli() (int, int) {
	m.mu.RLock()
	accounts := make([]AccountInfo, len(m.accounts))
	copy(accounts, m.accounts)
	m.mu.RUnlock()

	success, failed := 0, 0
	for _, a := range accounts {
		token, expires := loginAndGetToken(a.Email, a.Password)
		if token == "" {
			failed++
			logger.Error("AUTH", "force refresh login failed for %s", a.Email)
			continue
		}

		cli, err := InitCliForToken(token)
		if err != nil {
			failed++
			logger.Error("CLI", "force refresh cli init failed for %s: %v", a.Email, err)
			continue
		}

		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].Email == a.Email {
				m.accounts[i].Token = token
				m.accounts[i].Expires = expires
				m.accounts[i].CliInfo = cli
				m.accounts[i].CliInfo.RequestNumber = 0
				m.failures[a.Email] = 0
				break
			}
		}
		m.mu.Unlock()
		success++
		logger.Info("AUTH", "force refresh success for %s", a.Email)
		time.Sleep(1 * time.Second)
	}
	m.mu.Lock()
	m.saveToFileLocked()
	m.mu.Unlock()
	return success, failed
}

func (m *Manager) reInitCli(email, token string) {
	cli, err := InitCliForToken(token)
	if err != nil {
		logger.Error("CLI", "re-init cli for %s failed: %v", email, err)
		return
	}
	m.mu.Lock()
	for i := range m.accounts {
		if m.accounts[i].Email == email {
			m.accounts[i].CliInfo = cli
			break
		}
	}
	m.mu.Unlock()
	logger.Info("CLI", "re-init cli success for %s", email)
}

func (m *Manager) ExportFailureAccounts() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	passwordMap := make(map[string]string)
	for _, a := range m.accounts {
		passwordMap[a.Email] = a.Password
	}
	seen := make(map[string]bool)
	var result []string
	for _, f := range m.failureRecords {
		if seen[f.Email] {
			continue
		}
		seen[f.Email] = true
		if pw, ok := passwordMap[f.Email]; ok {
			result = append(result, f.Email+":"+pw)
		} else {
			result = append(result, f.Email)
		}
	}
	return result
}

func (m *Manager) autoRefresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	for i, a := range m.accounts {
		if a.Expires == 0 || a.Expires-float64(now) < 24*3600 {
			token, expires := loginAndGetToken(a.Email, a.Password)
			if token != "" {
				m.accounts[i].Token = token
				m.accounts[i].Expires = expires
				logger.Info("TOKEN", "refreshed token for %s", a.Email)
			}
			time.Sleep(1 * time.Second)
		}
	}
	m.saveToFileLocked()
}

func (m *Manager) initCliAccounts() {
	m.mu.RLock()
	accounts := make([]AccountInfo, len(m.accounts))
	copy(accounts, m.accounts)
	m.mu.RUnlock()

	for _, acc := range accounts {
		if acc.Token == "" {
			continue
		}
		cli, err := InitCliForToken(acc.Token)
		if err != nil {
			if strings.Contains(err.Error(), "WAF") {
				m.mu.Lock()
				m.WafBlocked = true
				m.WafBlockCount++
				m.mu.Unlock()
				logger.Error("CLI", "init cli for %s failed: %v (WAF blocked, need manual verify)", acc.Email, err)
			} else {
				logger.Error("CLI", "init cli for %s failed: %v", acc.Email, err)
			}
			continue
		}
		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].Email == acc.Email {
				m.accounts[i].CliInfo = cli
				break
			}
		}
		m.mu.Unlock()
		logger.Info("CLI", "initialized cli for %s", acc.Email)
	}

	go m.refreshCliLoop()
}

func (m *Manager) RetryInitCli() int {
	m.mu.Lock()
	m.WafBlocked = false
	m.WafBlockCount = 0
	m.mu.Unlock()

	if m.checkWaf() {
		logger.Info("CLI", "WAF still active, launching browser auto-verify...")
		go func() {
			err := BypassWAF(config.C.QwenChatProxyURL)
			if err != nil {
				logger.Error("WAF", "browser bypass failed: %v", err)
			}
			time.Sleep(3 * time.Second)
			m.doRetryInitCli()
		}()
		return -1
	}

	return m.doRetryInitCli()
}

func (m *Manager) InitAllCli() int {
	m.mu.Lock()
	m.WafBlocked = false
	m.WafBlockCount = 0
	m.mu.Unlock()

	if m.checkWaf() {
		logger.Info("CLI", "WAF still active, launching browser auto-verify...")
		go func() {
			err := BypassWAF(config.C.QwenChatProxyURL)
			if err != nil {
				logger.Error("WAF", "browser bypass failed: %v", err)
			}
			time.Sleep(3 * time.Second)
			m.doInitAllCli(false)
		}()
		return -1
	}

	return m.doInitAllCli(false)
}

func (m *Manager) InitFailedCli() int {
	m.mu.Lock()
	m.WafBlocked = false
	m.WafBlockCount = 0
	m.mu.Unlock()

	if m.checkWaf() {
		logger.Info("CLI", "WAF still active, launching browser auto-verify...")
		go func() {
			err := BypassWAF(config.C.QwenChatProxyURL)
			if err != nil {
				logger.Error("WAF", "browser bypass failed: %v", err)
			}
			time.Sleep(3 * time.Second)
			m.doInitAllCli(true)
		}()
		return -1
	}

	return m.doInitAllCli(true)
}

func (m *Manager) checkWaf() bool {
	client := proxy.Client()
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("client_id", "f0304373b74a44d2b584a3fb70ca9e56")
	data.Set("device_code", "test_check")
	data.Set("code_verifier", "test_check")
	req, _ := http.NewRequest("POST", config.C.QwenChatProxyURL+"/api/v1/oauth2/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return true
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(body), "aliyun_waf")
}

func (m *Manager) doRetryInitCli() int {
	m.mu.RLock()
	accounts := make([]AccountInfo, len(m.accounts))
	copy(accounts, m.accounts)
	m.mu.RUnlock()

	success := 0
	for _, acc := range accounts {
		if acc.CliInfo != nil && acc.CliInfo.AccessToken != "" {
			success++
			continue
		}
		if acc.Token == "" {
			continue
		}
		cli, err := InitCliForToken(acc.Token)
		if err != nil {
			if strings.Contains(err.Error(), "WAF") {
				m.mu.Lock()
				m.WafBlocked = true
				m.WafBlockCount++
				m.mu.Unlock()
			}
			continue
		}
		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].Email == acc.Email {
				m.accounts[i].CliInfo = cli
				break
			}
		}
		m.mu.Unlock()
		success++
	}
	m.mu.Lock()
	m.saveToFileLocked()
	m.mu.Unlock()
	return success
}

func (m *Manager) doInitAllCli(failedOnly bool) int {
	m.mu.RLock()
	accounts := make([]AccountInfo, len(m.accounts))
	copy(accounts, m.accounts)
	m.mu.RUnlock()

	success := 0
	for _, acc := range accounts {
		if failedOnly && acc.CliInfo != nil && acc.CliInfo.AccessToken != "" {
			continue
		}
		if acc.Token == "" {
			continue
		}
		cli, err := InitCliForToken(acc.Token)
		if err != nil {
			if strings.Contains(err.Error(), "WAF") {
				m.mu.Lock()
				m.WafBlocked = true
				m.WafBlockCount++
				m.mu.Unlock()
			}
			continue
		}
		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].Email == acc.Email {
				m.accounts[i].CliInfo = cli
				m.accounts[i].CliInfo.RequestNumber = 0
				break
			}
		}
		m.mu.Unlock()
		success++
	}
	m.mu.Lock()
	m.saveToFileLocked()
	m.mu.Unlock()
	return success
}

func (m *Manager) refreshCliLoop() {
	for {
		time.Sleep(2 * time.Hour)
		m.mu.RLock()
		var toRefresh []AccountInfo
		for _, a := range m.accounts {
			if a.CliInfo != nil && a.CliInfo.RefreshToken != "" {
				toRefresh = append(toRefresh, a)
			}
		}
		m.mu.RUnlock()

		for _, acc := range toRefresh {
			cli, err := RefreshCliToken(acc.CliInfo.RefreshToken)
			if err != nil {
				logger.Error("CLI", "refresh cli token for %s failed: %v", acc.Email, err)
				continue
			}
			m.mu.Lock()
			for i := range m.accounts {
				if m.accounts[i].Email == acc.Email {
					m.accounts[i].CliInfo = cli
					break
				}
			}
			m.mu.Unlock()
			logger.Info("CLI", "refreshed cli token for %s", acc.Email)
		}
	}
}

func (m *Manager) dailyResetCliRequests() {
	for {
		now := time.Now()
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		time.Sleep(tomorrow.Sub(now))

		m.mu.Lock()
		for i := range m.accounts {
			if m.accounts[i].CliInfo != nil {
				m.accounts[i].CliInfo.RequestNumber = 0
			}
		}
		m.mu.Unlock()
		logger.Info("CLI", "reset all cli request numbers")
	}
}

func (m *Manager) GetWafStatus() (bool, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.WafBlocked, m.WafBlockCount
}

func (m *Manager) GetFailureRecords() []FailureInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]FailureInfo, len(m.failureRecords))
	copy(result, m.failureRecords)
	return result
}

func (m *Manager) ClearFailureRecords() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRecords = make([]FailureInfo, 0)
}

func (m *Manager) saveToFileLocked() error {
	if config.C.DataSaveMode != "file" {
		return nil
	}
	os.MkdirAll("data", 0755)
	fd := struct {
		Accounts []AccountInfo `json:"accounts"`
	}{Accounts: m.accounts}
	data, _ := json.MarshalIndent(fd, "", "  ")
	return os.WriteFile("data/data.json", data, 0644)
}

func loginAndGetToken(email, password string) (string, float64) {
	token := Login(email, password)
	if token == "" {
		return "", 0
	}
	decoded, err := tools.JwtDecode(token)
	if err != nil {
		return token, 0
	}
	return token, decoded.Payload.Exp
}

func Login(email, password string) string {
	baseURL := proxy.ChatBaseURL()
	payload := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, tools.SHA256Hash(password))
	body := strings.NewReader(payload)
	req, err := http.NewRequest("POST", baseURL+"/api/v1/auths/signin", body)
	if err != nil {
		logger.Error("AUTH", "login request error: %v", err)
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := proxy.Client()
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("AUTH", "login error for %s: %v", email, err)
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Token != "" {
		logger.Info("AUTH", "login success: %s", email)
	}
	return result.Token
}
