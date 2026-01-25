// Package handlers provides HTTP handlers for the captcha service API.
package handlers

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/jmylchreest/refyne-api/captcha/internal/browser"
	"github.com/jmylchreest/refyne-api/captcha/internal/challenge"
	"github.com/jmylchreest/refyne-api/captcha/internal/config"
	"github.com/jmylchreest/refyne-api/captcha/internal/http/mw"
	"github.com/jmylchreest/refyne-api/captcha/internal/models"
	"github.com/jmylchreest/refyne-api/captcha/internal/session"
	"github.com/jmylchreest/refyne-api/captcha/internal/solver"
	"github.com/jmylchreest/refyne-api/captcha/internal/version"
)

// SolveHandler handles solve requests.
type SolveHandler struct {
	pool           *browser.Pool
	sessions       *session.Manager
	detector       *challenge.Detector
	solver         solver.Solver
	cfg            *config.Config
	logger         *slog.Logger
}

// NewSolveHandler creates a new solve handler.
func NewSolveHandler(
	pool *browser.Pool,
	sessions *session.Manager,
	detector *challenge.Detector,
	solverChain solver.Solver,
	cfg *config.Config,
	logger *slog.Logger,
) *SolveHandler {
	return &SolveHandler{
		pool:     pool,
		sessions: sessions,
		detector: detector,
		solver:   solverChain,
		cfg:      cfg,
		logger:   logger,
	}
}

// Handle processes a solve request.
func (h *SolveHandler) Handle(ctx context.Context, req *models.SolveRequest) *models.SolveResponse {
	startTime := time.Now().UnixMilli()
	ver := version.Get().Version

	// Extract user claims for logging
	claims := mw.GetUserClaims(ctx)
	userID := ""
	jobID := ""
	tier := ""
	if claims != nil {
		userID = claims.UserID
		jobID = claims.JobID
		tier = claims.Tier
	}

	// Log incoming request with user context
	h.logger.Info("solve request received",
		"user_id", userID,
		"job_id", jobID,
		"tier", tier,
		"cmd", req.Cmd,
		"url", req.URL,
		"session", req.Session,
		"has_cookies", len(req.Cookies) > 0,
		"max_timeout", req.MaxTimeout,
	)

	// Route to appropriate handler based on command
	switch req.Cmd {
	case models.CmdSessionsCreate:
		return h.handleSessionCreate(ctx, req, startTime, ver, userID, jobID)
	case models.CmdSessionsList:
		return h.handleSessionsList(ctx, startTime, ver, userID, jobID)
	case models.CmdSessionsDestroy:
		return h.handleSessionDestroy(ctx, req, startTime, ver, userID, jobID)
	case models.CmdRequestGet:
		return h.handleRequestGet(ctx, req, startTime, ver, userID, jobID)
	case models.CmdRequestPost:
		return h.handleRequestPost(ctx, req, startTime, ver, userID, jobID)
	default:
		return models.NewErrorResponse("unknown command: "+req.Cmd, startTime, time.Now().UnixMilli(), ver, "")
	}
}

