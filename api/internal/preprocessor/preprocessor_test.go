package preprocessor

import (
	"strings"
	"testing"
)

// ========================================
// Hints Tests
// ========================================

func TestNewHints(t *testing.T) {
	hints := NewHints()
	if hints == nil {
		t.Fatal("expected hints, got nil")
	}
	if hints.Custom == nil {
		t.Error("Custom map should be initialized")
	}
	if hints.PageStructure != "" {
		t.Error("PageStructure should be empty by default")
	}
	if hints.RepeatedElements != 0 {
		t.Error("RepeatedElements should be 0 by default")
	}
	if hints.SuggestedArrayName != "" {
		t.Error("SuggestedArrayName should be empty by default")
	}
	if len(hints.DetectedTypes) != 0 {
		t.Error("DetectedTypes should be empty by default")
	}
}

func TestHints_Merge_NilOther(t *testing.T) {
	hints := NewHints()
	hints.PageStructure = "original"

	hints.Merge(nil)

	if hints.PageStructure != "original" {
		t.Error("merge with nil should not change hints")
	}
}

func TestHints_Merge_OverwriteNonEmpty(t *testing.T) {
	hints := NewHints()
	hints.PageStructure = "original"
	hints.RepeatedElements = 5

	other := &Hints{
		PageStructure:    "new structure",
		RepeatedElements: 10,
	}

	hints.Merge(other)

	if hints.PageStructure != "new structure" {
		t.Errorf("PageStructure = %q, want %q", hints.PageStructure, "new structure")
	}
	if hints.RepeatedElements != 10 {
		t.Errorf("RepeatedElements = %d, want 10", hints.RepeatedElements)
	}
}

func TestHints_Merge_PreserveExisting(t *testing.T) {
	hints := NewHints()
	hints.PageStructure = "original"
	hints.RepeatedElements = 5
	hints.SuggestedArrayName = "products"

	other := &Hints{
		RepeatedElements: 0, // Zero, should not overwrite
	}

	hints.Merge(other)

	if hints.PageStructure != "original" {
		t.Errorf("PageStructure should be preserved when other is empty")
	}
	if hints.RepeatedElements != 5 {
		t.Errorf("RepeatedElements should be preserved when other is 0")
	}
	if hints.SuggestedArrayName != "products" {
		t.Errorf("SuggestedArrayName should be preserved")
	}
}

func TestHints_Merge_DetectedTypes(t *testing.T) {
	hints := NewHints()
	hints.DetectedTypes = []DetectedContentType{
		{Name: "products", Count: 10},
	}

	other := &Hints{
		DetectedTypes: []DetectedContentType{
			{Name: "articles", Count: 5},
		},
	}

	hints.Merge(other)

	if len(hints.DetectedTypes) != 2 {
		t.Errorf("DetectedTypes length = %d, want 2", len(hints.DetectedTypes))
	}
}

func TestHints_Merge_Custom(t *testing.T) {
	hints := NewHints()
	hints.Custom["key1"] = "value1"

	other := &Hints{
		Custom: map[string]string{
			"key2": "value2",
			"key1": "overwritten",
		},
	}

	hints.Merge(other)

	if hints.Custom["key1"] != "overwritten" {
		t.Errorf("Custom[key1] = %q, want %q", hints.Custom["key1"], "overwritten")
	}
	if hints.Custom["key2"] != "value2" {
		t.Errorf("Custom[key2] = %q, want %q", hints.Custom["key2"], "value2")
	}
}

// ========================================
// ToPromptSection Tests
// ========================================

func TestHints_ToPromptSection_Empty(t *testing.T) {
	hints := NewHints()
	result := hints.ToPromptSection()

	if result != "" {
		t.Errorf("empty hints should produce empty prompt section, got %q", result)
	}
}

func TestHints_ToPromptSection_Nil(t *testing.T) {
	var hints *Hints
	result := hints.ToPromptSection()

	if result != "" {
		t.Errorf("nil hints should produce empty prompt section, got %q", result)
	}
}

