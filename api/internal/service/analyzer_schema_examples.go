package service

import (
	"strings"

	"github.com/jmylchreest/refyne-api/internal/preprocessor"
)

// schemaExample holds a schema example for a content type.
type schemaExample struct {
	name        string
	description string
	yaml        string
}

// getSchemaExampleForType returns a schema example for the given content type.
func getSchemaExampleForType(contentType string) *schemaExample {
	examples := map[string]*schemaExample{
		"reviews": {
			name:        "reviews",
			description: "Customer/user reviews with sentiment analysis",
			yaml: `  - name: reviews
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: reviewer_name
          type: string
        - name: rating
          type: integer
          description: Rating value (e.g., 1-5)
        - name: review_text
          type: string
        - name: review_date
          type: string
        - name: sentiment
          type: string
          description: "positive, neutral, or negative"
        - name: persona_summary
          type: string
          description: Brief description of reviewer type (e.g., "experienced user", "first-time buyer")
        - name: verified_purchase
          type: boolean`,
		},
		"ratings": {
			name:        "ratings",
			description: "User ratings",
			yaml: `  - name: ratings
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: rating
          type: integer
        - name: max_rating
          type: integer
        - name: reviewer_name
          type: string
        - name: sentiment
          type: string
          description: "positive, neutral, or negative"`,
		},
		"comments": {
			name:        "comments",
			description: "User comments or discussion",
			yaml: `  - name: comments
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: author
          type: string
        - name: comment_text
          type: string
        - name: posted_date
          type: string
        - name: sentiment
          type: string
          description: "positive, neutral, or negative"
        - name: reply_count
          type: integer`,
		},
		"testimonials": {
			name:        "testimonials",
			description: "Customer testimonials",
			yaml: `  - name: testimonials
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: quote
          type: string
        - name: author_name
          type: string
        - name: author_title
          type: string
        - name: company
          type: string
        - name: sentiment
          type: string
          description: "positive, neutral, or negative"`,
		},
		"feedback": {
			name:        "feedback",
			description: "User feedback",
			yaml: `  - name: feedback
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: feedback_text
          type: string
        - name: author
          type: string
        - name: date
          type: string
        - name: sentiment
          type: string
          description: "positive, neutral, or negative"
        - name: category
          type: string`,
		},
		"products": {
			name:        "products",
			description: "Products for sale",
			yaml: `  - name: products
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: description
          type: string
        - name: price
          type: integer
          description: Price in smallest currency unit
        - name: currency
          type: string
        - name: image_url
          type: string
        - name: category
          type: string
        - name: sku
          type: string
        - name: in_stock
          type: boolean`,
		},
		"articles": {
			name:        "articles",
			description: "Blog posts or news articles",
			yaml: `  - name: articles
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: description
          type: string
        - name: author
          type: string
        - name: published_date
          type: string
        - name: category
          type: string
        - name: image_url
          type: string
        - name: read_time
          type: string`,
		},
		"jobs": {
			name:        "jobs",
			description: "Job listings",
			yaml: `  - name: jobs
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: company
          type: string
        - name: location
          type: string
        - name: salary
          type: string
        - name: job_type
          type: string
        - name: posted_date
          type: string
        - name: description
          type: string`,
		},
		"recipes": {
			name:        "recipes",
			description: "Recipes or meals",
			yaml: `  - name: recipes
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: description
          type: string
        - name: cooking_time
          type: string
        - name: prep_time
          type: string
        - name: servings
          type: integer
        - name: difficulty
          type: string
        - name: image_url
          type: string`,
		},
		"events": {
			name:        "events",
			description: "Events or happenings",
			yaml: `  - name: events
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: description
          type: string
        - name: date
          type: string
        - name: time
          type: string
        - name: location
          type: string
        - name: price
          type: integer
        - name: currency
          type: string`,
		},
	}

	if example, ok := examples[contentType]; ok {
		return example
	}
	return nil
}

// buildSchemaExamples generates schema examples based on detected content types.
func buildSchemaExamples(hints *preprocessor.Hints) string {
	var sb strings.Builder

	// Get detected types
	detectedTypes := []string{}
	if hints != nil {
		detectedTypes = hints.GetDetectedTypeNames()
	}

	// If we have detected specific types, show relevant examples
	if len(detectedTypes) > 0 {
		sb.WriteString("\n### Schema Examples for Detected Content\n\n")
		sb.WriteString("Based on the detected content, here are relevant schema examples:\n\n")
		sb.WriteString("```yaml\nfields:\n")

		for _, contentType := range detectedTypes {
			if example := getSchemaExampleForType(contentType); example != nil {
				sb.WriteString(example.yaml)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("```\n")

		// Add special guidance for feedback content
		if hints.HasFeedback() {
			sb.WriteString("\n**Important for feedback content**: Include `sentiment` (positive/neutral/negative) ")
			sb.WriteString("and `persona_summary` (brief reviewer description) fields to capture the qualitative aspects.\n")
		}
	} else {
		// No specific detection - show generic examples
		sb.WriteString("\n### Schema Example\n\n")
		sb.WriteString("Example for a site with blog posts:\n")
		sb.WriteString("```yaml\nfields:\n")
		sb.WriteString(`  - name: articles
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: published_date
          type: string
        - name: author
          type: string
`)
		sb.WriteString("```\n")
	}

	return sb.String()
}