// handleSessionCreate creates a new browser session.
func (h *SolveHandler) handleSessionCreate(ctx context.Context, req *models.SolveRequest, startTime int64, ver, userID, jobID string) *models.SolveResponse {
	h.logger.Debug("creating session",
		"user_id", userID,
		"job_id", jobID,
		"requested_id", req.Session,
	)

	// Use client-provided session name or let manager generate one
	sess, err := h.sessions.Create(ctx, req.Session, req.SessionOptions)
	if err != nil {
		h.logger.Warn("session creation failed",
			"user_id", userID,
			"job_id", jobID,
			"requested_id", req.Session,
			"error", err,
		)
		return models.NewErrorResponse("failed to create session: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	h.logger.Info("session created",
		"user_id", userID,
		"job_id", jobID,
		"id", sess.ID,
		"requested_id", req.Session,
	)

	return &models.SolveResponse{
		Status:         "ok",
		Message:        "Session created successfully.",
		Session:        sess.ID,
		StartTimestamp: startTime,
		EndTimestamp:   time.Now().UnixMilli(),
		Version:        ver,
	}
}

// handleSessionsList lists all sessions.
func (h *SolveHandler) handleSessionsList(ctx context.Context, startTime int64, ver, userID, jobID string) *models.SolveResponse {
	sessions := h.sessions.List()

	h.logger.Debug("sessions listed",
		"user_id", userID,
		"job_id", jobID,
		"count", len(sessions),
	)

	return &models.SolveResponse{
		Status:         "ok",
		Message:        "Sessions retrieved successfully.",
		Sessions:       sessions,
		StartTimestamp: startTime,
		EndTimestamp:   time.Now().UnixMilli(),
		Version:        ver,
	}
}

// handleSessionDestroy destroys a session.
func (h *SolveHandler) handleSessionDestroy(ctx context.Context, req *models.SolveRequest, startTime int64, ver, userID, jobID string) *models.SolveResponse {
	if req.Session == "" {
		return models.NewErrorResponse("session ID required", startTime, time.Now().UnixMilli(), ver, "")
	}

	h.logger.Debug("destroying session",
		"user_id", userID,
		"job_id", jobID,
		"id", req.Session,
	)

	if err := h.sessions.Destroy(req.Session); err != nil {
		h.logger.Warn("session destroy failed",
			"user_id", userID,
			"job_id", jobID,
			"id", req.Session,
			"error", err,
		)
		return models.NewErrorResponse("failed to destroy session: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	h.logger.Info("session destroyed",
		"user_id", userID,
		"job_id", jobID,
		"id", req.Session,
	)

	return &models.SolveResponse{
		Status:         "ok",
		Message:        "Session destroyed successfully.",
		StartTimestamp: startTime,
		EndTimestamp:   time.Now().UnixMilli(),
		Version:        ver,
	}
}

// handleRequestGet handles a GET request.
func (h *SolveHandler) handleRequestGet(ctx context.Context, req *models.SolveRequest, startTime int64, ver, userID, jobID string) *models.SolveResponse {
	return h.handleRequest(ctx, req, "GET", startTime, ver, userID, jobID)
}

// handleRequestPost handles a POST request.
func (h *SolveHandler) handleRequestPost(ctx context.Context, req *models.SolveRequest, startTime int64, ver, userID, jobID string) *models.SolveResponse {
	return h.handleRequest(ctx, req, "POST", startTime, ver, userID, jobID)
}

// handleRequest handles a request (GET or POST).
func (h *SolveHandler) handleRequest(ctx context.Context, req *models.SolveRequest, method string, startTime int64, ver, userID, jobID string) *models.SolveResponse {
	if req.URL == "" {
		return models.NewErrorResponse("URL required", startTime, time.Now().UnixMilli(), ver, "")
	}

	// Set timeout from request or use default
	timeout := h.cfg.ChallengeTimeout
	if req.MaxTimeout > 0 {
		timeout = time.Duration(req.MaxTimeout) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var page *rod.Page
	var managedBrowser *browser.ManagedBrowser
	var sess *session.Session
	var cleanup func()

	// Use session or acquire browser from pool
	if req.Session != "" {
		h.logger.Debug("acquiring session (will wait if busy)",
			"user_id", userID,
			"job_id", jobID,
			"id", req.Session,
			"url", req.URL,
		)
		var err error
		sess, err = h.sessions.Acquire(ctx, req.Session)
		if err != nil {
			h.logger.Warn("session acquire failed",
				"user_id", userID,
				"job_id", jobID,
				"id", req.Session,
				"error", err,
			)
			return models.NewErrorResponse("failed to acquire session: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
		}
		h.logger.Debug("session acquired",
			"user_id", userID,
			"job_id", jobID,
			"id", req.Session,
			"request_count", sess.RequestCount,
			"has_cookies", len(sess.Cookies) > 0,
			"cookie_count", len(sess.Cookies),
		)
		page = sess.Page
		cleanup = func() {
			h.logger.Debug("releasing session",
				"user_id", userID,
				"job_id", jobID,
				"id", req.Session,
			)
			h.sessions.Release(req.Session)
		}
	} else {
		var err error
		managedBrowser, err = h.pool.Acquire(ctx)
		if err != nil {
			return models.NewErrorResponse("failed to acquire browser: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
		}

		// Create page (stealth mode unless disabled for testing)
		page, err = browser.CreatePage(managedBrowser.Browser, h.cfg.DisableStealth)
		if err != nil {
			h.pool.Release(managedBrowser)
			return models.NewErrorResponse("failed to create page: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
		}
		cleanup = func() {
			page.Close()
			h.pool.Release(managedBrowser)
		}
	}
	defer cleanup()

	// Set cookies if provided
	if len(req.Cookies) > 0 {
		if err := h.setCookies(page, req.URL, req.Cookies); err != nil {
			h.logger.Warn("failed to set cookies", "error", err)
		}
	}

	// Set custom user agent if provided
	if req.UserAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: req.UserAgent,
		}); err != nil {
			h.logger.Warn("failed to set user agent", "error", err)
		}
	}

	// Navigate to URL
	if err := page.Navigate(req.URL); err != nil {
		return models.NewErrorResponse("failed to navigate: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	// Wait for initial load
	if err := page.WaitLoad(); err != nil {
		return models.NewErrorResponse("failed to wait for load: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	// Detect challenge
	detection, err := h.detector.Detect(ctx, page)
	if err != nil {
		return models.NewErrorResponse("failed to detect challenge: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	// Challenge tracking
	var (
		challengeType string
		solverUsed    string
		challenged    bool
		solved        bool
		resolveMethod string
	)

	// Handle challenge if detected
	if detection.Type != challenge.TypeNone {
		challenged = true
		challengeType = string(detection.Type)
		h.logger.Info("challenge detected",
			"user_id", userID,
			"job_id", jobID,
			"type", detection.Type,
			"can_auto", detection.CanAuto,
			"session", req.Session,
			"url", req.URL,
		)

		// Use solver chain for all challenge types
		if h.solver != nil && h.solver.CanSolve(detection.Type) {
			h.logger.Debug("attempting to solve challenge",
				"user_id", userID,
				"job_id", jobID,
				"type", detection.Type,
				"solver", h.solver.Name(),
				"timeout", timeout,
			)
			result, err := h.solver.Solve(ctx, solver.SolveParams{
				Type:    detection.Type,
				SiteKey: detection.SiteKey,
				PageURL: detection.PageURL,
				Action:  detection.Action,
				CData:   detection.CData,
				Page:    page,
				Timeout: timeout,
			})
			if err != nil {
				h.logger.Warn("challenge solve failed",
					"user_id", userID,
					"job_id", jobID,
					"type", detection.Type,
					"error", err,
					"session", req.Session,
					"url", req.URL,
				)
				return models.NewErrorResponse("failed to solve challenge: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
			}

			solverUsed = result.SolverName
			solved = true
			resolveMethod = result.SolverName
			h.logger.Info("challenge solved",
				"user_id", userID,
				"job_id", jobID,
				"type", detection.Type,
				"solver", result.SolverName,
				"session", req.Session,
				"url", req.URL,
			)

			// Inject token if provided (external solvers return tokens, wait solver doesn't)
			if result.Token != "" {
				if err := h.injectCaptchaToken(page, detection, result.Token); err != nil {
					return models.NewErrorResponse("failed to inject CAPTCHA token: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
				}
				// Wait for page to process token
				time.Sleep(2 * time.Second)
			}
		} else {
			return models.NewErrorResponse("challenge detected but no solver available for type: "+string(detection.Type), startTime, time.Now().UnixMilli(), ver, "")
		}
	} else {
		// No challenge detected
		challenged = false
		solved = false

		if req.Session != "" && sess != nil && len(sess.Cookies) > 0 {
			resolveMethod = "cached"
			h.logger.Info("no challenge - session cookies reused successfully",
				"user_id", userID,
				"job_id", jobID,
				"session", req.Session,
				"cookie_count", len(sess.Cookies),
				"request_count", sess.RequestCount,
				"url", req.URL,
				"method", resolveMethod,
			)
		} else {
			h.logger.Debug("no challenge detected",
				"user_id", userID,
				"job_id", jobID,
				"session", req.Session,
				"url", req.URL,
			)
		}
	}

	// Handle wait condition if specified
	if req.WaitFor != nil {
		if err := h.handleWaitCondition(ctx, page, req.WaitFor); err != nil {
			h.logger.Warn("wait condition failed", "error", err)
		}
	}

	// Get final page info
	info, err := page.Info()
	if err != nil {
		return models.NewErrorResponse("failed to get page info: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	// Get HTML content
	html, err := page.HTML()
	if err != nil {
		return models.NewErrorResponse("failed to get HTML: "+err.Error(), startTime, time.Now().UnixMilli(), ver, "")
	}

	// Get cookies
	cookies, err := page.Cookies(nil)
	if err != nil {
		h.logger.Warn("failed to get cookies", "error", err)
		cookies = nil
	}

	// Get user agent
	userAgent := ""
	result, err := page.Eval(`() => navigator.userAgent`)
	if err == nil {
		userAgent = result.Value.Str()
	}

	solution := &models.Solution{
		URL:       info.URL,
		Status:    200,
		Cookies:   h.convertCookies(cookies),
		UserAgent: userAgent,
		Response:  html,
		Title:     info.Title,
	}

	// Take screenshot if requested
	if req.Screenshot {
		screenshot, err := page.Screenshot(false, nil)
		if err == nil {
			solution.Screenshot = base64.StdEncoding.EncodeToString(screenshot)
		}
	}

	resp := models.NewSuccessResponse(solution, startTime, time.Now().UnixMilli(), ver, "")
	resp.ChallengeType = challengeType
	resp.SolverUsed = solverUsed
	resp.Challenged = challenged
	resp.Solved = solved
	resp.Method = resolveMethod

	return resp
}

// setCookies sets cookies on the page.
func (h *SolveHandler) setCookies(page *rod.Page, url string, cookies []models.Cookie) error {
	var cookieParams []*proto.NetworkCookieParam
	for _, c := range cookies {
		domain := c.Domain
		if domain == "" {
			// Extract domain from URL
			parts := strings.Split(url, "/")
			if len(parts) >= 3 {
				domain = parts[2]
			}
		}

		param := &proto.NetworkCookieParam{
			Name:   c.Name,
			Value:  c.Value,
			Domain: domain,
			Path:   c.Path,
		}
		if c.Expires > 0 {
			expires := proto.TimeSinceEpoch(c.Expires)
			param.Expires = expires
		}
		if c.Secure {
			param.Secure = true
		}
		if c.HTTPOnly {
			param.HTTPOnly = true
		}
		if c.SameSite != "" {
			switch strings.ToLower(c.SameSite) {
			case "strict":
				param.SameSite = proto.NetworkCookieSameSiteStrict
			case "lax":
				param.SameSite = proto.NetworkCookieSameSiteLax
			case "none":
				param.SameSite = proto.NetworkCookieSameSiteNone
			}
		}
		cookieParams = append(cookieParams, param)
	}

	return proto.NetworkSetCookies{Cookies: cookieParams}.Call(page)
}

// convertCookies converts proto cookies to model cookies.
func (h *SolveHandler) convertCookies(cookies []*proto.NetworkCookie) []models.Cookie {
	if cookies == nil {
		return nil
	}

	result := make([]models.Cookie, len(cookies))
	for i, c := range cookies {
		sameSite := ""
		switch c.SameSite {
		case proto.NetworkCookieSameSiteStrict:
			sameSite = "Strict"
		case proto.NetworkCookieSameSiteLax:
			sameSite = "Lax"
		case proto.NetworkCookieSameSiteNone:
			sameSite = "None"
		}

		result[i] = models.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  int64(c.Expires),
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: sameSite,
		}
	}
	return result
}

// handleWaitCondition waits for the specified condition.
func (h *SolveHandler) handleWaitCondition(ctx context.Context, page *rod.Page, cond *models.WaitCondition) error {
	if cond.Selector != "" {
		_, err := page.Timeout(30 * time.Second).Element(cond.Selector)
		if err != nil {
			return err
		}
	}

	if cond.Delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(cond.Delay) * time.Millisecond):
		}
	}

	if cond.NetworkIdle {
		if err := page.WaitIdle(30 * time.Second); err != nil {
			return err
		}
	}

	if cond.Load {
		if err := page.WaitLoad(); err != nil {
			return err
		}
	}

	return nil
}

// injectCaptchaToken injects a CAPTCHA token into the page.
func (h *SolveHandler) injectCaptchaToken(page *rod.Page, detection *challenge.Detection, token string) error {
	switch detection.Type {
	case challenge.TypeCloudflareTurnstile:
		// Inject token into Turnstile response field
		_, err := page.Eval(`(token) => {
			const input = document.querySelector('[name="cf-turnstile-response"]');
			if (input) {
				input.value = token;
			}
			// Also try callback
			if (window.turnstileCallback) {
				window.turnstileCallback(token);
			}
		}`, token)
		return err

	case challenge.TypeHCaptcha:
		_, err := page.Eval(`(token) => {
			const textarea = document.querySelector('[name="h-captcha-response"]');
			if (textarea) {
				textarea.value = token;
			}
			const input = document.querySelector('[name="g-recaptcha-response"]');
			if (input) {
				input.value = token;
			}
			// Trigger callback
			if (window.hcaptchaCallback) {
				window.hcaptchaCallback(token);
			}
		}`, token)
		return err

	case challenge.TypeReCaptchaV2, challenge.TypeReCaptchaV3:
		_, err := page.Eval(`(token) => {
			const textarea = document.querySelector('[name="g-recaptcha-response"]');
			if (textarea) {
				textarea.value = token;
			}
			// Trigger callback
			if (window.grecaptchaCallback) {
				window.grecaptchaCallback(token);
			}
		}`, token)
		return err

	default:
		return nil
	}
}