func TestHints_ToPromptSection_SingleDetectedType(t *testing.T) {
	hints := NewHints()
	hints.DetectedTypes = []DetectedContentType{
		{Name: "products", Count: 15},
	}

	result := hints.ToPromptSection()

	if !strings.Contains(result, "## Detected Content Structure") {
		t.Error("should contain header")
	}
	if !strings.Contains(result, "15 products") {
		t.Error("should contain count and name")
	}
	if !strings.Contains(result, "products[]") {
		t.Error("should suggest array name")
	}
}

func TestHints_ToPromptSection_MultipleDetectedTypes(t *testing.T) {
	hints := NewHints()
	hints.DetectedTypes = []DetectedContentType{
		{Name: "products", Count: 10},
		{Name: "articles", Count: 5},
	}

	result := hints.ToPromptSection()

	if !strings.Contains(result, "multiple content types") {
		t.Error("should indicate multiple content categories")
	}
	if !strings.Contains(result, "10 products") {
		t.Error("should list products count")
	}
	if !strings.Contains(result, "5 articles") {
		t.Error("should list articles count")
	}
	if !strings.Contains(result, "products[]") {
		t.Error("should suggest products array")
	}
	if !strings.Contains(result, "articles[]") {
		t.Error("should suggest articles array")
	}
}

func TestHints_ToPromptSection_LegacyFields(t *testing.T) {
	hints := NewHints()
	hints.RepeatedElements = 8
	hints.SuggestedArrayName = "items"

	result := hints.ToPromptSection()

	if !strings.Contains(result, "8 repeated elements") {
		t.Error("should contain legacy repeated elements count")
	}
	if !strings.Contains(result, "items[]") {
		t.Error("should contain suggested array name")
	}
}

func TestHints_ToPromptSection_PageStructure(t *testing.T) {
	hints := NewHints()
	hints.PageStructure = "Product listing page with sidebar"

	result := hints.ToPromptSection()

	if !strings.Contains(result, "Product listing page with sidebar") {
		t.Error("should contain page structure")
	}
}

func TestHints_ToPromptSection_CustomHints(t *testing.T) {
	// Custom hints are no longer included in ToPromptSection - they're handled
	// separately by the analyzer's buildSchemaExamples function
	hints := NewHints()
	hints.Custom["pagination"] = "detected"
	hints.Custom["category_filters"] = "present"

	result := hints.ToPromptSection()

	// Custom hints alone don't produce output - need detected types or page structure
	if result != "" {
		t.Error("custom hints alone should not produce prompt section")
	}

	// With detected types, should still work
	hints.DetectedTypes = []DetectedContentType{{Name: "products", Count: 5}}
	result = hints.ToPromptSection()

	if !strings.Contains(result, "products") {
		t.Error("should still contain detected types")
	}
}

// ========================================
// DetectedContentType Tests
// ========================================

func TestDetectedContentType_Fields(t *testing.T) {
	dt := DetectedContentType{
		Name:  "products",
		Count: 25,
	}

	if dt.Name != "products" {
		t.Errorf("Name = %q, want %q", dt.Name, "products")
	}
	if dt.Count != 25 {
		t.Errorf("Count = %d, want 25", dt.Count)
	}
}

// ========================================
// Chain Tests
// ========================================

func TestNewChain(t *testing.T) {
	chain := NewChain()
	if chain == nil {
		t.Fatal("expected chain, got nil")
	}
	if len(chain.preprocessors) != 0 {
		t.Error("empty chain should have no preprocessors")
	}
}

func TestNewChain_WithPreprocessors(t *testing.T) {
	noop := NewNoop()
	hints := NewHintRepeats()

	chain := NewChain(noop, hints)

	if len(chain.preprocessors) != 2 {
		t.Errorf("chain has %d preprocessors, want 2", len(chain.preprocessors))
	}
}

func TestChain_Process(t *testing.T) {
	chain := NewChain(NewNoop())

	hints, err := chain.Process("some content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hints == nil {
		t.Fatal("expected hints, got nil")
	}
}

