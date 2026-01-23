// Package preprocessor provides LLM preprocessing utilities that run after
// content cleaning but before LLM extraction. Preprocessors analyze content
// and generate hints that help the LLM produce better results.
package preprocessor

import "strconv"

// DetectedContentType represents a detected repeated content pattern.
type DetectedContentType struct {
	Name  string // e.g., "products", "articles", "jobs"
	Count int    // number of detected elements
}

// Hints holds contextual hints discovered during preprocessing.
// These hints are added to the LLM prompt to improve extraction quality.
type Hints struct {
	// PageStructure describes detected structural patterns (e.g., "listing page with 15 items")
	PageStructure string

	// RepeatedElements is the count of detected repeated elements (0 = not a listing)
	// Deprecated: Use DetectedTypes instead for multi-type support
	RepeatedElements int

	// SuggestedArrayName is a hint for what to name the array (e.g., "products", "articles")
	// Deprecated: Use DetectedTypes instead for multi-type support
	SuggestedArrayName string

	// DetectedTypes lists all detected content types with counts (for mixed-content pages)
	DetectedTypes []DetectedContentType

	// Custom allows preprocessors to add arbitrary hints
	Custom map[string]string
}

// NewHints creates an empty Hints struct.
func NewHints() *Hints {
	return &Hints{
		Custom: make(map[string]string),
	}
}

// HasFeedback returns true if feedback content (reviews, comments, etc.) was detected.
func (h *Hints) HasFeedback() bool {
	if h == nil {
		return false
	}
	return h.Custom["feedback_detected"] == "true"
}

// GetFeedbackTypes returns the detected feedback types (e.g., "reviews", "comments").
func (h *Hints) GetFeedbackTypes() string {
	if h == nil {
		return ""
	}
	return h.Custom["feedback_types"]
}

// GetDetectedTypeNames returns a list of all detected content type names.
func (h *Hints) GetDetectedTypeNames() []string {
	if h == nil {
		return nil
	}
	names := make([]string, len(h.DetectedTypes))
	for i, dt := range h.DetectedTypes {
		names[i] = dt.Name
	}
	return names
}

// Merge combines hints from another Hints struct, with other taking precedence.
func (h *Hints) Merge(other *Hints) {
	if other == nil {
		return
	}
	if other.PageStructure != "" {
		h.PageStructure = other.PageStructure
	}
	if other.RepeatedElements > 0 {
		h.RepeatedElements = other.RepeatedElements
	}
	if other.SuggestedArrayName != "" {
		h.SuggestedArrayName = other.SuggestedArrayName
	}
	if len(other.DetectedTypes) > 0 {
		h.DetectedTypes = append(h.DetectedTypes, other.DetectedTypes...)
	}
	for k, v := range other.Custom {
		h.Custom[k] = v
	}
}

// ToPromptSection formats hints as a prompt section for the LLM.
// This provides structural hints about detected content patterns.
func (h *Hints) ToPromptSection() string {
	if h == nil || (h.PageStructure == "" && h.RepeatedElements == 0 && len(h.DetectedTypes) == 0) {
		return ""
	}

	var result string
	result = "\n## Detected Content Structure\n"

	// Handle multiple detected content types (preferred)
	if len(h.DetectedTypes) > 0 {
		if len(h.DetectedTypes) == 1 {
			dt := h.DetectedTypes[0]
			result += "This page contains **" + strconv.Itoa(dt.Count) + " " + dt.Name + "** items.\n"
			result += "Use a `" + dt.Name + "[]` array in your schema.\n"
		} else {
			result += "This page contains **multiple content types**:\n"
			for _, dt := range h.DetectedTypes {
				result += "- " + strconv.Itoa(dt.Count) + " " + dt.Name + " → use `" + dt.Name + "[]` array\n"
			}
		}
	} else if h.RepeatedElements > 0 {
		result += "Found " + strconv.Itoa(h.RepeatedElements) + " repeated elements.\n"
		result += "Use an array-based schema to capture all items.\n"
		if h.SuggestedArrayName != "" {
			result += "Suggested array: `" + h.SuggestedArrayName + "[]`\n"
		}
	}

	if h.PageStructure != "" {
		result += "\n" + h.PageStructure + "\n"
	}

	return result
}

// LLMPreProcessor analyzes content and generates hints for LLM prompts.
type LLMPreProcessor interface {
	// Process analyzes content and returns hints.
	// The content parameter is the cleaned HTML/text content.
	Process(content string) (*Hints, error)

	// Name returns the preprocessor identifier.
	Name() string
}

// URLAwarePreProcessor is a preprocessor that can use URL context for detection.
type URLAwarePreProcessor interface {
	LLMPreProcessor

	// ProcessWithURL analyzes content with URL context for additional detection signals.
	ProcessWithURL(content, url string) (*Hints, error)
}

// Chain applies multiple preprocessors in sequence, merging their hints.
type Chain struct {
	preprocessors []LLMPreProcessor
}

// NewChain creates a new preprocessor chain.
func NewChain(preprocessors ...LLMPreProcessor) *Chain {
	return &Chain{
		preprocessors: preprocessors,
	}
}

// Process runs all preprocessors and merges their hints.
func (c *Chain) Process(content string) (*Hints, error) {
	return c.ProcessWithURL(content, "")
}

// ProcessWithURL runs all preprocessors with URL context and merges their hints.
// Preprocessors that implement URLAwarePreProcessor will receive the URL for
// additional detection signals (e.g., URL path-based feedback detection).
func (c *Chain) ProcessWithURL(content, url string) (*Hints, error) {
	result := NewHints()

	for _, p := range c.preprocessors {
		var hints *Hints
		var err error

		// Use URL-aware processing if the preprocessor supports it
		if urlAware, ok := p.(URLAwarePreProcessor); ok && url != "" {
			hints, err = urlAware.ProcessWithURL(content, url)
		} else {
			hints, err = p.Process(content)
		}

		if err != nil {
			// Log but continue - preprocessing errors shouldn't block extraction
			continue
		}
		result.Merge(hints)
	}

	return result, nil
}

// Name returns the chain identifier.
func (c *Chain) Name() string {
	if len(c.preprocessors) == 0 {
		return "chain(empty)"
	}
	names := ""
	for i, p := range c.preprocessors {
		if i > 0 {
			names += "->"
		}
		names += p.Name()
	}
	return "chain(" + names + ")"
}
