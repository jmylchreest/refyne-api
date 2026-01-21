// Package main provides a CLI tool to generate the OpenAPI specification for the Refyne API.
// This binary uses the shared route definitions with stub handlers to produce an accurate
// OpenAPI spec without requiring any real services, databases, or external dependencies.
//
// Usage:
//
//	go run ./cmd/refyne-openapi > openapi.json
//	go run ./cmd/refyne-openapi -yaml > openapi.yaml
//	go run ./cmd/refyne-openapi -output openapi.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/jmylchreest/refyne-api/internal/http/routes"
	"github.com/jmylchreest/refyne-api/internal/version"
)

func main() {
	// Parse flags
	outputFile := flag.String("output", "", "Output file path (default: stdout)")
	outputYAML := flag.Bool("yaml", false, "Output as YAML instead of JSON")
	baseURL := flag.String("base-url", "https://api.refyne.uk", "Base URL for the API server")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Get().Short())
		return
	}

	// Create a minimal chi router - we won't actually serve requests
	router := chi.NewRouter()

	// Create Huma API with shared config
	cfg := routes.NewHumaConfig(*baseURL)
	api := humachi.New(router, cfg)

	// Register all routes with stub handlers
	routes.Register(api, routes.StubHandlers())

	// Get the OpenAPI spec
	spec := api.OpenAPI()

	// Marshal the spec to the desired format
	var data []byte
	var err error

	if *outputYAML {
		data, err = yaml.Marshal(spec)
	} else {
		data, err = json.MarshalIndent(spec, "", "  ")
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	// Output to file or stdout
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing to file: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "OpenAPI spec written to %s\n", *outputFile)
	} else {
		fmt.Print(string(data))
	}
}