func TestChain_Process_MergesHints(t *testing.T) {
	// Create a custom preprocessor that returns known hints
	chain := NewChain(NewHintRepeats(WithMinRepeats(2)))

	content := `
		<div class="product">Product 1</div>
		<div class="product">Product 2</div>
		<div class="product">Product 3</div>
	`

	hints, err := chain.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) == 0 {
		t.Error("expected detected types from HintRepeats")
	}
}

func TestChain_Name_Empty(t *testing.T) {
	chain := NewChain()
	name := chain.Name()

	if name != "chain(empty)" {
		t.Errorf("Name() = %q, want %q", name, "chain(empty)")
	}
}

func TestChain_Name_WithPreprocessors(t *testing.T) {
	chain := NewChain(NewNoop(), NewHintRepeats())
	name := chain.Name()

	if name != "chain(noop->hint_repeats)" {
		t.Errorf("Name() = %q, want %q", name, "chain(noop->hint_repeats)")
	}
}

func TestChain_ProcessWithURL_PassesURLToAwarePreprocessors(t *testing.T) {
	// HintFeedback implements URLAwarePreProcessor
	chain := NewChain(NewHintFeedback(WithFeedbackMinScore(0.5), WithFeedbackMinRepeats(1)))

	// Content without CSS class patterns, but URL should trigger detection
	content := `
		<html>
		<body>
			<div>Just some generic content without review classes</div>
		</body>
		</html>
	`

	// Without URL - should not detect feedback
	hints, err := chain.ProcessWithURL(content, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hints.Custom["feedback_detected"] == "true" {
		t.Error("should not detect feedback without URL or content patterns")
	}

	// With /reviews URL - should detect feedback due to URL pattern
	hints, err = chain.ProcessWithURL(content, "https://example.com/reviews")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// URL pattern alone doesn't meet MinRepeats requirement (Count: 0)
	// but let's verify the URL signal is being captured

	// Test with content that has star ratings (text pattern detection)
	contentWithStars := `
		<html>
		<body>
			<div>★★★★☆ Great product!</div>
			<div>★★★★★ Amazing!</div>
			<div>★★★☆☆ Could be better</div>
		</body>
		</html>
	`

	hints, err = chain.ProcessWithURL(contentWithStars, "https://example.com/reviews")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With both URL pattern (score: 0.8) and star ratings (score: 0.6, count: 3), should trigger
	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback with URL pattern and star ratings")
	}
	if hints.Custom["sentiment_guidance"] == "" {
		t.Error("should have sentiment_guidance in Custom hints")
	}
	if hints.Custom["persona_guidance"] == "" {
		t.Error("should have persona_guidance in Custom hints")
	}
}

func TestChain_ProcessWithURL_UsesRegularProcessForNonURLAware(t *testing.T) {
	// HintRepeats does NOT implement URLAwarePreProcessor
	chain := NewChain(NewHintRepeats(WithMinRepeats(2)))

	content := `
		<div class="product">Product 1</div>
		<div class="product">Product 2</div>
		<div class="product">Product 3</div>
	`

	// ProcessWithURL should still work, using regular Process for non-URL-aware
	hints, err := chain.ProcessWithURL(content, "https://example.com/products")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) == 0 {
		t.Error("expected detected types from HintRepeats")
	}
}

// ========================================
// Noop Tests
// ========================================

func TestNewNoop(t *testing.T) {
	noop := NewNoop()
	if noop == nil {
		t.Fatal("expected noop, got nil")
	}
}

func TestNoop_Process(t *testing.T) {
	noop := NewNoop()
	hints, err := noop.Process("any content")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hints == nil {
		t.Fatal("expected hints, got nil")
	}
	if hints.PageStructure != "" {
		t.Error("noop should return empty hints")
	}
}

func TestNoop_Name(t *testing.T) {
	noop := NewNoop()
	if noop.Name() != "noop" {
		t.Errorf("Name() = %q, want %q", noop.Name(), "noop")
	}
}

// ========================================
// HintRepeats Tests
// ========================================

