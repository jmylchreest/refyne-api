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

	if !strings.Contains(result, "## Detected Page Structure") {
		t.Error("should contain header")
	}
	if !strings.Contains(result, "15 repeated products") {
		t.Error("should contain count and name")
	}
	if !strings.Contains(result, "products[] array") {
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

	if !strings.Contains(result, "MIXED CONTENT") {
		t.Error("should indicate mixed content")
	}
	if !strings.Contains(result, "10 products") {
		t.Error("should list products count")
	}
	if !strings.Contains(result, "5 articles") {
		t.Error("should list articles count")
	}
	if !strings.Contains(result, "content_type") {
		t.Error("should suggest content_type field")
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
	hints := NewHints()
	hints.Custom["pagination"] = "detected"
	hints.Custom["category_filters"] = "present"

	result := hints.ToPromptSection()

	if !strings.Contains(result, "pagination: detected") {
		t.Error("should contain custom hints")
	}
	if !strings.Contains(result, "category_filters: present") {
		t.Error("should contain all custom hints")
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
}
