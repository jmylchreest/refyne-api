package preprocessor

import (
	"regexp"
	"strings"
)

// HintRepeats detects repeated HTML patterns that suggest listing pages.
// It analyzes the HTML structure to find repeated elements like product cards,
// list items, or article entries, and generates hints for the LLM to use
// array-based schemas.
type HintRepeats struct {
	// MinRepeats is the minimum number of repeated elements to consider it a listing.
	// Default: 3
	MinRepeats int
}

// HintRepeatsOption configures the HintRepeats preprocessor.
type HintRepeatsOption func(*HintRepeats)

// WithMinRepeats sets the minimum repeat threshold.
func WithMinRepeats(n int) HintRepeatsOption {
	return func(h *HintRepeats) {
		h.MinRepeats = n
	}
}

// NewHintRepeats creates a new HintRepeats preprocessor.
func NewHintRepeats(opts ...HintRepeatsOption) *HintRepeats {
	h := &HintRepeats{
		MinRepeats: 3,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Process analyzes content for repeated patterns.
func (h *HintRepeats) Process(content string) (*Hints, error) {
	hints := NewHints()

	// Detect all repeated element types that meet the threshold
	detectedTypes := h.detectAllRepeatedElements(content)

	if len(detectedTypes) > 0 {
		hints.DetectedTypes = detectedTypes

		// Also set legacy fields for backward compatibility
		if len(detectedTypes) == 1 {
			hints.RepeatedElements = detectedTypes[0].Count
			hints.SuggestedArrayName = detectedTypes[0].Name
			hints.PageStructure = "Listing page with repeated " + detectedTypes[0].Name + " elements"
		} else {
			// Sum total for legacy field
			total := 0
			for _, dt := range detectedTypes {
				total += dt.Count
			}
			hints.RepeatedElements = total
			hints.PageStructure = "Mixed content page with multiple content types"
		}
	}

	return hints, nil
}

// Name returns the preprocessor identifier.
func (h *HintRepeats) Name() string {
	return "hint_repeats"
}

// detectAllRepeatedElements scans HTML for all repeated structural patterns.
// Returns all content types that meet the minimum threshold, sorted by count descending.
func (h *HintRepeats) detectAllRepeatedElements(content string) []DetectedContentType {
	lower := strings.ToLower(content)

	// Pattern detectors with their associated content types
	detectors := []struct {
		detect      func(string) int
		contentType string
	}{
		{h.countProductPatterns, "products"},
		{h.countArticlePatterns, "articles"},
		{h.countJobPatterns, "jobs"},
		{h.countRecipePatterns, "recipes"},
		{h.countCaseStudyPatterns, "case_studies"},
		{h.countEventPatterns, "events"},
		{h.countServicePatterns, "services"},
		{h.countTeamPatterns, "team_members"},
		{h.countPodcastPatterns, "episodes"},
	}

	var detected []DetectedContentType

	for _, d := range detectors {
		count := d.detect(lower)
		if count >= h.MinRepeats {
			detected = append(detected, DetectedContentType{
				Name:  d.contentType,
				Count: count,
			})
		}
	}

	// If no specific types detected, try generic patterns
	if len(detected) == 0 {
		genericDetectors := []struct {
			detect      func(string) int
			contentType string
		}{
			{h.countCards, "items"},
			{h.countListItems, "items"},
			{h.countTableRows, "items"},
		}

		maxCount := 0
		for _, d := range genericDetectors {
			count := d.detect(lower)
			if count > maxCount {
				maxCount = count
			}
		}

		if maxCount >= h.MinRepeats {
			detected = append(detected, DetectedContentType{
				Name:  "items",
				Count: maxCount,
			})
		}
	}

	// Sort by count descending
	for i := 0; i < len(detected)-1; i++ {
		for j := i + 1; j < len(detected); j++ {
			if detected[j].Count > detected[i].Count {
				detected[i], detected[j] = detected[j], detected[i]
			}
		}
	}

	return detected
}

// countProductPatterns counts product-related repeated elements.
// Uses specific product-related patterns to avoid false positives from generic "item" classes.
func (h *HintRepeats) countProductPatterns(content string) int {
	patterns := []string{
		`class="[^"]*product[^"]*"`,
		`class="[^"]*product-card[^"]*"`,
		`class="[^"]*product-item[^"]*"`,
		`class="[^"]*shop-item[^"]*"`,
		`class="[^"]*store-item[^"]*"`,
		`data-product`,
		`itemprop="product"`,
		`itemtype="[^"]*Product"`,
	}
	return h.countPatterns(content, patterns)
}

// countArticlePatterns counts article-related repeated elements.
func (h *HintRepeats) countArticlePatterns(content string) int {
	patterns := []string{
		`<article`,
		`class="[^"]*article[^"]*"`,
		`class="[^"]*post[^"]*"`,
		`class="[^"]*blog[^"]*"`,
		`class="[^"]*news[^"]*"`,
		`class="[^"]*story[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countJobPatterns counts job-related repeated elements.
func (h *HintRepeats) countJobPatterns(content string) int {
	patterns := []string{
		`class="[^"]*job[^"]*"`,
		`class="[^"]*career[^"]*"`,
		`class="[^"]*position[^"]*"`,
		`class="[^"]*vacancy[^"]*"`,
		`class="[^"]*opening[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countRecipePatterns counts recipe-related repeated elements.
func (h *HintRepeats) countRecipePatterns(content string) int {
	patterns := []string{
		`class="[^"]*recipe[^"]*"`,
		`class="[^"]*recipe-card[^"]*"`,
		`class="[^"]*meal[^"]*"`,
		`class="[^"]*dish[^"]*"`,
		`class="[^"]*food-item[^"]*"`,
		`class="[^"]*cooking[^"]*"`,
		`itemprop="recipe"`,
		`itemtype="[^"]*Recipe"`,
	}
	return h.countPatterns(content, patterns)
}

// countCaseStudyPatterns counts case study related elements.
func (h *HintRepeats) countCaseStudyPatterns(content string) int {
	patterns := []string{
		`class="[^"]*case-study[^"]*"`,
		`class="[^"]*casestudy[^"]*"`,
		`class="[^"]*case_study[^"]*"`,
		`class="[^"]*success-stor[^"]*"`,
		`class="[^"]*testimonial[^"]*"`,
		`class="[^"]*customer-stor[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countEventPatterns counts event-related repeated elements.
func (h *HintRepeats) countEventPatterns(content string) int {
	patterns := []string{
		`class="[^"]*event[^"]*"`,
		`class="[^"]*webinar[^"]*"`,
		`class="[^"]*conference[^"]*"`,
		`class="[^"]*meetup[^"]*"`,
		`class="[^"]*workshop[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countServicePatterns counts service-related repeated elements.
func (h *HintRepeats) countServicePatterns(content string) int {
	patterns := []string{
		`class="[^"]*service[^"]*"`,
		`class="[^"]*solution[^"]*"`,
		`class="[^"]*offering[^"]*"`,
		`class="[^"]*capability[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countTeamPatterns counts team/people related elements.
func (h *HintRepeats) countTeamPatterns(content string) int {
	patterns := []string{
		`class="[^"]*team[^"]*"`,
		`class="[^"]*member[^"]*"`,
		`class="[^"]*staff[^"]*"`,
		`class="[^"]*employee[^"]*"`,
		`class="[^"]*people[^"]*"`,
		`class="[^"]*author[^"]*"`,
		`class="[^"]*profile[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countPodcastPatterns counts podcast/episode related elements.
func (h *HintRepeats) countPodcastPatterns(content string) int {
	patterns := []string{
		`class="[^"]*episode[^"]*"`,
		`class="[^"]*podcast[^"]*"`,
		`class="[^"]*audio[^"]*"`,
		`class="[^"]*show[^"]*"`,
		`class="[^"]*listen[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countListItems counts list item elements.
func (h *HintRepeats) countListItems(content string) int {
	// Count <li> tags, but only if there are many (suggesting a content list, not navigation)
	liCount := strings.Count(content, "<li")
	// Require more li elements to distinguish from nav menus
	if liCount >= 5 {
		return liCount
	}
	return 0
}

// countCards counts card-like elements.
func (h *HintRepeats) countCards(content string) int {
	patterns := []string{
		`class="[^"]*card[^"]*"`,
		`class="[^"]*tile[^"]*"`,
		`class="[^"]*grid-item[^"]*"`,
		`class="[^"]*cell[^"]*"`,
	}
	return h.countPatterns(content, patterns)
}

// countTableRows counts table rows (for tabular listings).
func (h *HintRepeats) countTableRows(content string) int {
	// Count <tr> tags in tbody (data rows, not headers)
	tbodyStart := strings.Index(content, "<tbody")
	if tbodyStart == -1 {
		return 0
	}
	tbodyEnd := strings.Index(content[tbodyStart:], "</tbody")
	if tbodyEnd == -1 {
		return 0
	}
	tbody := content[tbodyStart : tbodyStart+tbodyEnd]
	return strings.Count(tbody, "<tr")
}

// countPatterns counts occurrences of any of the given regex patterns.
func (h *HintRepeats) countPatterns(content string, patterns []string) int {
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
