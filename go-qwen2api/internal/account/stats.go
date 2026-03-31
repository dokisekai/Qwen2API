package account

import (
	"sync"
	"time"
)

type HourlyStats struct {
	Hour    string `json:"hour"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

type StatsCollector struct {
	mu           sync.RWMutex
	hourly       []HourlyStats
	totalCalls   int64 `json:"total_calls"`
	totalSuccess int64 `json:"total_success"`
	totalFailed  int64 `json:"total_failed"`
	dailyLimit   int   `json:"daily_limit"`
	dailyUsed    int   `json:"daily_used"`
}

var Stats = &StatsCollector{dailyLimit: 1000}

func RecordCall(success bool) {
	Stats.mu.Lock()
	defer Stats.mu.Unlock()

	Stats.totalCalls++
	if success {
		Stats.totalSuccess++
	} else {
		Stats.totalFailed++
	}

	if Stats.dailyUsed < Stats.dailyLimit {
		Stats.dailyUsed++
	}

	now := time.Now()
	hourKey := now.Format("2006-01-02 15:04")

	if len(Stats.hourly) == 0 || Stats.hourly[len(Stats.hourly)-1].Hour != hourKey {
		Stats.hourly = append(Stats.hourly, HourlyStats{Hour: hourKey})
	}

	h := &Stats.hourly[len(Stats.hourly)-1]
	if success {
		h.Success++
	} else {
		h.Failed++
	}
}

func GetStatsSummary() map[string]interface{} {
	Stats.mu.RLock()
	defer Stats.mu.RUnlock()

	var hourlyData []map[string]interface{}
	start := 0
	if len(Stats.hourly) > 24 {
		start = len(Stats.hourly) - 24
	}
	for i := start; i < len(Stats.hourly); i++ {
		h := Stats.hourly[i]
		hourlyData = append(hourlyData, map[string]interface{}{
			"hour":    h.Hour,
			"success": h.Success,
			"failed":  h.Failed,
		})
	}

	limitReachedCount := 0
	if M != nil {
		M.mu.RLock()
		for _, acc := range M.accounts {
			if acc.CliInfo != nil && acc.CliInfo.RequestNumber >= 1000 {
				limitReachedCount++
			}
		}
		M.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_calls":         Stats.totalCalls,
		"total_success":       Stats.totalSuccess,
		"total_failed":        Stats.totalFailed,
		"daily_limit":         Stats.dailyLimit,
		"daily_used":          Stats.dailyUsed,
		"hourly":              hourlyData,
		"limit_reached_count": limitReachedCount,
		"per_account_limit":   1000,
	}
}

func ResetDailyUsage() {
	Stats.mu.Lock()
	defer Stats.mu.Unlock()
	Stats.dailyUsed = 0
}

func initStatsResetLoop() {
	for {
		now := time.Now()
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		time.Sleep(tomorrow.Sub(now))
		ResetDailyUsage()
	}
}