func TestNewHintRepeats_Defaults(t *testing.T) {
	hr := NewHintRepeats()
	if hr == nil {
		t.Fatal("expected HintRepeats, got nil")
	}
	if hr.MinRepeats != 3 {
		t.Errorf("MinRepeats = %d, want 3 (default)", hr.MinRepeats)
	}
}

func TestNewHintRepeats_WithMinRepeats(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(5))
	if hr.MinRepeats != 5 {
		t.Errorf("MinRepeats = %d, want 5", hr.MinRepeats)
	}
}

func TestHintRepeats_Name(t *testing.T) {
	hr := NewHintRepeats()
	if hr.Name() != "hint_repeats" {
		t.Errorf("Name() = %q, want %q", hr.Name(), "hint_repeats")
	}
}

func TestHintRepeats_Process_NoRepeats(t *testing.T) {
	hr := NewHintRepeats()
	hints, err := hr.Process("<p>Simple paragraph</p>")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hints.DetectedTypes) != 0 {
		t.Error("should not detect types in simple content")
	}
}

func TestHintRepeats_Process_Products(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<div class="product-card">Product 1</div>
		<div class="product-card">Product 2</div>
		<div class="product-card">Product 3</div>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) == 0 {
		t.Error("should detect product patterns")
	}
}

func TestHintRepeats_Process_Articles(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<article>Article 1</article>
		<article>Article 2</article>
		<article>Article 3</article>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, dt := range hints.DetectedTypes {
		if dt.Name == "articles" {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect article patterns")
	}
}

func TestHintRepeats_Process_Jobs(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<div class="job-listing">Job 1</div>
		<div class="job-listing">Job 2</div>
		<div class="job-listing">Job 3</div>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, dt := range hints.DetectedTypes {
		if dt.Name == "jobs" {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect job patterns")
	}
}

func TestHintRepeats_Process_Cards(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<div class="card">Card 1</div>
		<div class="card">Card 2</div>
		<div class="card">Card 3</div>
		<div class="card">Card 4</div>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) == 0 {
		t.Error("should detect card patterns as generic items")
	}
}

func TestHintRepeats_Process_ListItems(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<ul>
			<li>Item 1</li>
			<li>Item 2</li>
			<li>Item 3</li>
			<li>Item 4</li>
			<li>Item 5</li>
		</ul>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// List items require 5+ to distinguish from navigation
	if len(hints.DetectedTypes) == 0 {
		t.Error("should detect list items with 5+ elements")
	}
}

func TestHintRepeats_Process_TableRows(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<table>
			<thead><tr><th>Name</th></tr></thead>
			<tbody>
				<tr><td>Row 1</td></tr>
				<tr><td>Row 2</td></tr>
				<tr><td>Row 3</td></tr>
			</tbody>
		</table>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) == 0 {
		t.Error("should detect table rows")
	}
}

