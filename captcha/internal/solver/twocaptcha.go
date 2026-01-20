package solver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/captcha/internal/challenge"
)

const (
	twoCaptchaBaseURL = "https://2captcha.com"
	// Pricing per 1000 solves (as of 2025)
	twoCaptchaTurnstilePrice = 0.00145 // $1.45/1000
	twoCaptchaHCaptchaPrice  = 0.00299 // $2.99/1000
	twoCaptchaReCaptchaPrice = 0.00299 // $2.99/1000
)

// TwoCaptcha implements the Solver interface using 2Captcha's API.
type TwoCaptcha struct {
	apiKey     string
	client     *http.Client
	pollDelay  time.Duration
	maxRetries int
}

// NewTwoCaptcha creates a new 2Captcha solver.
func NewTwoCaptcha(apiKey string) *TwoCaptcha {
	return &TwoCaptcha{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		pollDelay:  5 * time.Second,
		maxRetries: 60, // 5 minutes max (60 * 5s)
	}
}

// Name returns "2captcha".
func (t *TwoCaptcha) Name() string {
	return "2captcha"
}

// CanSolve returns true for supported challenge types.
func (t *TwoCaptcha) CanSolve(challengeType challenge.Type) bool {
	switch challengeType {
	case challenge.TypeCloudflareTurnstile,
		challenge.TypeHCaptcha,
		challenge.TypeReCaptchaV2,
		challenge.TypeReCaptchaV3:
		return true
	default:
		return false
	}
}

// Solve submits a CAPTCHA to 2Captcha and waits for the solution.
func (t *TwoCaptcha) Solve(ctx context.Context, params SolveParams) (*SolveResult, error) {
	// Submit the task
	taskID, err := t.submitTask(ctx, params)
	if err != nil {
		return nil, err
	}

	// Poll for result
	token, err := t.pollResult(ctx, taskID)
	if err != nil {
		return nil, err
	}

	return &SolveResult{
		Token:      token,
		Valid:      2 * time.Minute, // Tokens typically valid for ~2 minutes
		Cost:       t.Cost(params.Type),
		SolverName: t.Name(),
	}, nil
}

// Cost returns the cost per solve for the given challenge type.
func (t *TwoCaptcha) Cost(challengeType challenge.Type) float64 {
	switch challengeType {
	case challenge.TypeCloudflareTurnstile:
		return twoCaptchaTurnstilePrice
	case challenge.TypeHCaptcha:
		return twoCaptchaHCaptchaPrice
	case challenge.TypeReCaptchaV2, challenge.TypeReCaptchaV3:
		return twoCaptchaReCaptchaPrice
	default:
		return 0
	}
}

// Balance returns the current account balance.
func (t *TwoCaptcha) Balance(ctx context.Context) (float64, error) {
	resp, err := t.request(ctx, "/res.php", url.Values{
		"key":    {t.apiKey},
		"action": {"getbalance"},
		"json":   {"1"},
	})
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, err
	}

	var result struct {
		Status  int     `json:"status"`
		Request string  `json:"request"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// Try parsing as plain number
		balance, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
		if err != nil {
			return -1, fmt.Errorf("failed to parse balance: %s", string(body))
		}
		return balance, nil
	}

	if result.Status != 1 {
		return -1, fmt.Errorf("failed to get balance: %s", result.Request)
	}

	balance, _ := strconv.ParseFloat(result.Request, 64)
	return balance, nil
}

// submitTask submits a CAPTCHA task to 2Captcha.
func (t *TwoCaptcha) submitTask(ctx context.Context, params SolveParams) (string, error) {
	values := url.Values{
		"key":      {t.apiKey},
		"json":     {"1"},
		"pageurl":  {params.PageURL},
		"sitekey":  {params.SiteKey},
	}

	// Add method-specific parameters
	switch params.Type {
	case challenge.TypeCloudflareTurnstile:
		values.Set("method", "turnstile")
		if params.Action != "" {
			values.Set("action", params.Action)
		}
		if params.CData != "" {
			values.Set("data", params.CData)
		}
	case challenge.TypeHCaptcha:
		values.Set("method", "hcaptcha")
	case challenge.TypeReCaptchaV2:
		values.Set("method", "userrecaptcha")
	case challenge.TypeReCaptchaV3:
		values.Set("method", "userrecaptcha")
		values.Set("version", "v3")
		if params.Action != "" {
			values.Set("action", params.Action)
		}
	default:
		return "", fmt.Errorf("unsupported challenge type: %s", params.Type)
	}

	// Add proxy if provided
	if params.Proxy != nil {
		proxyStr := fmt.Sprintf("%s:%d", params.Proxy.Host, params.Proxy.Port)
		if params.Proxy.Username != "" {
			proxyStr = fmt.Sprintf("%s:%s@%s", params.Proxy.Username, params.Proxy.Password, proxyStr)
		}
		values.Set("proxy", proxyStr)
		values.Set("proxytype", strings.ToUpper(params.Proxy.Type))
	}

	resp, err := t.request(ctx, "/in.php", values)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %s", string(body))
	}

	if result.Status != 1 {
		return "", &SolverError{Message: fmt.Sprintf("2captcha error: %s", result.Request)}
	}

	return result.Request, nil
}

// pollResult polls for the CAPTCHA solution.
func (t *TwoCaptcha) pollResult(ctx context.Context, taskID string) (string, error) {
	values := url.Values{
		"key":    {t.apiKey},
		"action": {"get"},
		"id":     {taskID},
		"json":   {"1"},
	}

	for i := 0; i < t.maxRetries; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(t.pollDelay):
		}

		resp, err := t.request(ctx, "/res.php", values)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var result struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		if result.Status == 1 {
			return result.Request, nil
		}

		// Check for terminal errors
		switch result.Request {
		case "CAPCHA_NOT_READY":
			continue
		case "ERROR_CAPTCHA_UNSOLVABLE":
			return "", &SolverError{Message: "CAPTCHA is unsolvable"}
		case "ERROR_WRONG_CAPTCHA_ID":
			return "", &SolverError{Message: "wrong CAPTCHA ID"}
		default:
			if strings.HasPrefix(result.Request, "ERROR_") {
				return "", &SolverError{Message: result.Request}
			}
		}
	}

	return "", ErrSolverTimeout
}

// request makes an HTTP request to the 2Captcha API.
func (t *TwoCaptcha) request(ctx context.Context, path string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", twoCaptchaBaseURL+path+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}

	return t.client.Do(req)
}
