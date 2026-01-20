// Package challenge provides challenge detection and handling for the captcha service.
package challenge

import (
	"context"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// Type represents the type of challenge detected on a page.
type Type string

const (
	// TypeNone indicates no challenge was detected.
	TypeNone Type = "none"
	// TypeCloudflareJS indicates a Cloudflare JavaScript challenge (auto-resolves).
	TypeCloudflareJS Type = "cloudflare_js"
	// TypeCloudflareTurnstile indicates a Cloudflare Turnstile CAPTCHA.
	TypeCloudflareTurnstile Type = "cloudflare_turnstile"
	// TypeCloudflareInterstitial indicates a Cloudflare interstitial page.
	TypeCloudflareInterstitial Type = "cloudflare_interstitial"
	// TypeDDoSGuard indicates a DDoS-Guard challenge.
	TypeDDoSGuard Type = "ddosguard"
	// TypeHCaptcha indicates an hCaptcha challenge.
	TypeHCaptcha Type = "hcaptcha"
	// TypeReCaptchaV2 indicates a reCAPTCHA v2 challenge.
	TypeReCaptchaV2 Type = "recaptcha_v2"
	// TypeReCaptchaV3 indicates a reCAPTCHA v3 (invisible).
	TypeReCaptchaV3 Type = "recaptcha_v3"
	// TypeUnknown indicates an unknown challenge type.
	TypeUnknown Type = "unknown"
)

// Detection contains information about a detected challenge.
type Detection struct {
	Type     Type   `json:"type"`
	SiteKey  string `json:"siteKey,omitempty"`  // For CAPTCHA challenges
	Action   string `json:"action,omitempty"`   // For Turnstile
	CData    string `json:"cdata,omitempty"`    // For Turnstile
	PageURL  string `json:"pageUrl"`
	Title    string `json:"title"`
	CanAuto  bool   `json:"canAuto"`            // Can be auto-resolved by waiting
}

// Detector detects challenges on web pages.
type Detector struct {
	// Configuration options can be added here
}

// NewDetector creates a new challenge detector.
func NewDetector() *Detector {
	return &Detector{}
}

// Detect analyzes a page and returns the detected challenge type.
func (d *Detector) Detect(ctx context.Context, page *rod.Page) (*Detection, error) {
	// Get page info
	info, err := page.Info()
	if err != nil {
		return nil, err
	}

	pageURL := info.URL
	title := info.Title

	detection := &Detection{
		Type:    TypeNone,
		PageURL: pageURL,
		Title:   title,
		CanAuto: false,
	}

	// Check for Cloudflare challenges
	if d.isCloudflareChallenge(title) {
		detection.Type = TypeCloudflareJS
		detection.CanAuto = true
		return detection, nil
	}

	// Check for Cloudflare interstitial
	if d.isCloudflareInterstitial(page) {
		detection.Type = TypeCloudflareInterstitial
		detection.CanAuto = true
		return detection, nil
	}

	// Check for Turnstile
	if siteKey, action, cdata := d.detectTurnstile(page); siteKey != "" {
		detection.Type = TypeCloudflareTurnstile
		detection.SiteKey = siteKey
		detection.Action = action
		detection.CData = cdata
		detection.CanAuto = false
		return detection, nil
	}

	// Check for hCaptcha
	if siteKey := d.detectHCaptcha(page); siteKey != "" {
		detection.Type = TypeHCaptcha
		detection.SiteKey = siteKey
		detection.CanAuto = false
		return detection, nil
	}

	// Check for reCAPTCHA
	if siteKey, version := d.detectReCaptcha(page); siteKey != "" {
		if version == 3 {
			detection.Type = TypeReCaptchaV3
		} else {
			detection.Type = TypeReCaptchaV2
		}
		detection.SiteKey = siteKey
		detection.CanAuto = false
		return detection, nil
	}

	// Check for DDoS-Guard
	if d.isDDoSGuard(page, title) {
		detection.Type = TypeDDoSGuard
		detection.CanAuto = true
		return detection, nil
	}

	return detection, nil
}

// WaitForChallenge waits for an auto-resolvable challenge to complete.
func (d *Detector) WaitForChallenge(ctx context.Context, page *rod.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		detection, err := d.Detect(ctx, page)
		if err != nil {
			return err
		}

		// Challenge resolved
		if detection.Type == TypeNone {
			return nil
		}

		// Challenge requires manual solving
		if !detection.CanAuto {
			return nil // Return so caller can handle CAPTCHA
		}

		// Wait a bit before checking again
		time.Sleep(500 * time.Millisecond)
	}

	return context.DeadlineExceeded
}

