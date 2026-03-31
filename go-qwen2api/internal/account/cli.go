package account

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/logger"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/proxy"
	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/tools"
)

type CliInfo struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	ExpiresIn     int    `json:"expires_in"`
	ExpiryDate    int64  `json:"expiry_date"`
	RequestNumber int    `json:"request_number"`
	InitializedAt int64  `json:"initialized_at"`
}

const cliClientID = "f0304373b74a44d2b584a3fb70ca9e56"

func InitCliForToken(token string) (*CliInfo, error) {
	codeVerifier := tools.GenerateCodeVerifier()
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	baseURL := proxy.ChatBaseURL()
	client := proxy.UTLSClient()

	step1Data := url.Values{}
	step1Data.Set("client_id", cliClientID)
	step1Data.Set("scope", "openid profile email model.completion")
	step1Data.Set("code_challenge", codeChallenge)
	step1Data.Set("code_challenge_method", "S256")

	req, err := http.NewRequest("POST", baseURL+"/api/v1/oauth2/device/code", strings.NewReader(step1Data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("step1 request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("step1 error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("step1 failed (%d): %s", resp.StatusCode, string(body))
	}

	var step1 struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&step1); err != nil {
		return nil, fmt.Errorf("step1 decode error: %w", err)
	}
	if step1.DeviceCode == "" {
		return nil, fmt.Errorf("empty device_code")
	}

	authReq := map[string]interface{}{
		"approved":  true,
		"user_code": step1.UserCode,
	}
	authBody, _ := json.Marshal(authReq)
	authReq2, err := http.NewRequest("POST", baseURL+"/api/v2/oauth2/authorize", strings.NewReader(string(authBody)))
	if err != nil {
		return nil, fmt.Errorf("authorize request error: %w", err)
	}
	authReq2.Header.Set("Content-Type", "application/json")
	authReq2.Header.Set("Authorization", "Bearer "+token)
	authReq2.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	authReq2.Header.Set("Origin", baseURL)
	authReq2.Header.Set("Referer", baseURL+"/")

	authResp, err := client.Do(authReq2)
	if err != nil {
		return nil, fmt.Errorf("authorize error: %w", err)
	}
	authResp.Body.Close()
	if authResp.StatusCode != 200 {
		return nil, fmt.Errorf("authorize failed (%d)", authResp.StatusCode)
	}

	interval := time.Duration(step1.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}

	maxAttempts := 30
	if step1.ExpiresIn > 0 && step1.ExpiresIn < maxAttempts {
		maxAttempts = step1.ExpiresIn
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		time.Sleep(interval)

		tokenData := url.Values{}
		tokenData.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		tokenData.Set("client_id", cliClientID)
		tokenData.Set("device_code", step1.DeviceCode)
		tokenData.Set("code_verifier", codeVerifier)

		tokenReq, err := http.NewRequest("POST", baseURL+"/api/v1/oauth2/token", strings.NewReader(tokenData.Encode()))
		if err != nil {
			continue
		}
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		tokenReq.Header.Set("Accept", "application/json")
		tokenReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
		tokenReq.Header.Set("Origin", baseURL)
		tokenReq.Header.Set("Referer", baseURL+"/")

		tokenResp, err := client.Do(tokenReq)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(tokenResp.Body)
		tokenResp.Body.Close()

		if strings.Contains(string(body), "aliyun_waf") || strings.Contains(string(body), "captcha") {
			snippet := string(body)
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			logger.Error("CLI", "WAF raw response (status=%d, len=%d): %s", tokenResp.StatusCode, len(body), snippet)
			return nil, fmt.Errorf("WAF captcha blocked")
		}

		if tokenResp.StatusCode != 200 {
			logger.Error("CLI", "token poll status=%d body=%s", tokenResp.StatusCode, string(body))
		}

		if tokenResp.StatusCode == 200 {
			var tr struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
			}
			if err := json.Unmarshal(body, &tr); err != nil {
				continue
			}
			if tr.AccessToken != "" {
				return &CliInfo{
					AccessToken:   tr.AccessToken,
					RefreshToken:  tr.RefreshToken,
					ExpiresIn:     tr.ExpiresIn,
					ExpiryDate:    time.Now().UnixMilli() + int64(tr.ExpiresIn)*1000,
					RequestNumber: 0,
					InitializedAt: time.Now().Unix(),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("device auth polling timeout")
}

func RefreshCliToken(refreshToken string) (*CliInfo, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("empty refresh token")
	}

	baseURL := proxy.ChatBaseURL()
	client := proxy.UTLSClient()

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", cliClientID)

	req, err := http.NewRequest("POST", baseURL+"/api/v1/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("refresh request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("refresh decode error: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("refresh returned empty access_token")
	}

	return &CliInfo{
		AccessToken:   tr.AccessToken,
		RefreshToken:  coalesce(tr.RefreshToken, refreshToken),
		ExpiresIn:     tr.ExpiresIn,
		ExpiryDate:    time.Now().UnixMilli() + int64(tr.ExpiresIn)*1000,
		InitializedAt: time.Now().Unix(),
	}, nil
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
