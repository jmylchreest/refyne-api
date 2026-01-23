#!/bin/bash
# Test different cleaner optionsurations with schema extraction
# Usage: ./test-cleaner-extraction.sh [recipe|product] [url]

set -e
API_URL="${API_URL:-http://localhost:8080}"

# Recipe schema (for allrecipes, etc.)
RECIPE_SCHEMA=$(cat <<'EOF'
name: Recipe
fields:
  - name: title
    type: string
    required: true
  - name: description
    type: string
  - name: prep_time
    type: string
  - name: cook_time
    type: string
  - name: total_time
    type: string
  - name: servings
    type: string
  - name: ingredients
    type: array
    required: true
    items:
      type: object
      properties:
        ingredient:
          type: string
          required: true
        quantity:
          type: string
        unit:
          type: string
  - name: instructions
    type: array
    required: true
    items:
      type: string
  - name: nutrition
    type: object
    properties:
      calories:
        type: string
      protein:
        type: string
      carbohydrates:
        type: string
      fat:
        type: string
EOF
)

# Product schema (for e-commerce sites)
PRODUCT_SCHEMA=$(cat <<'EOF'
name: Product
fields:
  - name: title
    type: string
    required: true
  - name: price
    type: string
    required: true
  - name: currency
    type: string
  - name: description
    type: string
  - name: availability
    type: string
  - name: sku
    type: string
  - name: brand
    type: string
  - name: features
    type: array
    items:
      type: string
  - name: specifications
    type: array
    items:
      type: object
      properties:
        name:
          type: string
        value:
          type: string
  - name: images
    type: array
    items:
      type: string
EOF
)

# Default URLs
RECIPE_URL="https://www.allrecipes.com/recipe/23891/grilled-cheese-sandwich/"
PRODUCT_URL="https://thepihut.com/products/raspberry-pi-5"

# Parse arguments
SCHEMA_TYPE="${1:-recipe}"
if [ "$SCHEMA_TYPE" = "recipe" ]; then
    SCHEMA="$RECIPE_SCHEMA"
    TEST_URL="${2:-$RECIPE_URL}"
elif [ "$SCHEMA_TYPE" = "product" ]; then
    SCHEMA="$PRODUCT_SCHEMA"
    TEST_URL="${2:-$PRODUCT_URL}"
else
    echo "Usage: $0 [recipe|product] [url]"
    exit 1
fi

echo "========================================"
echo "Testing: $SCHEMA_TYPE extraction"
echo "URL: $TEST_URL"
echo "========================================"
echo

# Cleaners to test
CLEANERS=("markdown" "refyne" "refyne:aggressive" "trafilatura")

for CLEANER in "${CLEANERS[@]}"; do
    echo "--- Cleaner: $CLEANER ---"

    # Build cleaner chain based on type
    if [[ "$CLEANER" == *":"* ]]; then
        # Has preset like "refyne:aggressive"
        NAME="${CLEANER%%:*}"
        PRESET="${CLEANER##*:}"
        CHAIN="[{\"name\": \"$NAME\", \"options\": {\"preset\": \"$PRESET\"}}, {\"name\": \"markdown\"}]"
    elif [ "$CLEANER" = "refyne" ]; then
        CHAIN='[{"name": "refyne"}, {"name": "markdown"}]'
    else
        CHAIN="[{\"name\": \"$CLEANER\"}]"
    fi

    BODY=$(jq -n \
        --arg url "$TEST_URL" \
        --arg schema "$SCHEMA" \
        --argjson chain "$CHAIN" \
        '{url: $url, schema: $schema, cleaner_chain: $chain}')

    RESULT=$(curl -s -X POST "${API_URL}/api/v1/extract" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer rfs_demo_recipe_app_2026" \
        -H "Origin: http://localhost:8080" \
        -d "$BODY")

    # Extract key metrics
    INPUT_TOKENS=$(echo "$RESULT" | jq -r '.usage.input_tokens // "error"')
    OUTPUT_TOKENS=$(echo "$RESULT" | jq -r '.usage.output_tokens // "error"')
    TITLE=$(echo "$RESULT" | jq -r '.data.title // .error // "no title"')

    printf "  Input tokens: %s | Output tokens: %s\n" "$INPUT_TOKENS" "$OUTPUT_TOKENS"
    printf "  Title: %s\n\n" "$TITLE"
done

echo "========================================"
echo "Full extraction with best cleaner (refyne -> markdown):"
echo "========================================"

CHAIN='[{"name": "refyne"}, {"name": "markdown"}]'
BODY=$(jq -n \
    --arg url "$TEST_URL" \
    --arg schema "$SCHEMA" \
    --argjson chain "$CHAIN" \
    '{url: $url, schema: $schema, cleaner_chain: $chain}')

curl -s -X POST "${API_URL}/api/v1/extract" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer rfs_demo_recipe_app_2026" \
    -H "Origin: http://localhost:8080" \
    -d "$BODY" | jq '{
        title: .data.title,
        input_tokens: .usage.input_tokens,
        output_tokens: .usage.output_tokens,
        data_preview: (if .data.ingredients then
            {ingredients_count: (.data.ingredients | length), first_ingredient: .data.ingredients[0]}
        elif .data.features then
            {features_count: (.data.features | length), price: .data.price}
        else
            .data
        end)
    }'