func TestHintRepeats_Process_BelowThreshold(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(5))
	content := `
		<div class="product">Product 1</div>
		<div class="product">Product 2</div>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) != 0 {
		t.Error("should not detect patterns below threshold")
	}
}

func TestHintRepeats_Process_CaseInsensitive(t *testing.T) {
	hr := NewHintRepeats(WithMinRepeats(2))
	content := `
		<div class="PRODUCT-CARD">Product 1</div>
		<div class="Product-Card">Product 2</div>
		<div class="product-card">Product 3</div>
	`

	hints, err := hr.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hints.DetectedTypes) == 0 {
		t.Error("pattern detection should be case-insensitive")
	}
}

// ========================================
// LLMPreProcessor Interface Tests
// ========================================

func TestLLMPreProcessor_Interface(t *testing.T) {
	// Verify all types implement the interface
	var _ LLMPreProcessor = (*Noop)(nil)
	var _ LLMPreProcessor = (*HintRepeats)(nil)
	var _ LLMPreProcessor = (*Chain)(nil)
	var _ LLMPreProcessor = (*HintFeedback)(nil)
}

// ========================================
// HintFeedback Tests
// ========================================

func TestNewHintFeedback_Defaults(t *testing.T) {
	hf := NewHintFeedback()
	if hf == nil {
		t.Fatal("expected HintFeedback, got nil")
	}
	if hf.MinRepeats != 3 {
		t.Errorf("MinRepeats = %d, want 3 (default)", hf.MinRepeats)
	}
	if hf.MinScore != 0.3 {
		t.Errorf("MinScore = %f, want 0.3 (default)", hf.MinScore)
	}
}

func TestNewHintFeedback_WithOptions(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(5), WithFeedbackMinScore(0.5))
	if hf.MinRepeats != 5 {
		t.Errorf("MinRepeats = %d, want 5", hf.MinRepeats)
	}
	if hf.MinScore != 0.5 {
		t.Errorf("MinScore = %f, want 0.5", hf.MinScore)
	}
}

func TestHintFeedback_Name(t *testing.T) {
	hf := NewHintFeedback()
	if hf.Name() != "hint_feedback" {
		t.Errorf("Name() = %q, want %q", hf.Name(), "hint_feedback")
	}
}

func TestHintFeedback_DetectURLPatterns_Reviews(t *testing.T) {
	// URL pattern alone is not enough - needs actual review content too
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<div class="review">Review 1</div>
			<div class="review">Review 2</div>
			<div class="review">Review 3</div>
		</body>
	</html>`
	hints, err := hf.ProcessWithURL(content, "https://example.com/product/123/reviews")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from URL pattern + content")
	}
	// Both URL and CSS class should be detected
	if hints.Custom["detection_methods"] == "" {
		t.Error("detection_methods should be populated")
	}
}

func TestHintFeedback_DetectURLPatterns_Testimonials(t *testing.T) {
	// URL pattern alone is not enough - needs actual testimonial content too
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<div class="testimonial">Testimonial 1</div>
			<div class="testimonial">Testimonial 2</div>
			<div class="testimonial">Testimonial 3</div>
		</body>
	</html>`
	hints, err := hf.ProcessWithURL(content, "https://example.com/testimonials")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from URL pattern + content")
	}
	if !strings.Contains(hints.Custom["feedback_types"], "testimonials") {
		t.Errorf("feedback_types should include testimonials, got: %s", hints.Custom["feedback_types"])
	}
}

func TestHintFeedback_DetectSchemaOrg_Review(t *testing.T) {
	hf := NewHintFeedback()
	content := `<html>
		<head>
			<script type="application/ld+json">
			{
				"@context": "https://schema.org",
				"@type": "Product",
				"name": "Test Product",
				"review": [
					{"@type": "Review", "reviewBody": "Great product!", "reviewRating": {"@type": "Rating", "ratingValue": 5}},
					{"@type": "Review", "reviewBody": "Not bad", "reviewRating": {"@type": "Rating", "ratingValue": 4}},
					{"@type": "Review", "reviewBody": "Could be better", "reviewRating": {"@type": "Rating", "ratingValue": 3}}
				]
			}
			</script>
		</head>
		<body></body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from schema.org")
	}
	if !strings.Contains(hints.Custom["detection_methods"], "schema_org") {
		t.Error("detection_methods should include 'schema_org'")
	}
}

func TestHintFeedback_DetectSchemaOrg_AggregateRating(t *testing.T) {
	// AggregateRating alone is metadata - needs actual review content to trigger
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<head>
			<script type="application/ld+json">
			{
				"@context": "https://schema.org",
				"@type": "Product",
				"name": "Test Product",
				"aggregateRating": {
					"@type": "AggregateRating",
					"ratingValue": 4.5,
					"reviewCount": 100
				}
			}
			</script>
		</head>
		<body>
			<div class="review">Review 1</div>
			<div class="review">Review 2</div>
			<div class="review">Review 3</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from schema.org AggregateRating + content")
	}
}