// isCloudflareChallenge checks if the page title indicates a Cloudflare challenge.
func (d *Detector) isCloudflareChallenge(title string) bool {
	patterns := []string{
		"Just a moment",
		"Checking your browser",
		"Please wait",
		"Attention Required",
		"One more step",
		"Verify you are human",
	}

	titleLower := strings.ToLower(title)
	for _, pattern := range patterns {
		if strings.Contains(titleLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// isCloudflareInterstitial checks for Cloudflare interstitial pages.
func (d *Detector) isCloudflareInterstitial(page *rod.Page) bool {
	// Check for cf-browser-verification div
	has, _, _ := page.Has("#cf-browser-verification")
	if has {
		return true
	}

	// Check for challenge-running class
	has, _, _ = page.Has(".challenge-running")
	if has {
		return true
	}

	// Check for cf-challenge-running
	has, _, _ = page.Has("#cf-challenge-running")
	if has {
		return true
	}

	return false
}

// detectTurnstile checks for Cloudflare Turnstile and extracts parameters.
func (d *Detector) detectTurnstile(page *rod.Page) (siteKey, action, cdata string) {
	// Check for Turnstile iframe
	has, _, _ := page.Has(`iframe[src*="challenges.cloudflare.com"]`)
	if has {
		// Try to get sitekey from turnstile element
		el, err := page.Element(`[data-sitekey]`)
		if err == nil {
			if attr := el.MustAttribute("data-sitekey"); attr != nil {
				siteKey = *attr
			}
			if attr := el.MustAttribute("data-action"); attr != nil {
				action = *attr
			}
			if attr := el.MustAttribute("data-cdata"); attr != nil {
				cdata = *attr
			}
		}
		return
	}

	// Check for Turnstile widget
	has, _, _ = page.Has(`.cf-turnstile`)
	if has {
		el, err := page.Element(`.cf-turnstile`)
		if err == nil {
			if attr := el.MustAttribute("data-sitekey"); attr != nil {
				siteKey = *attr
			}
			if attr := el.MustAttribute("data-action"); attr != nil {
				action = *attr
			}
			if attr := el.MustAttribute("data-cdata"); attr != nil {
				cdata = *attr
			}
		}
		return
	}

	return "", "", ""
}

// detectHCaptcha checks for hCaptcha and extracts the site key.
func (d *Detector) detectHCaptcha(page *rod.Page) string {
	// Check for hCaptcha iframe
	has, _, _ := page.Has(`iframe[src*="hcaptcha.com"]`)
	if has {
		el, err := page.Element(`[data-sitekey]`)
		if err == nil {
			siteKey, _ := el.Attribute("data-sitekey")
			if siteKey != nil {
				return *siteKey
			}
		}
	}

	// Check for h-captcha class
	has, _, _ = page.Has(`.h-captcha`)
	if has {
		el, err := page.Element(`.h-captcha`)
		if err == nil {
			siteKey, _ := el.Attribute("data-sitekey")
			if siteKey != nil {
				return *siteKey
			}
		}
	}

	return ""
}

// detectReCaptcha checks for reCAPTCHA and extracts the site key.
func (d *Detector) detectReCaptcha(page *rod.Page) (siteKey string, version int) {
	// Check for reCAPTCHA v2
	has, _, _ := page.Has(`.g-recaptcha`)
	if has {
		el, err := page.Element(`.g-recaptcha`)
		if err == nil {
			sk, _ := el.Attribute("data-sitekey")
			if sk != nil {
				return *sk, 2
			}
		}
	}

	// Check for reCAPTCHA v3 (usually invisible)
	result, err := page.Eval(`() => {
		if (window.grecaptcha && window.grecaptcha.enterprise) {
			// Try to find site key in page scripts
			const scripts = document.querySelectorAll('script[src*="recaptcha"]');
			for (const script of scripts) {
				const match = script.src.match(/render=([^&]+)/);
				if (match) return { siteKey: match[1], version: 3 };
			}
		}
		return null;
	}`)
	if err == nil && result.Value.Str() != "" {
		// Parse the result
		// This is simplified - in production you'd properly parse the JSON
		return "", 3
	}

	return "", 0
}

// isDDoSGuard checks for DDoS-Guard protection.
func (d *Detector) isDDoSGuard(page *rod.Page, title string) bool {
	// Check title
	if strings.Contains(strings.ToLower(title), "ddos-guard") {
		return true
	}

	// Check for DDoS-Guard meta tag
	has, _, _ := page.Has(`meta[name="generator"][content*="DDoS-GUARD"]`)
	if has {
		return true
	}

	// Check for DDoS-Guard in page content
	result, err := page.Eval(`() => document.body.innerText.includes('DDoS-GUARD')`)
	if err == nil && result.Value.Bool() {
		return true
	}

	return false
}
