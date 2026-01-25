package preprocessor

import (
	"cmp"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// FeedbackSignal represents a detected feedback indicator with its source and confidence.
type FeedbackSignal struct {
	Source string  // "url", "schema_org", "text_pattern", "css_class"
	Type   string  // "reviews", "comments", "testimonials", "ratings", "feedback"
	Score  float64 // 0.0-1.0 confidence score
	Count  int     // Number of detected elements (if applicable)
}

// HintFeedback detects human feedback patterns like reviews, comments, and testimonials.
// When such content is detected, it generates hints suggesting the LLM should extract
// sentiment analysis and persona summary fields alongside the regular content.
//
// Detection methods:
// 1. URL patterns - /reviews/, /feedback/, /testimonials/, /comments/
// 2. Schema.org - JSON-LD with @type: "Review", "AggregateRating"
// 3. Text patterns - Star ratings, headings, verified purchase indicators
// 4. CSS classes - Traditional class-based detection (still works on some sites)
type HintFeedback struct {
	// MinRepeats is the minimum number of feedback elements to trigger the hint.
	// Default: 3
	MinRepeats int

	// MinScore is the minimum aggregate score to trigger feedback hints.
	// Default: 0.3
	MinScore float64
}

// HintFeedbackOption configures the HintFeedback preprocessor.
type HintFeedbackOption func(*HintFeedback)

// WithFeedbackMinRepeats sets the minimum repeat threshold for feedback detection.
func WithFeedbackMinRepeats(n int) HintFeedbackOption {
	return func(h *HintFeedback) {
		h.MinRepeats = n
	}
}

// WithFeedbackMinScore sets the minimum score threshold for feedback detection.
func WithFeedbackMinScore(score float64) HintFeedbackOption {
	return func(h *HintFeedback) {
		h.MinScore = score
	}
}

// NewHintFeedback creates a new HintFeedback preprocessor.
func NewHintFeedback(opts ...HintFeedbackOption) *HintFeedback {
	h := &HintFeedback{
		MinRepeats: 3,
		MinScore:   0.3,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Process analyzes content for human feedback patterns and adds sentiment hints.
// This method processes content without URL context. Use ProcessWithURL for URL-aware detection.
func (h *HintFeedback) Process(content string) (*Hints, error) {
	return h.ProcessWithURL(content, "")
}

// ProcessWithURL analyzes content for human feedback patterns with URL context.
// The URL parameter enables URL path-based detection for additional confidence.
func (h *HintFeedback) ProcessWithURL(content, url string) (*Hints, error) {
	hints := NewHints()
	lower := strings.ToLower(content)

	// Collect signals from all detection methods
	var signals []FeedbackSignal

	// 1. URL pattern detection (if URL provided)
	if url != "" {
		urlSignals := h.detectURLPatterns(url)
		signals = append(signals, urlSignals...)
	}

	// 2. Schema.org JSON-LD detection
	schemaSignals := h.detectSchemaOrg(content)
	signals = append(signals, schemaSignals...)

	// 3. Text pattern detection (star ratings, headings, indicators)
	textSignals := h.detectTextPatterns(lower)
	signals = append(signals, textSignals...)

	// 4. CSS class detection (traditional method)
	cssSignals := h.detectCSSPatterns(lower)
	signals = append(signals, cssSignals...)

	// Aggregate signals and determine if feedback is present
	aggregated := h.aggregateSignals(signals)

	// Determine if we have enough signal to indicate feedback content
	// Two paths to detection:
	// 1. High-confidence URL detection (e.g., /reviews path) - trust it even without element counts
	// 2. Content-based detection with sufficient score AND element counts
	hasHighConfidenceURL := aggregated.urlScore >= 0.5
	hasContentDetection := aggregated.totalScore >= h.MinScore && aggregated.totalCount >= h.MinRepeats

	if hasHighConfidenceURL || hasContentDetection {
		// Collect detected feedback types
		// For URL-based detection, add the URL-detected type even without counts
		for feedbackType, count := range aggregated.typeCounts {
			if count >= h.MinRepeats {
				hints.DetectedTypes = append(hints.DetectedTypes, DetectedContentType{
					Name:  feedbackType,
					Count: count,
				})
			}
		}

		// For high-confidence URL detection without element counts, add the URL type
		if hasHighConfidenceURL && len(hints.DetectedTypes) == 0 {
			for feedbackType, score := range aggregated.typeScores {
				// Only add types that came from URL detection (high individual score)
				if score >= 0.5 {
					hints.DetectedTypes = append(hints.DetectedTypes, DetectedContentType{
						Name:  feedbackType,
						Count: 0, // Unknown count, but high confidence from URL
					})
				}
			}
		}

		// Sort by count descending (types with counts first, then alphabetically)
		slices.SortFunc(hints.DetectedTypes, func(a, b DetectedContentType) int {
			return cmp.Compare(b.Count, a.Count)
		})

		// Add feedback hints if we detected feedback types
		if len(hints.DetectedTypes) > 0 {
			// Build type names for hint output
			typeNames := make([]string, 0, len(hints.DetectedTypes))
			totalFeedback := 0
			for _, ft := range hints.DetectedTypes {
				typeNames = append(typeNames, ft.Name)
				totalFeedback += ft.Count
			}

			// Add sentiment analysis guidance as custom hints
			hints.Custom["feedback_detected"] = "true"
			hints.Custom["feedback_types"] = strings.Join(typeNames, ", ")
			hints.Custom["detection_methods"] = strings.Join(aggregated.methods, ", ")
			hints.Custom["sentiment_guidance"] = "For each feedback item, include a 'sentiment' field with values: positive, neutral, or negative"
			hints.Custom["persona_guidance"] = "Include a 'persona_summary' field briefly describing the reviewer's apparent characteristics (e.g., 'experienced user', 'first-time buyer', 'power user', 'price-conscious shopper')"

			// Set page structure hint
			if len(hints.DetectedTypes) == 1 {
				hints.PageStructure = "Feedback page with " + hints.DetectedTypes[0].Name + " - include sentiment analysis"
				hints.SuggestedArrayName = hints.DetectedTypes[0].Name
				hints.RepeatedElements = hints.DetectedTypes[0].Count
			} else {
				hints.PageStructure = "Mixed feedback page with reviews/comments - include sentiment analysis for each item"
				hints.RepeatedElements = totalFeedback
			}
		}
	}

	return hints, nil
}

// aggregatedSignals holds the combined results from all detection methods.
type aggregatedSignals struct {
	totalScore float64
	urlScore   float64 // Score from URL-based detection only
	totalCount int
	typeCounts map[string]int
	typeScores map[string]float64
	methods    []string
}

// aggregateSignals combines signals from all detection methods.
func (h *HintFeedback) aggregateSignals(signals []FeedbackSignal) aggregatedSignals {
	result := aggregatedSignals{
		typeCounts: make(map[string]int),
		typeScores: make(map[string]float64),
	}

	methodSet := make(map[string]bool)

	for _, s := range signals {
		result.totalScore += s.Score
		result.totalCount += s.Count
		result.typeCounts[s.Type] += s.Count

		// Track URL-based score separately for high-confidence detection
		if s.Source == "url" {
			result.urlScore += s.Score
		}
		result.typeScores[s.Type] += s.Score

		if !methodSet[s.Source] {
			methodSet[s.Source] = true
			result.methods = append(result.methods, s.Source)
		}
	}

	return result
}

// detectURLPatterns checks URL path for feedback-related patterns.
func (h *HintFeedback) detectURLPatterns(url string) []FeedbackSignal {
	var signals []FeedbackSignal
	lower := strings.ToLower(url)

	patterns := map[string]string{
		"/reviews":      "reviews",
		"/review":       "reviews",
		"/feedback":     "feedback",
		"/testimonials": "testimonials",
		"/testimonial":  "testimonials",
		"/comments":     "comments",
		"/comment":      "comments",
		"/ratings":      "ratings",
		"/rating":       "ratings",
		"/opinions":     "feedback",
		"/endorsements": "testimonials",
	}

	for pattern, feedbackType := range patterns {
		if strings.Contains(lower, pattern) {
			signals = append(signals, FeedbackSignal{
				Source: "url",
				Type:   feedbackType,
				Score:  0.8, // High confidence when URL explicitly indicates feedback
				Count:  0,   // Unknown count from URL alone
			})
		}
	}

	return signals
}

// detectSchemaOrg parses JSON-LD scripts for schema.org Review/Rating types.
func (h *HintFeedback) detectSchemaOrg(content string) []FeedbackSignal {
	var signals []FeedbackSignal

	// Extract JSON-LD scripts
	jsonLDPattern := regexp.MustCompile(`(?s)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	matches := jsonLDPattern.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		jsonContent := strings.TrimSpace(match[1])

		// Try to parse as JSON
		var data any
		if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
			continue
		}

		// Check for review/rating types in the JSON
		reviewCount, ratingCount := h.countSchemaOrgTypes(data)

		if reviewCount > 0 {
			signals = append(signals, FeedbackSignal{
				Source: "schema_org",
				Type:   "reviews",
				Score:  0.9, // Very high confidence for schema.org markup
				Count:  reviewCount,
			})
		}

		if ratingCount > 0 {
			signals = append(signals, FeedbackSignal{
				Source: "schema_org",
				Type:   "ratings",
				Score:  0.9,
				Count:  ratingCount,
			})
		}
	}

	return signals
}

// countSchemaOrgTypes recursively searches JSON-LD data for Review/Rating types.
func (h *HintFeedback) countSchemaOrgTypes(data any) (reviewCount, ratingCount int) {
	switch v := data.(type) {
	case map[string]any:
		// Check @type field
		if typeVal, ok := v["@type"]; ok {
			typeStr := ""
			switch t := typeVal.(type) {
			case string:
				typeStr = strings.ToLower(t)
			case []any:
				for _, item := range t {
					if s, ok := item.(string); ok {
						typeStr = strings.ToLower(s)
						break
					}
				}
			}

			if strings.Contains(typeStr, "review") {
				reviewCount++
			}
			if strings.Contains(typeStr, "rating") || strings.Contains(typeStr, "aggregaterating") {
				ratingCount++
			}
		}

		// Check for review array
		if reviews, ok := v["review"]; ok {
			if arr, ok := reviews.([]any); ok {
				reviewCount += len(arr)
			} else {
				reviewCount++ // Single review object
			}
		}

		// Recurse into nested objects
		for _, val := range v {
			r, ra := h.countSchemaOrgTypes(val)
			reviewCount += r
			ratingCount += ra
		}

	case []any:
		// Process array items
		for _, item := range v {
			r, ra := h.countSchemaOrgTypes(item)
			reviewCount += r
			ratingCount += ra
		}
	}

	return reviewCount, ratingCount
}

// detectTextPatterns looks for text-based indicators of feedback content.
func (h *HintFeedback) detectTextPatterns(content string) []FeedbackSignal {
	var signals []FeedbackSignal

	// Star rating patterns (unicode stars, emoji, text)
	starPatterns := []string{
		`[★☆]{3,5}`,                     // Unicode stars (3-5 in a row)
		`(?:⭐){1,5}`,                    // Star emoji
		`\d(?:\.\d)?\s*(?:/\s*5|out of 5|stars?)`, // "4.5/5", "4 out of 5", "4 stars"
		`(?:rating|rated)[:.]?\s*\d`,    // "Rating: 4", "Rated 5"
	}

	starCount := 0
	for _, pattern := range starPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindAllString(content, -1)
		starCount += len(matches)
	}

	if starCount >= h.MinRepeats {
		signals = append(signals, FeedbackSignal{
			Source: "text_pattern",
			Type:   "ratings",
			Score:  0.6,
			Count:  starCount,
		})
	}

	// Review heading patterns
	headingPatterns := []string{
		`(?:customer|user|client|product|buyer)\s*reviews?`,
		`(?:reviews?|testimonials?|feedback)\s*(?:from|by)`,
		`what\s+(?:customers?|users?|people|others?)\s+(?:say|think|are saying)`,
		`(?:top|latest|recent|all)\s+reviews?`,
	}

	headingCount := 0
	for _, pattern := range headingPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if re.MatchString(content) {
			headingCount++
		}
	}

	if headingCount > 0 {
		signals = append(signals, FeedbackSignal{
			Source: "text_pattern",
			Type:   "reviews",
			Score:  0.4 * float64(headingCount), // Score increases with more headings
			Count:  0,
		})
	}

	// Verified purchase/buyer indicators
	verifiedPatterns := []string{
		`verified\s+(?:purchase|buyer|customer|review)`,
		`(?:bought|purchased)\s+this\s+(?:item|product)`,
		`verified\s+owner`,
	}

	verifiedCount := 0
	for _, pattern := range verifiedPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindAllString(content, -1)
		verifiedCount += len(matches)
	}

	if verifiedCount > 0 {
		signals = append(signals, FeedbackSignal{
			Source: "text_pattern",
			Type:   "reviews",
			Score:  0.5,
			Count:  verifiedCount,
		})
	}

	// Comment/reply indicators
	commentPatterns := []string{
		`(?:\d+)\s*(?:comments?|replies|responses)`,
		`leave\s+a\s+(?:comment|review|reply)`,
		`(?:reply|respond)\s+to\s+this`,
	}

	commentCount := 0
	for _, pattern := range commentPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if re.MatchString(content) {
			commentCount++
		}
	}

	if commentCount > 0 {
		signals = append(signals, FeedbackSignal{
			Source: "text_pattern",
			Type:   "comments",
			Score:  0.3 * float64(commentCount),
			Count:  0,
		})
	}

	// Testimonial indicators
	testimonialPatterns := []string{
		`(?:hear|see)\s+(?:what|from)\s+(?:our|satisfied)\s+(?:customers?|clients?)`,
		`(?:success|customer)\s+(?:stories|story)`,
		`(?:they|customers?)\s+(?:love|loved)\s+(?:it|us|our)`,
	}

	testimonialCount := 0
	for _, pattern := range testimonialPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if re.MatchString(content) {
			testimonialCount++
		}
	}

	if testimonialCount > 0 {
		signals = append(signals, FeedbackSignal{
			Source: "text_pattern",
			Type:   "testimonials",
			Score:  0.4 * float64(testimonialCount),
			Count:  0,
		})
	}

	return signals
}

// detectCSSPatterns uses traditional CSS class-based detection.
func (h *HintFeedback) detectCSSPatterns(content string) []FeedbackSignal {
	var signals []FeedbackSignal

	// Detect feedback types and counts using CSS classes
	feedbackTypes := h.detectFeedbackTypes(content)

	for _, ft := range feedbackTypes {
		signals = append(signals, FeedbackSignal{
			Source: "css_class",
			Type:   ft.Name,
			Score:  0.5, // Medium confidence for CSS classes
			Count:  ft.Count,
		})
	}

	return signals
}

// Name returns the preprocessor identifier.
func (h *HintFeedback) Name() string {
	return "hint_feedback"
}

// detectFeedbackTypes scans content for various human feedback patterns.
func (h *HintFeedback) detectFeedbackTypes(content string) []DetectedContentType {
	detectors := []struct {
		detect      func(string) int
		contentType string
	}{
		{h.countReviewPatterns, "reviews"},
		{h.countCommentPatterns, "comments"},
		{h.countTestimonialPatterns, "testimonials"},
		{h.countFeedbackPatterns, "feedback"},
		{h.countRatingPatterns, "ratings"},
	}

	var detected []DetectedContentType

	for _, d := range detectors {
		count := d.detect(content)
		if count >= h.MinRepeats {
			detected = append(detected, DetectedContentType{
				Name:  d.contentType,
				Count: count,
			})
		}
	}

	// Sort by count descending
	slices.SortFunc(detected, func(a, b DetectedContentType) int {
		return cmp.Compare(b.Count, a.Count)
	})

	return detected
}

// countReviewPatterns counts review-related elements.
func (h *HintFeedback) countReviewPatterns(content string) int {
	patterns := []string{
		`class="[^"]*review[^"]*"`,
		`class="[^"]*rating[^"]*"`,
		`class="[^"]*user-review[^"]*"`,
		`class="[^"]*customer-review[^"]*"`,
		`class="[^"]*product-review[^"]*"`,
		`data-review`,
		`itemprop="review"`,
		`itemtype="[^"]*Review"`,
	}
	return h.countPatterns(content, patterns)
}

// countCommentPatterns counts comment-related elements.
func (h *HintFeedback) countCommentPatterns(content string) int {
	patterns := []string{
		`class="[^"]*comment[^"]*"`,
		`class="[^"]*reply[^"]*"`,
		`class="[^"]*response[^"]*"`,
		`class="[^"]*user-comment[^"]*"`,
		`class="[^"]*discussion[^"]*"`,
		`id="[^"]*comment[^"]*"`,
		`data-comment`,
		`itemtype="[^"]*Comment"`,
	}
	return h.countPatterns(content, patterns)
}

// countTestimonialPatterns counts testimonial-related elements.
func (h *HintFeedback) countTestimonialPatterns(content string) int {
	patterns := []string{
		`class="[^"]*testimonial[^"]*"`,
		`class="[^"]*quote[^"]*"`,
		`class="[^"]*customer-quote[^"]*"`,
		`class="[^"]*client-feedback[^"]*"`,
		`class="[^"]*success-story[^"]*"`,
		`class="[^"]*endorsement[^"]*"`,
		`class="[^"]*recommendation[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countFeedbackPatterns counts general feedback elements.
func (h *HintFeedback) countFeedbackPatterns(content string) int {
	patterns := []string{
		`class="[^"]*feedback[^"]*"`,
		`class="[^"]*opinion[^"]*"`,
		`class="[^"]*user-feedback[^"]*"`,
		`class="[^"]*customer-feedback[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countRatingPatterns counts rating-related elements (stars, scores, etc).
func (h *HintFeedback) countRatingPatterns(content string) int {
	patterns := []string{
		`class="[^"]*star[^"]*"`,
		`class="[^"]*stars[^"]*"`,
		`class="[^"]*rating[^"]*"`,
		`class="[^"]*score[^"]*"`,
		`aria-label="[^"]*rating[^"]*"`,
		`aria-label="[^"]*star[^"]*"`,
		`data-rating`,
		`data-score`,
	}

	// For ratings, we look for distinct rating containers, not individual stars
	// Count unique rating containers by looking for rating wrapper patterns
	wrapperPatterns := []string{
		`class="[^"]*rating-wrapper[^"]*"`,
		`class="[^"]*rating-container[^"]*"`,
		`class="[^"]*stars-wrapper[^"]*"`,
		`class="[^"]*review-rating[^"]*"`,
	}

	wrapperCount := h.countPatterns(content, wrapperPatterns)
	if wrapperCount >= h.MinRepeats {
		return wrapperCount
	}

	// Fall back to counting rating classes, but divide by 5 (typical star count)
	ratingCount := h.countPatterns(content, patterns)
	if ratingCount >= 5 {
		// Assume 5 stars per rating, so divide to get approximate review count
		estimated := ratingCount / 5
		if estimated >= h.MinRepeats {
			return estimated
		}
	}

	return 0
}

// countPatterns counts occurrences of any of the given regex patterns.
func (h *HintFeedback) countPatterns(content string, patterns []string) int {
	maxCount := 0
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		matches := re.FindAllString(content, -1)
		if len(matches) > maxCount {
			maxCount = len(matches)
		}
	}
	return maxCount
}
