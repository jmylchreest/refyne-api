# Refyne Captcha Service - Initial Research & Plan

## Executive Summary

This document outlines the research findings and proposed architecture for building a scalable captcha/anti-bot bypass service integrated with the Refyne API. The service will provide FlareSolverr-compatible APIs while being implemented in Go for single-binary deployment.

**Key Finding:** WASM/Cloudflare Workers is **not viable** for this use case. Browser automation requires real browser processes, system threading, and filesystem access - none of which are available in WASM environments.

---

## Table of Contents

1. [Understanding FlareSolverr](#understanding-flaresolverr)
2. [How Anti-Bot Detection Works](#how-anti-bot-detection-works)
3. [Undetected-ChromeDriver Deep Dive](#undetected-chromedriver-deep-dive)
4. [CAPTCHA Solving Services](#captcha-solving-services)
5. [Go Browser Automation Options](#go-browser-automation-options)
6. [Alternative: Camoufox](#alternative-camoufox)
7. [Why WASM Won't Work](#why-wasm-wont-work)
8. [Proposed Architecture](#proposed-architecture)
9. [Implementation Plan](#implementation-plan)
10. [Cost Analysis](#cost-analysis)
11. [References](#references)

---

## Understanding FlareSolverr

[FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) is an open-source proxy server that bypasses Cloudflare and DDoS-GUARD protection.

### How It Works

1. **Request Reception**: Client sends a request to FlareSolverr's JSON-RPC API
2. **Browser Launch**: FlareSolverr spawns a headless Chrome instance via Selenium
3. **Challenge Detection**: Navigates to the target URL, detects protection type
4. **Challenge Resolution**: Waits for Cloudflare's JavaScript challenge to auto-resolve
5. **Session Capture**: Extracts cookies, user-agent, and HTML content
6. **Response**: Returns solution to client for reuse in subsequent requests

### Technology Stack

- **Python 3.13** backend
- **Selenium WebDriver** for browser automation
- **[undetected-chromedriver](https://github.com/ultrafunkamsterdam/undetected-chromedriver)** - patched ChromeDriver
- **Chrome/Chromium** browser (bundled in Docker images)

### API Endpoints

```
POST http://localhost:8191/v1

Commands:
- sessions.create    # Create persistent browser session
- sessions.list      # List active sessions
- sessions.destroy   # Terminate browser session
- request.get        # Fetch URL with challenge bypass
- request.post       # POST request with challenge bypass
```

### Limitations

- **Resource Heavy**: ~100-200MB RAM per browser instance
- **CAPTCHA Solving Broken**: Documentation states "none of the captcha solvers work"
- **Arms Race**: Cloudflare constantly updates detection methods
- **Single-Threaded Python**: Bottleneck at scale

Sources:
- [FlareSolverr GitHub](https://github.com/FlareSolverr/FlareSolverr)
- [ZenRows FlareSolverr Guide](https://www.zenrows.com/blog/flaresolverr)
- [ScrapFly FlareSolverr Guide](https://scrapfly.io/blog/posts/how-to-bypass-cloudflare-with-flaresolverr)

---

## How Anti-Bot Detection Works

Cloudflare and similar services detect bots through multiple layers:

### 1. Browser Fingerprinting

| Check | What It Detects |
|-------|-----------------|
| `navigator.webdriver` | True when automated |
| `navigator.plugins` | Empty in headless mode |
| User-Agent | "HeadlessChrome" flag |
| `window.chrome` | Missing in some automations |
| WebGL fingerprint | GPU rendering inconsistencies |
| Canvas fingerprint | Rendering differences |
| Audio fingerprint | Audio processing variations |
| Screen/viewport | Unusual dimensions |
| Timezone/locale | Mismatches with IP geolocation |

### 2. Behavioral Analysis

- Mouse movement patterns (or lack thereof)
- Keyboard events timing
- Scroll behavior
- Request timing patterns
- Navigation flow

### 3. Network Analysis

- IP reputation (datacenter IPs flagged)
- Request headers order/format
- TLS fingerprinting (JA3/JA4)
- HTTP/2 settings fingerprint

### 4. JavaScript Challenges

- Proof-of-work computations
- Browser environment validation
- Runtime behavior checks

Sources:
- [ZenRows - How to Bypass Cloudflare](https://www.zenrows.com/blog/bypass-cloudflare)
- [Oxylabs - Undetected ChromeDriver](https://oxylabs.io/blog/undetected-chromedriver)

---

## Undetected-ChromeDriver Deep Dive

[undetected-chromedriver](https://github.com/ultrafunkamsterdam/undetected-chromedriver) is the core technology that makes FlareSolverr work.

### Key Patching Techniques

#### 1. `cdc_` Variable Prevention

The ChromeDriver exposes a `cdc_` prefixed variable that anti-bot systems detect. Version 3.4.0 rewrote the approach:

> "Instead of removing and renaming variables, we just keep them, but prevent them from being injected in the first place."

This is more resilient than simple find-and-replace patching.

#### 2. Modified Browser Launch Process

Standard Selenium:
```
ChromeDriver starts → Launches Chrome with driver attached → Detectable
```

Undetected approach:
```
Launch Chrome as standalone process → Attach driver afterwards → Mimics human startup
```

#### 3. User-Agent Modification

Removes telltale signs like:
- `HeadlessChrome` in user-agent string
- Headless-specific viewport sizes
- Missing GPU/graphics info

#### 4. WebDriver Property Patching

```javascript
// Standard Chrome (detectable)
navigator.webdriver === true

// Patched Chrome
navigator.webdriver === undefined  // or false
```

#### 5. Plugin/Extension Simulation

Populates `navigator.plugins` with realistic browser plugins that are empty in headless mode.

### Successor: NoDriver

[NoDriver](https://github.com/ultrafunkamsterdam/nodriver) is the official successor:

> "NoDriver is an asynchronous tool that replaces traditional components such as Selenium or webdriver binaries, providing direct communication with browsers."

Benefits:
- No WebDriver binary (direct CDP communication)
- Lower detection rate
- Better performance
- Async-first design

Sources:
- [undetected-chromedriver GitHub](https://github.com/ultrafunkamsterdam/undetected-chromedriver)
- [ScrapingBee Tutorial](https://www.scrapingbee.com/blog/undetected-chromedriver-python-tutorial-avoiding-bot-detection/)
- [Rebrowser Guide](https://rebrowser.net/blog/undetected-chromedriver-the-ultimate-guide-to-bypassing-bot-detection)

---

## CAPTCHA Solving Services

When browser automation alone cannot solve CAPTCHAs (Turnstile, hCaptcha, reCAPTCHA), external services are needed.

### How 2Captcha Works

[2Captcha](https://2captcha.com) is a human-powered CAPTCHA solving service.

#### Workflow

```
1. Your Code                    2. 2Captcha API              3. Human Workers
   │                               │                            │
   ├─► Submit CAPTCHA params ────► │                            │
   │   (sitekey, URL, type)        │                            │
   │                               ├─► Distribute to worker ──► │
   │                               │                            │
   │                               │ ◄── Solve CAPTCHA ────────┤
   │                               │                            │
   │ ◄── Return token ────────────┤                            │
   │                               │                            │
   ├─► Inject token into page                                   │
   │                                                            │
   └─► Continue automation
```

#### Cloudflare Turnstile Solving

```python
# Pseudocode for Turnstile solving
result = solver.turnstile(
    sitekey="0x4AAAAAAABS...",  # data-sitekey from page
    url="https://example.com",
    action="login",             # optional
    data="custom_data"          # optional
)
# Returns: {"token": "0.Qz0D..."}
# Inject token into cf-turnstile-response hidden field
```

#### Pricing (as of 2025)

| CAPTCHA Type | Price per 1000 |
|--------------|----------------|
| Cloudflare Turnstile | $1.45 |
| hCaptcha | $2.99 |
| reCAPTCHA v2 | $2.99 |
| reCAPTCHA v3 | $2.99 |

#### Alternative Services

| Service | Strengths |
|---------|-----------|
| [CapSolver](https://capsolver.com) | AI-powered, faster, API-focused |
| [Anti-Captcha](https://anti-captcha.com) | Similar to 2Captcha, competitive pricing |
| [Death By Captcha](https://deathbycaptcha.com) | Older service, reliable |

Sources:
- [2Captcha API Documentation](https://2captcha.com/api-docs)
- [2Captcha Turnstile Solver](https://2captcha.com/p/cloudflare-turnstile)
- [2Captcha JavaScript Library](https://github.com/2captcha/2captcha-javascript)

---

## Go Browser Automation Options

### Option 1: go-rod/rod

[Rod](https://github.com/go-rod/rod) is a high-level DevTools Protocol driver for Go.

```go
package main

import "github.com/go-rod/rod"

func main() {
    browser := rod.New().MustConnect()
    defer browser.MustClose()

    page := browser.MustPage("https://example.com")
    html := page.MustHTML()
}
```

#### With Stealth Plugin

[go-rod/stealth](https://github.com/go-rod/stealth) adds anti-detection features:

```go
import (
    "github.com/go-rod/rod"
    "github.com/go-rod/stealth"
)

func main() {
    browser := rod.New().MustConnect()
    page := stealth.MustPage(browser)  // Applies stealth patches
    page.MustNavigate("https://example.com")
}
```

**Stealth Features:**
- Ports puppeteer-extra stealth evasions to Go
- Patches navigator.webdriver
- Simulates plugins
- Modifies permissions

**Limitations:**
- Less actively maintained than Python alternatives
- May not keep pace with Cloudflare updates
- No equivalent to undetected-chromedriver's binary patching

#### Custom Chrome Binary

Rod supports custom browser binaries:

```go
import "github.com/go-rod/rod/lib/launcher"

func main() {
    // Use custom patched Chrome binary
    url := launcher.New().
        Bin("/path/to/patched-chrome").
        Set("disable-blink-features", "AutomationControlled").
        MustLaunch()

    browser := rod.New().ControlURL(url).MustConnect()
}
```

### Option 2: chromedp

[chromedp](https://github.com/chromedp/chromedp) is a lower-level CDP library:

```go
import "github.com/chromedp/chromedp"

func main() {
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()

    var html string
    err := chromedp.Run(ctx,
        chromedp.Navigate("https://example.com"),
        chromedp.OuterHTML("html", &html),
    )
}
```

**Comparison:**

| Feature | go-rod | chromedp |
|---------|--------|----------|
| Abstraction Level | High | Low |
| Stealth Plugin | Yes (go-rod/stealth) | No (manual) |
| Auto-download browser | Yes | No |
| Session management | Built-in | Manual |
| Learning curve | Easier | Steeper |

### Key Insight: Rod Doesn't Need Binary Patching!

After examining FlareSolverr's actual source code, here's what undetected-chromedriver does:

**1. Binary Patching (`patcher.py`):**
```python
# Searches chromedriver binary for:
match_injected_codeblock = re.search(rb"\{window\.cdc.*?;\}", content)
# Replaces with placeholder to prevent cdc_ variable injection
```

**2. JavaScript Injection (`__init__.py`):**
- Masks `navigator.webdriver`
- Removes "Headless" from user-agent
- Spoofs touch capabilities
- Emulates `window.chrome` runtime
- Modifies permission query behavior

**Why rod doesn't need the binary patching:**

| Aspect | Selenium + ChromeDriver | go-rod |
|--------|------------------------|--------|
| Communication | Through chromedriver binary | Direct CDP (Chrome DevTools Protocol) |
| `cdc_` variables | Injected by chromedriver | **Not present - no chromedriver!** |
| Binary patching needed | Yes | **No** |
| JS injection needed | Yes | Yes (go-rod/stealth provides this) |

**Rod communicates directly with Chrome via CDP**, bypassing the chromedriver entirely. The `cdc_` variable detection that FlareSolverr patches out simply doesn't exist when using rod.

The JavaScript injection layer (stealth evasions) is still needed, and go-rod/stealth provides this by embedding the same [puppeteer-extra-plugin-stealth](https://www.npmjs.com/package/puppeteer-extra-plugin-stealth) evasions.

### What go-rod/stealth Provides

From `extract-stealth-evasions@latest` (puppeteer-extra):
- `navigator.webdriver` removal (ES6 Proxy-based)
- `chrome.app`, `chrome.csi`, `chrome.loadTimes` mocking
- `navigator.plugins` population
- `navigator.languages` spoofing
- WebGL vendor/renderer masking
- `iframe.contentWindow` fixes
- Permission query normalization
- And [17 total evasion modules](https://github.com/berstend/puppeteer-extra/tree/master/packages/puppeteer-extra-plugin-stealth)

### Existing Refyne CLI Stealth Code

The refyne CLI already has comprehensive stealth implementation in `cmd/refyne/fetcher/stealth.go`:
- 230+ lines of JavaScript evasions
- Chrome flags for anti-detection
- WebGL fingerprint masking
- `chrome.runtime` mocking

This can be directly ported to the captcha service.

### Rod CSS Selector Support

Rod has **full CSS selector support**, equivalent to chromedp:

```go
// Single element by CSS selector
el, err := page.Element("div.product > a.link")
el := page.MustElement("input[type='text']")

// Multiple elements
els, err := page.Elements("ul.items li")
els := page.MustElements(".product-card")

// Check existence
has, el, err := page.Has("button.submit")
has := page.MustHas(".error-message")

// XPath support
el := page.MustElementX("//button[@id='submit']")

// Regex filtering on text content
el := page.MustElementR("span", "Add to Cart")

// JavaScript-based selection
el := page.MustElementByJS(`() => document.querySelector('.dynamic-element')`)

// Wait for element
el := page.Timeout(10*time.Second).MustElement(".loaded-content")

// Chained operations
page.MustElement("input.search").MustInput("query").MustType(input.Enter)

// Collection methods
els := page.MustElements("input")
first := els.First()
last := els.Last()
```

**Comparison with refyne CLI's chromedp usage:**

| chromedp Pattern | Rod Equivalent |
|------------------|----------------|
| `chromedp.Navigate(url)` | `page.MustNavigate(url)` |
| `chromedp.WaitReady(selector)` | `page.MustElement(selector).MustWaitLoad()` |
| `chromedp.OuterHTML("html", &html)` | `html := page.MustHTML()` |
| `chromedp.Title(&title)` | `title := page.MustInfo().Title` |
| `chromedp.Sleep(duration)` | `page.MustWaitStable()` or `time.Sleep()` |
| `chromedp.CaptureScreenshot(&buf)` | `buf := page.MustScreenshot()` |
| `network.SetCookies(cookies)` | `page.MustSetCookies(cookies...)` |

Sources:
- [go-rod/rod GitHub](https://github.com/go-rod/rod)
- [go-rod/stealth GitHub](https://github.com/go-rod/stealth)
- [Rod API Documentation](https://pkg.go.dev/github.com/go-rod/rod)
- [chromedp GitHub](https://github.com/chromedp/chromedp)
- [Rod Custom Launch Docs](https://github.com/go-rod/go-rod.github.io/blob/main/custom-launch.md)

---

## Alternative: Camoufox

[Camoufox](https://github.com/daijro/camoufox) is an open-source anti-detect browser built on Firefox.

### Why It's Interesting

1. **C++ Level Fingerprinting**: Unlike JavaScript injection (detectable), Camoufox modifies device info at the C++ level

2. **Playwright Sandboxing**: All Playwright JavaScript is isolated, making detection via JS inspection impossible

3. **Human-like Cursor**: Built-in natural mouse movement algorithm (C++ implementation)

4. **Comprehensive Fingerprint Spoofing**:
   - Navigator properties
   - Screen properties
   - WebGL/Canvas
   - Audio context
   - Geolocation
   - Battery API

### Current Status (2025)

- Active development (FF v146 as of January 2026)
- Python library (recommended over launcher)
- Fork maintained while original author recovers from medical emergency

### Limitations

- **Python-only**: No Go bindings
- **Firefox-based**: Some WAFs specifically test for Chromium behaviors
- **Not production-ready**: "May not be reliable for high-volume, mission-critical operations"

### Potential Integration

Could run Camoufox as a sidecar container, orchestrated by Go service:

```
Go Service → gRPC/HTTP → Camoufox Container → Target Site
```

Sources:
- [Camoufox GitHub](https://github.com/daijro/camoufox)
- [Camoufox Documentation](https://camoufox.com/)
- [ZenRows Camoufox Guide](https://www.zenrows.com/blog/web-scraping-with-camoufox)
- [ScrapingBee Camoufox Tutorial](https://www.scrapingbee.com/blog/how-to-scrape-with-camoufox-to-bypass-antibot-technology/)

---

## Why WASM Won't Work

### Technical Constraints

From [Cloudflare's WebAssembly documentation](https://developers.cloudflare.com/workers/runtime-apis/webassembly/):

1. **No Threading**: "Threading is not possible in Workers. Each Worker runs in a single thread."

2. **No System I/O**: "WebAssembly does not provide any standard interface for I/O tasks such as interacting with files, accessing the network, or reading the system clock."

3. **Module Size Limits**: "Wasm modules end up like statically-linked binaries – they include not just your program itself, but also your programming language's entire standard library."

4. **Performance**: "Code that mostly interacts with external objects without doing any serious number crunching likely does not benefit from WASM."

### What About Cloudflare Browser Rendering?

[Cloudflare Browser Rendering](https://developers.cloudflare.com/browser-rendering/) offers serverless Puppeteer:

> "The userAgent parameter does not bypass bot protection. Requests from Browser Rendering will **always be identified as a bot**."

This is by design - Cloudflare won't help you bypass their own protection.

### Bottom Line

Browser automation requires:
- Real browser processes (Chrome/Firefox binaries)
- Multi-threading for browser rendering
- File system access for browser profiles
- Network stack control for proxy integration

None of these are available in WASM or Cloudflare Workers.

Sources:
- [Cloudflare WASM Docs](https://developers.cloudflare.com/workers/runtime-apis/webassembly/)
- [Cloudflare Browser Rendering FAQ](https://developers.cloudflare.com/browser-rendering/faq/)
- [Cloudflare Blog - WASI on Workers](https://blog.cloudflare.com/announcing-wasi-on-workers/)

---

## Proposed Architecture

### High-Level Design

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           REFYNE API (Existing)                         │
│                 Authentication, Rate Limiting, Quota, Billing           │
└─────────────────────────────────────────────────────────────────────────┘
                                      │
                                      │ Internal API (mTLS/HMAC signed)
                                      ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      CAPTCHA SERVICE (New - Go Binary)                  │
│  ┌──────────────┐  ┌───────────────┐  ┌────────────────────────────┐   │
│  │   Huma API   │  │   Challenge   │  │     Session Manager        │   │
│  │   Router     │──│   Detector    │──│  (Cookies, Fingerprints)   │   │
│  └──────────────┘  └───────────────┘  └────────────────────────────┘   │
│          │                 │                        │                   │
│          ▼                 ▼                        ▼                   │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                    Browser Pool Manager                          │  │
│  │   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────────┐  │  │
│  │   │ Browser1 │  │ Browser2 │  │ BrowserN │  │ Health Monitor │  │  │
│  │   │ (rod)    │  │ (rod)    │  │ (rod)    │  │                │  │  │
│  │   └──────────┘  └──────────┘  └──────────┘  └────────────────┘  │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│          │                                              │               │
│          ▼                                              ▼               │
│  ┌────────────────────┐                    ┌────────────────────────┐  │
│  │   Solver Plugins   │                    │   Usage Tracking       │  │
│  │  ┌─────────────┐   │                    │  (→ Refyne DB)         │  │
│  │  │  2Captcha   │   │                    └────────────────────────┘  │
│  │  ├─────────────┤   │                                                │
│  │  │  CapSolver  │   │                                                │
│  │  ├─────────────┤   │                                                │
│  │  │  Internal   │   │                                                │
│  │  └─────────────┘   │                                                │
│  └────────────────────┘                                                │
└─────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        SCALING INFRASTRUCTURE                           │
│  ┌─────────────────┐  ┌──────────────────┐  ┌─────────────────────┐    │
│  │  Fly Machines   │  │   Redis/Valkey   │  │    Proxy Pool       │    │
│  │  Auto-scaling   │  │   Job Queue      │  │  (Residential IPs)  │    │
│  └─────────────────┘  └──────────────────┘  └─────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Component Details

#### 1. API Layer (Huma/Chi)

FlareSolverr-compatible + extensions:

```go
// FlareSolverr compatible request
type SolveRequest struct {
    Cmd        string            `json:"cmd"`              // "request.get" | "request.post"
    URL        string            `json:"url"`
    Session    string            `json:"session,omitempty"`
    MaxTimeout int               `json:"maxTimeout"`       // ms
    Cookies    []Cookie          `json:"cookies,omitempty"`
    Proxy      *ProxyConfig      `json:"proxy,omitempty"`
    // Refyne Extensions
    UserAgent  string            `json:"userAgent,omitempty"`
    Headers    map[string]string `json:"headers,omitempty"`
    WaitFor    *WaitCondition    `json:"waitFor,omitempty"`
    Screenshot bool              `json:"screenshot,omitempty"`
}

// FlareSolverr compatible response
type SolveResponse struct {
    Status    string    `json:"status"`    // "ok" | "error"
    Message   string    `json:"message"`
    Solution  *Solution `json:"solution,omitempty"`
    StartTime int64     `json:"startTimestamp"`
    EndTime   int64     `json:"endTimestamp"`
    Version   string    `json:"version"`
    // Refyne Extensions
    Usage     *UsageInfo `json:"usage,omitempty"`
    RequestID string     `json:"requestId,omitempty"`
}

type Solution struct {
    URL       string            `json:"url"`
    Status    int               `json:"status"`
    Headers   map[string]string `json:"headers,omitempty"`
    Cookies   []Cookie          `json:"cookies"`
    UserAgent string            `json:"userAgent"`
    Response  string            `json:"response"`  // HTML content
}
```

#### 2. Browser Pool Manager

```go
type BrowserPool struct {
    mu          sync.RWMutex
    browsers    []*ManagedBrowser
    maxSize     int
    idleTimeout time.Duration
    launcher    *launcher.Launcher
}

type ManagedBrowser struct {
    ID          string
    Browser     *rod.Browser
    InUse       bool
    CreatedAt   time.Time
    LastUsedAt  time.Time
    RequestCount int
    Proxy       *ProxyConfig
}

func (p *BrowserPool) Acquire(ctx context.Context) (*ManagedBrowser, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Find idle browser or create new one
    for _, b := range p.browsers {
        if !b.InUse {
            b.InUse = true
            b.LastUsedAt = time.Now()
            return b, nil
        }
    }

    if len(p.browsers) < p.maxSize {
        return p.createBrowser(ctx)
    }

    return nil, ErrPoolExhausted
}
```

#### 3. Challenge Detector

```go
type ChallengeType string

const (
    ChallengeNone       ChallengeType = "none"
    ChallengeCloudflare ChallengeType = "cloudflare"
    ChallengeTurnstile  ChallengeType = "turnstile"
    ChallengeDDoSGuard  ChallengeType = "ddosguard"
    ChallengeHCaptcha   ChallengeType = "hcaptcha"
    ChallengeReCaptcha  ChallengeType = "recaptcha"
)

func DetectChallenge(page *rod.Page) (ChallengeType, error) {
    // Check page title for Cloudflare patterns
    title := page.MustEval(`document.title`).String()
    if strings.Contains(title, "Just a moment") ||
       strings.Contains(title, "Checking your browser") {
        return ChallengeCloudflare, nil
    }

    // Check for Turnstile iframe
    has := page.MustHas(`iframe[src*="challenges.cloudflare.com"]`)
    if has {
        return ChallengeTurnstile, nil
    }

    // ... more detection logic
}
```

#### 4. CAPTCHA Solver Plugin Interface

```go
type CaptchaSolver interface {
    Name() string
    CanSolve(challenge ChallengeType) bool
    Solve(ctx context.Context, params SolveParams) (*SolveResult, error)
    Cost(challenge ChallengeType) float64
}

type SolveParams struct {
    Type     ChallengeType
    SiteKey  string
    PageURL  string
    Action   string // For Turnstile
    CData    string // For Turnstile
    Proxy    *ProxyConfig
}

type SolveResult struct {
    Token    string
    Valid    time.Duration
    Cost     float64
}

// 2Captcha implementation
type TwoCaptchaSolver struct {
    apiKey string
    client *http.Client
}

func (s *TwoCaptchaSolver) Solve(ctx context.Context, p SolveParams) (*SolveResult, error) {
    // Submit task
    taskID, err := s.submitTask(ctx, p)
    if err != nil {
        return nil, err
    }

    // Poll for result
    return s.pollResult(ctx, taskID)
}
```

#### 5. Authentication Integration

```go
// Middleware validates requests from Refyne API
func ValidateRefyneRequest(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Verify HMAC signature
            signature := r.Header.Get("X-Refyne-Signature")
            timestamp := r.Header.Get("X-Refyne-Timestamp")

            if !verifyHMAC(signature, timestamp, r.Body, secret) {
                http.Error(w, "Unauthorized", 401)
                return
            }

            // Extract user context
            ctx := r.Context()
            ctx = context.WithValue(ctx, "user_id", r.Header.Get("X-User-ID"))
            ctx = context.WithValue(ctx, "tier", r.Header.Get("X-User-Tier"))
            ctx = context.WithValue(ctx, "features", r.Header.Get("X-User-Features"))

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

#### 6. Usage Tracking

```go
// Mirrors refyne-api's usage tracking pattern
type CaptchaUsageRecord struct {
    ID            string    `db:"id"`
    UserID        string    `db:"user_id"`
    RequestID     string    `db:"request_id"`
    TargetDomain  string    `db:"target_domain"`
    ChallengeType string    `db:"challenge_type"`
    SolverUsed    string    `db:"solver_used"`
    Success       bool      `db:"success"`
    DurationMS    int64     `db:"duration_ms"`
    BrowserCostUSD float64  `db:"browser_cost_usd"`
    SolverCostUSD  float64  `db:"solver_cost_usd"`
    ProxyCostUSD   float64  `db:"proxy_cost_usd"`
    CreatedAt     time.Time `db:"created_at"`
}
```

---

## Implementation Plan

### Directory Structure

```
captcha/
├── cmd/
│   └── captcha-server/
│       └── main.go                 # Entry point
├── internal/
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── solve.go            # Main solve endpoint
│   │   │   ├── session.go          # Session management
│   │   │   └── health.go           # Health checks
│   │   ├── middleware/
│   │   │   ├── auth.go             # Refyne auth validation
│   │   │   └── ratelimit.go        # Per-user rate limiting
│   │   └── routes.go               # Route definitions
│   ├── browser/
│   │   ├── pool.go                 # Browser instance pool
│   │   ├── stealth.go              # Stealth patches
│   │   ├── fingerprint.go          # Fingerprint generation
│   │   └── session.go              # Browser session management
│   ├── challenge/
│   │   ├── detector.go             # Challenge type detection
│   │   ├── cloudflare.go           # Cloudflare JS challenge
│   │   ├── turnstile.go            # Turnstile CAPTCHA
│   │   └── ddosguard.go            # DDoS-Guard handling
│   ├── solver/
│   │   ├── interface.go            # Solver plugin interface
│   │   ├── internal.go             # Internal browser-based solving
│   │   ├── twocaptcha.go           # 2Captcha integration
│   │   ├── capsolver.go            # CapSolver integration
│   │   └── chain.go                # Solver chain/fallback
│   ├── proxy/
│   │   ├── manager.go              # Proxy pool management
│   │   ├── rotation.go             # IP rotation logic
│   │   └── health.go               # Proxy health checking
│   ├── models/
│   │   ├── request.go              # API request types
│   │   ├── response.go             # API response types
│   │   └── session.go              # Session types
│   ├── repository/
│   │   ├── session.go              # Session persistence (SQLite/Turso)
│   │   └── usage.go                # Usage tracking
│   └── config/
│       └── config.go               # Configuration management
├── pkg/
│   └── client/
│       └── client.go               # Go SDK for Refyne API integration
├── Dockerfile                      # Multi-stage build
├── fly.toml                        # Fly.io deployment config
└── README.md
```

### Phased Implementation

#### Phase 1: Core Infrastructure (Week 1-2)
- [ ] Project scaffolding matching refyne-api patterns
- [ ] Browser pool with go-rod/stealth
- [ ] Basic FlareSolverr-compatible API
- [ ] Health checks and metrics
- [ ] Docker container with Chrome

#### Phase 2: Challenge Handling (Week 2-3)
- [ ] Cloudflare JS challenge detection
- [ ] Cloudflare challenge auto-wait
- [ ] Session/cookie persistence
- [ ] Turnstile detection
- [ ] DDoS-Guard detection

#### Phase 3: CAPTCHA Solver Integration (Week 3-4)
- [ ] Solver plugin interface
- [ ] 2Captcha integration
- [ ] CapSolver integration
- [ ] Solver chain with fallback

#### Phase 4: Refyne Integration (Week 4-5)
- [ ] Authentication middleware
- [ ] Usage tracking
- [ ] Tier-based rate limiting
- [ ] Refyne API client updates

#### Phase 5: Scaling & Production (Week 5-6)
- [ ] Fly.io machine configuration
- [ ] Auto-scaling setup
- [ ] Redis/Valkey job queue
- [ ] Proxy pool integration
- [ ] Monitoring and alerting

---

## Cost Analysis

### Infrastructure Costs (Monthly, at Scale)

| Component | Specification | Estimated Cost |
|-----------|---------------|----------------|
| Fly.io Machines | 4GB RAM × 5 instances | $100-200 |
| Browser instances | ~20-25 per machine | (included) |
| Redis/Valkey | Fly.io Redis | $20-50 |
| Turso DB | Usage-based | $10-30 |
| **Infrastructure Total** | | **$130-280/month** |

### Variable Costs (per 1000 requests)

| Component | Cost per 1000 |
|-----------|---------------|
| Browser time (~10s/req) | ~$0.50-1.00 |
| 2Captcha (Turnstile) | $1.45 |
| 2Captcha (hCaptcha) | $2.99 |
| Residential proxies | $0.50-2.00 |
| **Per-request range** | **$0.001-0.01** |

### Comparison

| Solution | Setup | Reliability | CAPTCHA Solving | Scale |
|----------|-------|-------------|-----------------|-------|
| FlareSolverr (self-hosted) | Easy | Medium | Broken | Limited |
| This proposal | Medium | High | Yes (external) | Excellent |
| Commercial (ZenRows, etc) | None | High | Yes | Excellent |

---

## Open Questions

1. **Chrome Binary Strategy**: Use stock Chrome + go-rod/stealth, or maintain patched Chrome builds?

2. **Proxy Strategy**: Integrate third-party proxy pools, or allow BYOP (bring-your-own-proxy)?

3. **Session Storage**: Use Turso (shared with refyne-api) or separate Redis for low-latency?

4. **Pricing Model**: Per-request, per-session, or bundled with refyne-api tiers?

5. **Browser Choice**: Chrome-only, or support Firefox (Camoufox) as alternative?

---

## References

### FlareSolverr
- [FlareSolverr GitHub](https://github.com/FlareSolverr/FlareSolverr)
- [ZenRows - FlareSolverr Complete Guide](https://www.zenrows.com/blog/flaresolverr)
- [ScrapFly - Bypass Cloudflare with FlareSolverr](https://scrapfly.io/blog/posts/how-to-bypass-cloudflare-with-flaresolverr)
- [ScrapeOps - FlareSolverr Guide](https://scrapeops.io/python-web-scraping-playbook/python-flaresolverr/)

### Cloudflare Bypass
- [ZenRows - How to Bypass Cloudflare (9 Methods)](https://www.zenrows.com/blog/bypass-cloudflare)
- [BrightData - Bypass Cloudflare 2026](https://brightdata.com/blog/web-data/bypass-cloudflare)
- [Browserless - How to Bypass Cloudflare](https://www.browserless.io/blog/how-to-bypass-cloudflare-scraping)

### Undetected ChromeDriver
- [undetected-chromedriver GitHub](https://github.com/ultrafunkamsterdam/undetected-chromedriver)
- [Oxylabs - Undetected ChromeDriver Guide](https://oxylabs.io/blog/undetected-chromedriver)
- [ScrapingBee - undetected_chromedriver Tutorial](https://www.scrapingbee.com/blog/undetected-chromedriver-python-tutorial-avoiding-bot-detection/)
- [Rebrowser - Ultimate Guide](https://rebrowser.net/blog/undetected-chromedriver-the-ultimate-guide-to-bypassing-bot-detection)

### Go Browser Automation
- [go-rod/rod GitHub](https://github.com/go-rod/rod)
- [go-rod/stealth GitHub](https://github.com/go-rod/stealth)
- [chromedp GitHub](https://github.com/chromedp/chromedp)
- [ZenRows - chromedp Tutorial](https://www.zenrows.com/blog/chromedp)
- [ZenRows - Golang Cloudflare Bypass](https://www.zenrows.com/blog/golang-cloudflare-bypass)

### Camoufox
- [Camoufox GitHub](https://github.com/daijro/camoufox)
- [Camoufox Documentation](https://camoufox.com/)
- [ZenRows - Web Scraping with Camoufox](https://www.zenrows.com/blog/web-scraping-with-camoufox)
- [ScrapingBee - Camoufox Tutorial](https://www.scrapingbee.com/blog/how-to-scrape-with-camoufox-to-bypass-antibot-technology/)

### CAPTCHA Solving Services
- [2Captcha Official](https://2captcha.com)
- [2Captcha API Documentation](https://2captcha.com/api-docs)
- [2Captcha Turnstile Solver](https://2captcha.com/p/cloudflare-turnstile)
- [2Captcha JavaScript Library](https://github.com/2captcha/2captcha-javascript)
- [CapSolver](https://capsolver.com)

### Cloudflare Workers/WASM Limitations
- [Cloudflare WebAssembly Docs](https://developers.cloudflare.com/workers/runtime-apis/webassembly/)
- [Cloudflare Browser Rendering](https://developers.cloudflare.com/browser-rendering/)
- [Cloudflare Browser Rendering FAQ](https://developers.cloudflare.com/browser-rendering/faq/)
- [Cloudflare Blog - WASI on Workers](https://blog.cloudflare.com/announcing-wasi-on-workers/)

---

*Document created: 2026-01-19*
*Last updated: 2026-01-19*
