#!/bin/bash
# Demonstrate using custom remove/keep selectors for site-specific cleaning
# Usage: ./test-custom-selectors.sh

set -e
API_URL="${API_URL:-http://localhost:8080}"

PRODUCT_URL="https://thepihut.com/products/raspberry-pi-5"

PRODUCT_SCHEMA=$(cat <<'EOF'
name: Product
fields:
  - name: title
    type: string
    required: true
  - name: price
    type: string
    required: true
  - name: description
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
EOF
)

echo "========================================"
echo "Custom Selector Examples for thepihut.com"
echo "========================================"
echo

# Example 1: Default refyne (no custom selectors)
echo "--- 1. Default refyne -> markdown ---"
BODY=$(jq -n \
    --arg url "$PRODUCT_URL" \
    --arg schema "$PRODUCT_SCHEMA" \
    '{
        url: $url,
        schema: $schema,
        cleaner_chain: [
            {"name": "refyne"},
            {"name": "markdown"}
        ]
    }')

RESULT=$(curl -s -X POST "${API_URL}/api/v1/extract" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer rfs_demo_recipe_app_2026" \
    -H "Origin: http://localhost:8080" \
    -d "$BODY")

echo "$RESULT" | jq '{input_tokens: .usage.input_tokens, title: .data.title}'
echo

# Example 2: With site-specific remove selectors
echo "--- 2. With custom remove selectors (site navigation, related products) ---"
BODY=$(jq -n \
    --arg url "$PRODUCT_URL" \
    --arg schema "$PRODUCT_SCHEMA" \
    '{
        url: $url,
        schema: $schema,
        cleaner_chain: [
            {
                "name": "refyne",
                "options": {
                    "remove_selectors": [
                        ".site-header",
                        ".site-footer",
                        ".product-recommendations",
                        ".recently-viewed",
                        ".breadcrumb",
                        "[data-section-type=\"featured-collection\"]",
                        ".shopify-section--footer",
                        ".announcement-bar"
                    ]
                }
            },
            {"name": "markdown"}
        ]
    }')

RESULT=$(curl -s -X POST "${API_URL}/api/v1/extract" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer rfs_demo_recipe_app_2026" \
    -H "Origin: http://localhost:8080" \
    -d "$BODY")

echo "$RESULT" | jq '{input_tokens: .usage.input_tokens, title: .data.title}'
echo

# Example 3: Aggressive preset with keep selectors to preserve product info
echo "--- 3. Aggressive preset + keep selectors (preserve product details) ---"
BODY=$(jq -n \
    --arg url "$PRODUCT_URL" \
    --arg schema "$PRODUCT_SCHEMA" \
    '{
        url: $url,
        schema: $schema,
        cleaner_chain: [
            {
                "name": "refyne",
                "options": {
                    "preset": "aggressive",
                    "keep_selectors": [
                        ".product-single",
                        ".product__title",
                        ".product__price",
                        ".product__description",
                        ".product-specs",
                        "[itemprop=\"description\"]",
                        "[itemprop=\"price\"]"
                    ]
                }
            },
            {"name": "markdown"}
        ]
    }')

RESULT=$(curl -s -X POST "${API_URL}/api/v1/extract" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer rfs_demo_recipe_app_2026" \
    -H "Origin: http://localhost:8080" \
    -d "$BODY")

echo "$RESULT" | jq '{input_tokens: .usage.input_tokens, title: .data.title}'
echo

echo "========================================"
echo "Token Usage Summary"
echo "========================================"
echo "Lower input_tokens = more efficient cleaning"
echo "The goal is to reduce tokens while preserving extraction accuracy"