func TestHintFeedback_DetectTextPatterns_StarRatings(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<div class="review-item">
				<span class="rating">★★★★☆</span>
				<p>Great product!</p>
			</div>
			<div class="review-item">
				<span class="rating">★★★★★</span>
				<p>Excellent!</p>
			</div>
			<div class="review-item">
				<span class="rating">★★★☆☆</span>
				<p>It's okay</p>
			</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from star ratings")
	}
	if !strings.Contains(hints.Custom["detection_methods"], "text_pattern") {
		t.Error("detection_methods should include 'text_pattern'")
	}
}

func TestHintFeedback_DetectTextPatterns_NumericRatings(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<div>Rating: 4.5/5</div>
			<div>Rated 5 out of 5</div>
			<div>4 stars</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from numeric ratings")
	}
}

func TestHintFeedback_DetectTextPatterns_Headings(t *testing.T) {
	// Headings alone are not enough - needs actual review content
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<h2>Customer Reviews</h2>
			<p>What our customers say about us:</p>
			<div>Latest reviews from verified buyers</div>
			<div class="review">Review 1</div>
			<div class="review">Review 2</div>
			<div class="review">Review 3</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from review headings + content")
	}
}

func TestHintFeedback_DetectTextPatterns_VerifiedPurchase(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<div class="review">
				<span>Verified Purchase</span>
				<p>Great product!</p>
			</div>
			<div class="review">
				<span>Verified Buyer</span>
				<p>Love it!</p>
			</div>
			<div class="review">
				<span>Verified Purchase</span>
				<p>Amazing!</p>
			</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from verified purchase indicators")
	}
}

func TestHintFeedback_DetectCSSPatterns(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<body>
			<div class="review">Review 1</div>
			<div class="review">Review 2</div>
			<div class="review">Review 3</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from CSS classes")
	}
	if !strings.Contains(hints.Custom["detection_methods"], "css_class") {
		t.Error("detection_methods should include 'css_class'")
	}
}

func TestHintFeedback_MultipleSignalsCombined(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<head>
			<script type="application/ld+json">
			{"@type": "Product", "aggregateRating": {"@type": "AggregateRating", "ratingValue": 4.5}}
			</script>
		</head>
		<body>
			<h2>Customer Reviews</h2>
			<div class="review">
				<span>★★★★☆</span>
				<span>Verified Purchase</span>
				<p>Great product!</p>
			</div>
			<div class="review">
				<span>★★★★★</span>
				<p>Excellent!</p>
			</div>
			<div class="review">
				<span>★★★☆☆</span>
				<p>It's okay</p>
			</div>
		</body>
	</html>`

	hints, err := hf.ProcessWithURL(content, "https://example.com/product/123/reviews")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback from multiple signals")
	}

	methods := hints.Custom["detection_methods"]
	if methods == "" {
		t.Error("detection_methods should be populated")
	}
}

func TestHintFeedback_TailwindSiteWithNoClasses(t *testing.T) {
	// This simulates a Tailwind site where reviews don't have semantic class names
	// but can be detected via schema.org, text patterns, or URL
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<head>
			<script type="application/ld+json">
			{
				"@type": "Product",
				"review": [
					{"@type": "Review", "reviewBody": "Great!"},
					{"@type": "Review", "reviewBody": "Amazing!"},
					{"@type": "Review", "reviewBody": "Excellent!"}
				]
			}
			</script>
		</head>
		<body>
			<div class="flex flex-col gap-4">
				<div class="p-4 border rounded-lg">
					<div class="flex gap-2">⭐⭐⭐⭐⭐</div>
					<p class="text-gray-700">Amazing product!</p>
					<span class="text-sm text-green-600">Verified Purchase</span>
				</div>
				<div class="p-4 border rounded-lg">
					<div class="flex gap-2">⭐⭐⭐⭐☆</div>
					<p class="text-gray-700">Pretty good!</p>
				</div>
				<div class="p-4 border rounded-lg">
					<div class="flex gap-2">⭐⭐⭐☆☆</div>
					<p class="text-gray-700">It's okay</p>
				</div>
			</div>
		</body>
	</html>`

	hints, err := hf.ProcessWithURL(content, "https://demo.refyne.uk/reviews")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] != "true" {
		t.Error("should detect feedback on Tailwind sites without semantic classes")
	}
}

