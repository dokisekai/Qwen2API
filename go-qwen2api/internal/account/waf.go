package account

import (
	"context"
	"math/rand"
	"time"

	"github.com/Rfym21/Qwen2API/go-qwen2api/internal/logger"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

type trackPoint struct {
	x, y float64
}

func BypassWAF(baseURL string) error {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(),
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-notifications", true),
		chromedp.Flag("window-size", "800,600"),
		chromedp.UserAgent(ua),
	)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	logger.Info("WAF", "opening browser: %s", baseURL)

	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return err
	}

	var captchaVisible bool
	for i := 0; i < 10; i++ {
		_ = chromedp.Run(ctx, chromedp.Evaluate(`!!document.querySelector('#aliyunCaptcha-sliding-slider')`, &captchaVisible))
		if captchaVisible {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if captchaVisible {
		logger.Info("WAF", "captcha detected, attempting auto slide...")

		for attempt := 0; attempt < 3; attempt++ {
			err = autoSlideCaptcha(ctx)
			if err != nil {
				logger.Warn("WAF", "auto slide attempt %d failed: %v", attempt+1, err)
				time.Sleep(2 * time.Second)
				continue
			}

			time.Sleep(2 * time.Second)

			var stillVisible bool
			_ = chromedp.Run(ctx, chromedp.Evaluate(`!!document.querySelector('.nc-container') && document.querySelector('.nc-container').offsetParent !== null`, &stillVisible))
			if !stillVisible {
				logger.Info("WAF", "auto slide succeeded on attempt %d", attempt+1)
				return nil
			}
			logger.Warn("WAF", "auto slide attempt %d: captcha still visible, retrying...", attempt+1)
			time.Sleep(2 * time.Second)
		}
		logger.Warn("WAF", "auto slide failed after 3 attempts, please manually complete in browser")
	} else {
		logger.Info("WAF", "no captcha detected, page loaded normally")
	}

	logger.Info("WAF", "waiting for manual verification (max 90s)...")
	for i := 0; i < 18; i++ {
		time.Sleep(5 * time.Second)
		var visible bool
		_ = chromedp.Run(ctx, chromedp.Evaluate(`!!document.querySelector('.nc-container') && document.querySelector('.nc-container').offsetParent !== null`, &visible))
		if !visible {
			logger.Info("WAF", "verification completed")
			return nil
		}
	}

	logger.Warn("WAF", "browser verification timeout, closing browser")
	return nil
}

func autoSlideCaptcha(ctx context.Context) error {
	type sliderInfo struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
	}
	var info sliderInfo
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const el = document.querySelector('#aliyunCaptcha-sliding-slider');
			if (!el) return {x:0,y:0,w:0};
			const rect = el.getBoundingClientRect();
			return {x: rect.x + rect.width/2, y: rect.y + rect.height/2, w: rect.width};
		})()
	`, &info))
	if err != nil || info.X == 0 {
		return err
	}

	var containerWidth float64
	_ = chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const el = document.querySelector('#captcha-element');
			if (!el) return 0;
			const rect = el.getBoundingClientRect();
			return rect.width;
		})()
	`, &containerWidth))

	slideDistance := containerWidth - info.W
	if slideDistance <= 0 {
		slideDistance = 260
	}

	points := generateHumanTrack(info.X, info.X+slideDistance, info.Y)

	err = chromedp.Run(ctx, input.DispatchMouseEvent(input.MousePressed, info.X, info.Y))
	if err != nil {
		return err
	}

	for i := 1; i < len(points); i++ {
		delay := time.Duration(3+rand.Intn(8)) * time.Millisecond
		time.Sleep(delay)
		err = chromedp.Run(ctx, input.DispatchMouseEvent(input.MouseMoved, points[i].x, points[i].y))
		if err != nil {
			return err
		}
	}

	time.Sleep(50 * time.Millisecond)
	err = chromedp.Run(ctx, input.DispatchMouseEvent(input.MouseReleased, points[len(points)-1].x, points[len(points)-1].y))

	return err
}

func generateHumanTrack(startX, endX, startY float64) []trackPoint {
	distance := endX - startX
	totalSteps := int(distance/8) + rand.Intn(10) + 20

	points := make([]trackPoint, 0, totalSteps+1)

	for i := 0; i <= totalSteps; i++ {
		t := float64(i) / float64(totalSteps)
		ease := easeInOutCubic(t)
		x := startX + distance*ease
		y := startY + (rand.Float64()-0.5)*3

		if t < 0.7 {
			y += (rand.Float64() - 0.5) * 1.5
		} else {
			y += (rand.Float64() - 0.5) * 0.5
		}

		points = append(points, trackPoint{x: x, y: y})
	}

	if len(points) > 3 {
		last := len(points) - 1
		points[last].x = endX
		points[last].y = startY + (rand.Float64()-0.5)*1
	}

	return points
}

func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - 9*t + 6*t*t - t*t*t
}
