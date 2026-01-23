#!/bin/bash
# Test the demo API key with a recipe extraction

API_URL="${API_URL:-http://localhost:8080}"

SCHEMA=$(cat <<'EOF'
# Recipe extraction schema
name: Recipe

fields:
  # Basic info
  - name: title
    type: string
    required: true

  - name: image_url
    type: string

  - name: cuisine
    type: string

  - name: difficulty
    type: string  # Easy, Medium, or Hard

  # Timing
  - name: prep_time
    type: string

  - name: cook_time
    type: string

  - name: total_time
    type: string

  # Servings & nutrition
  - name: servings
    type: integer

  - name: calories_per_serving
    type: integer

  - name: meal_type
    type: string  # Breakfast, Lunch, Dinner, etc.

  # Ratings
  - name: rating
    type: number  # Out of 5

  - name: review_count
    type: integer

  # Ingredients - food items needed with quantities
  - name: ingredients
    type: array
    required: true
    items:
      type: object
      properties:
        ingredient:
          type: string
          required: true
          description: Ingredient name (e.g., "all-purpose flour")
        quantity:
          type: string
          description: Amount (e.g., "2", "1/2", "3-4")
        unit:
          type: string
          description: Unit of measure (e.g., "cups", "tbsp", "oz")

  # Instructions - numbered cooking steps
  - name: instructions
    type: array
    required: true
    items:
      type: string  # e.g. "Preheat oven to 475F", "Roll out dough"

  # Tags/categories
  - name: tags
    type: array
    items:
      type: string
EOF
)

TEST_URL="${1:-https://www.allrecipes.com/recipe/23891/grilled-cheese-sandwich/}"

curl -X POST "${API_URL}/api/v1/extract" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer rfs_demo_recipe_app_2026" \
  -H "Origin: http://localhost:8080" \
  -d "$(jq -n --arg url "$TEST_URL" --arg schema "$SCHEMA" '{url: $url, schema: $schema}')"