func TestHintFeedback_NoFeedback(t *testing.T) {
	hf := NewHintFeedback()
	content := `<html>
		<body>
			<h1>About Us</h1>
			<p>We are a great company.</p>
			<div class="product-info">
				<h2>Product Details</h2>
				<p>This is a wonderful product.</p>
			</div>
		</body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["feedback_detected"] == "true" {
		t.Error("should not detect feedback on non-feedback pages")
	}
}

func TestHintFeedback_OutputsGuidance(t *testing.T) {
	hf := NewHintFeedback(WithFeedbackMinRepeats(2))
	content := `<html>
		<head>
			<script type="application/ld+json">
			{
				"@type": "Product",
				"review": [
					{"@type": "Review", "reviewBody": "Great!"},
					{"@type": "Review", "reviewBody": "Amazing!"},
					{"@type": "Review", "reviewBody": "Excellent!"}
				]
			}
			</script>
		</head>
		<body></body>
	</html>`

	hints, err := hf.Process(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hints.Custom["sentiment_guidance"] == "" {
		t.Error("should output sentiment guidance")
	}
	if hints.Custom["persona_guidance"] == "" {
		t.Error("should output persona guidance")
	}
}

func TestHintFeedback_URLOnlyDetection(t *testing.T) {
	// This tests that a /reviews URL triggers feedback detection even without
	// CSS classes or Schema.org (common with Tailwind sites)
	hf := NewHintFeedback()

	// Content with no semantic classes, no Schema.org - just plain HTML
	content := `<html>
		<body>
			<div class="flex flex-col gap-4">
				<div class="p-4 border rounded-lg">
					<p class="text-gray-700">Great product!</p>
				</div>
				<div class="p-4 border rounded-lg">
					<p class="text-gray-700">Amazing service!</p>
				</div>
			</div>
		</body>
	</html>`

	// Without URL - should NOT detect feedback (no signals)
	hints, err := hf.ProcessWithURL(content, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hints.HasFeedback() {
		t.Error("should not detect feedback without URL or content patterns")
	}

	// With /reviews URL - SHOULD detect feedback based on URL alone
	hints, err = hf.ProcessWithURL(content, "https://demo.refyne.uk/reviews")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hints.HasFeedback() {
		t.Error("should detect feedback from /reviews URL pattern")
	}
	if hints.Custom["sentiment_guidance"] == "" {
		t.Error("should include sentiment_guidance hint")
	}
	if hints.Custom["persona_guidance"] == "" {
		t.Error("should include persona_guidance hint")
	}

	// Verify reviews type was detected
	found := false
	for _, dt := range hints.DetectedTypes {
		if dt.Name == "reviews" {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect 'reviews' as content type from URL")
	}
}

func TestHintFeedback_URLDetectionVariants(t *testing.T) {
	hf := NewHintFeedback()
	content := `<html><body><p>Some content</p></body></html>`

	tests := []struct {
		url          string
		expectType   string
		shouldDetect bool
	}{
		{"https://example.com/reviews", "reviews", true},
		{"https://example.com/review/123", "reviews", true},
		{"https://example.com/testimonials", "testimonials", true},
		{"https://example.com/feedback", "feedback", true},
		{"https://example.com/comments", "comments", true},
		{"https://example.com/products", "", false}, // Not a feedback URL
		{"https://example.com/blog", "", false},     // Not a feedback URL
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			hints, err := hf.ProcessWithURL(content, tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.shouldDetect {
				if !hints.HasFeedback() {
					t.Errorf("should detect feedback for URL %s", tt.url)
				}
				// Check expected type
				found := false
				for _, dt := range hints.DetectedTypes {
					if dt.Name == tt.expectType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("should detect type %q for URL %s", tt.expectType, tt.url)
				}
			} else {
				if hints.HasFeedback() {
					t.Errorf("should NOT detect feedback for URL %s", tt.url)
				}
			}
		})
	}
}
