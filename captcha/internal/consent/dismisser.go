// Package consent provides cookie consent banner dismissal for browser automation.
package consent

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Common cookie consent button selectors, ordered by specificity and reliability.
// These cover the most common consent management platforms (CMP) and custom implementations.
var consentButtonSelectors = []string{
	// Instructables/Autodesk specific (OneTrust)
	`button#onetrust-accept-btn-handler`,
	`button.onetrust-close-btn-handler`,
	`#onetrust-accept-btn-handler`,

	// OneTrust (common CMP)
	`button[id*="onetrust-accept"]`,
	`button[class*="onetrust-accept"]`,
	`#accept-recommended-btn-handler`,

	// Cookiebot
	`button#CybotCookiebotDialogBodyLevelButtonLevelOptinAllowAll`,
	`button#CybotCookiebotDialogBodyButtonAccept`,
	`a#CybotCookiebotDialogBodyLevelButtonLevelOptinAllowAll`,

	// Quantcast/TCF
	`button.qc-cmp2-summary-buttons button[mode="primary"]`,
	`button.qc-cmp-button`,
	`button[class*="qc-cmp"]`,

	// TrustArc
	`button.trustarc-agree-btn`,
	`a.call[onclick*="accept"]`,
	`#truste-consent-button`,

	// Didomi
	`button#didomi-notice-agree-button`,
	`button[class*="didomi-agree"]`,

	// Generic "Accept" buttons (common patterns)
	`button[data-testid="cookie-policy-dialog-accept-button"]`,
	`button[data-testid="accept-cookies"]`,
	`button[data-testid="cookie-accept"]`,
	`button[aria-label*="Accept"]`,
	`button[aria-label*="accept"]`,
	`button[aria-label*="Agree"]`,

	// Common class/id patterns
	`button.cookie-accept`,
	`button.accept-cookies`,
	`button.consent-accept`,
	`button.gdpr-accept`,
	`button#accept-cookies`,
	`button#acceptCookies`,
	`button#cookie-accept`,
	`button#cookieAccept`,
	`a.cookie-accept`,
	`a.accept-cookies`,

	// Common text-based selectors (buttons containing specific text)
	`button:has-text("Accept All")`,
	`button:has-text("Accept all")`,
	`button:has-text("Accept Cookies")`,
	`button:has-text("Accept cookies")`,
	`button:has-text("I Accept")`,
	`button:has-text("I agree")`,
	`button:has-text("Got it")`,
	`button:has-text("OK")`,
	`a:has-text("Accept All")`,
	`a:has-text("Accept all")`,
	`a:has-text("I Accept")`,
	`a:has-text("I agree")`,

	// Generic modal close buttons (less specific, last resort)
	`button[class*="accept"][class*="cookie"]`,
	`button[class*="cookie"][class*="accept"]`,
	`div[class*="cookie"] button[class*="accept"]`,
	`div[class*="consent"] button[class*="accept"]`,
	`div[class*="gdpr"] button[class*="accept"]`,
}

// Dismisser handles cookie consent banner dismissal.
type Dismisser struct {
	logger  *slog.Logger
	timeout time.Duration
}

// NewDismisser creates a new cookie consent dismisser.
func NewDismisser(logger *slog.Logger) *Dismisser {
	return &Dismisser{
		logger:  logger,
		timeout: 2 * time.Second, // Short timeout - don't wait long for consent banners
	}
}

// Dismiss attempts to dismiss any cookie consent banner on the page.
// It tries multiple common selectors and clicks the first matching element.
// Returns true if a consent banner was dismissed, false otherwise.
func (d *Dismisser) Dismiss(ctx context.Context, page *rod.Page) bool {
	// First, wait a brief moment for any consent banners to render
	time.Sleep(500 * time.Millisecond)

	// Try each selector
	for _, selector := range consentButtonSelectors {
		// Skip :has-text selectors as rod doesn't support them directly
		if strings.Contains(selector, ":has-text") {
			continue
		}

		dismissed := d.tryClickSelector(ctx, page, selector)
		if dismissed {
			return true
		}
	}

	// Try text-based search as fallback
	if d.tryClickByText(ctx, page) {
		return true
	}

	return false
}

// tryClickSelector attempts to find and click an element matching the selector.
func (d *Dismisser) tryClickSelector(ctx context.Context, page *rod.Page, selector string) bool {
	// Use a short timeout to avoid blocking
	elem, err := page.Timeout(d.timeout).Element(selector)
	if err != nil {
		return false
	}

	// Check if element is visible
	visible, err := elem.Visible()
	if err != nil || !visible {
		return false
	}

	// Try to click the element
	err = elem.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		d.logger.Debug("failed to click consent button",
			"selector", selector,
			"error", err,
		)
		return false
	}

	d.logger.Info("dismissed cookie consent banner",
		"selector", selector,
	)

	// Wait briefly for the banner to be dismissed
	time.Sleep(300 * time.Millisecond)

	return true
}

// tryClickByText attempts to find and click buttons by their text content.
func (d *Dismisser) tryClickByText(ctx context.Context, page *rod.Page) bool {
	acceptTexts := []string{
		"Accept All",
		"Accept all",
		"Accept All Cookies",
		"Accept Cookies",
		"I Accept",
		"I Agree",
		"Got it",
		"Allow All",
		"Allow all",
		"Agree",
	}

	for _, text := range acceptTexts {
		// Try button first
		js := `(text) => {
			const buttons = document.querySelectorAll('button');
			for (const btn of buttons) {
				if (btn.textContent.trim() === text || btn.textContent.includes(text)) {
					const rect = btn.getBoundingClientRect();
					if (rect.width > 0 && rect.height > 0) {
						return btn;
					}
				}
			}
			// Also check anchor tags
			const links = document.querySelectorAll('a');
			for (const link of links) {
				if (link.textContent.trim() === text || link.textContent.includes(text)) {
					const rect = link.getBoundingClientRect();
					if (rect.width > 0 && rect.height > 0) {
						return link;
					}
				}
			}
			return null;
		}`

		result, err := page.Timeout(d.timeout).Eval(js, text)
		if err != nil || result.Value.Nil() {
			continue
		}

		// Click the element using JavaScript
		clickJS := `(text) => {
			const buttons = document.querySelectorAll('button');
			for (const btn of buttons) {
				if (btn.textContent.trim() === text || btn.textContent.includes(text)) {
					btn.click();
					return true;
				}
			}
			const links = document.querySelectorAll('a');
			for (const link of links) {
				if (link.textContent.trim() === text || link.textContent.includes(text)) {
					link.click();
					return true;
				}
			}
			return false;
		}`

		clickResult, err := page.Timeout(d.timeout).Eval(clickJS, text)
		if err == nil && clickResult.Value.Bool() {
			d.logger.Info("dismissed cookie consent banner",
				"method", "text_search",
				"text", text,
			)
			time.Sleep(300 * time.Millisecond)
			return true
		}
	}

	return false
}
