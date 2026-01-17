// Package preprocessor provides LLM preprocessing utilities that run after
// content cleaning but before LLM extraction. Preprocessors analyze content
// and generate hints that help the LLM produce better results.
package preprocessor

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
func (h *Hints) ToPromptSection() string {
	if h == nil || (h.PageStructure == "" && h.RepeatedElements == 0 && len(h.DetectedTypes) == 0 && len(h.Custom) == 0) {
		return ""
	}

	var result string
	result = "\n## Detected Page Structure\n"

	// Handle multiple detected content types (preferred)
	if len(h.DetectedTypes) > 0 {
		if len(h.DetectedTypes) == 1 {
			// Single content type - simple listing page
			dt := h.DetectedTypes[0]
			result += "- Found " + itoa(dt.Count) + " repeated " + dt.Name + " elements\n"
			result += "- This is a listing page - use " + dt.Name + "[] array\n"
		} else {
			// Multiple content types - mixed content page
			result += "- This is a MIXED CONTENT page with multiple content types:\n"
			for _, dt := range h.DetectedTypes {
				result += "  - " + itoa(dt.Count) + " " + dt.Name + "\n"
			}
			result += "- Use items[] with a content_type field to distinguish between types\n"
			result += "- Add a metadata object for type-specific fields\n"
		}
	} else if h.RepeatedElements > 0 {
		// Fallback to legacy single-type detection
		result += "- Found " + itoa(h.RepeatedElements) + " repeated elements (this is a listing page)\n"
		result += "- Use an array-based schema to capture all items\n"
		if h.SuggestedArrayName != "" {
			result += "- Suggested array name: " + h.SuggestedArrayName + "[]\n"
		}
	}

	if h.PageStructure != "" {
		result += "- " + h.PageStructure + "\n"
	}

	for k, v := range h.Custom {
		result += "- " + k + ": " + v + "\n"
	}

	return result
}

// itoa is a simple int to string conversion to avoid importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// LLMPreProcessor analyzes content and generates hints for LLM prompts.
type LLMPreProcessor interface {
	// Process analyzes content and returns hints.
	// The content parameter is the cleaned HTML/text content.
	Process(content string) (*Hints, error)

	// Name returns the preprocessor identifier.
	Name() string
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
	result := NewHints()

	for _, p := range c.preprocessors {
		hints, err := p.Process(content)
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
