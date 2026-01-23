package handlers

import (
	"context"

	"github.com/jmylchreest/refyne-api/internal/service"
)

// CleanerOptionResponse describes an available option for a cleaner.
type CleanerOptionResponse struct {
	Name        string `json:"name" doc:"Option name (e.g., 'output', 'tables')"`
	Type        string `json:"type" doc:"Option type (string, boolean)"`
	Default     any    `json:"default" doc:"Default value if not specified"`
	Description string `json:"description" doc:"Description of what this option does"`
}

// CleanerResponse describes an available cleaner.
type CleanerResponse struct {
	Name        string                  `json:"name" doc:"Cleaner name to use in cleaner_chain"`
	Description string                  `json:"description" doc:"Description of what this cleaner does"`
	Options     []CleanerOptionResponse `json:"options,omitempty" doc:"Available options for this cleaner"`
}

// CleanerChainItemResponse describes a cleaner in a chain.
type CleanerChainItemResponse struct {
	Name string `json:"name" doc:"Cleaner name"`
}

// ListCleanersOutput is the response for the cleaners listing endpoint.
type ListCleanersOutput struct {
	Body struct {
		Cleaners                []CleanerResponse          `json:"cleaners" doc:"List of available cleaners"`
		DefaultExtractionChain  []CleanerChainItemResponse `json:"default_extraction_chain" doc:"Default cleaner chain for extraction operations"`
		DefaultAnalysisChain    []CleanerChainItemResponse `json:"default_analysis_chain" doc:"Default cleaner chain for analysis operations"`
	}
}

// ListCleaners returns information about all available content cleaners.
// This is a public endpoint for documentation and client configuration.
func ListCleaners(ctx context.Context, _ *struct{}) (*ListCleanersOutput, error) {
	// Get cleaner info from service
	cleanerInfos := service.GetAvailableCleaners()

	// Convert to response format
	cleaners := make([]CleanerResponse, len(cleanerInfos))
	for i, info := range cleanerInfos {
		var options []CleanerOptionResponse
		if len(info.Options) > 0 {
			options = make([]CleanerOptionResponse, len(info.Options))
			for j, opt := range info.Options {
				options[j] = CleanerOptionResponse{
					Name:        opt.Name,
					Type:        opt.Type,
					Default:     opt.Default,
					Description: opt.Description,
				}
			}
		}
		cleaners[i] = CleanerResponse{
			Name:        info.Name,
			Description: info.Description,
			Options:     options,
		}
	}

	// Get default chains
	extractionChain := service.GetDefaultExtractionChain()
	analysisChain := service.GetDefaultAnalyzerChain()

	extractionChainResp := make([]CleanerChainItemResponse, len(extractionChain))
	for i, cfg := range extractionChain {
		extractionChainResp[i] = CleanerChainItemResponse{Name: cfg.Name}
	}

	analysisChainResp := make([]CleanerChainItemResponse, len(analysisChain))
	for i, cfg := range analysisChain {
		analysisChainResp[i] = CleanerChainItemResponse{Name: cfg.Name}
	}

	return &ListCleanersOutput{
		Body: struct {
			Cleaners               []CleanerResponse          `json:"cleaners" doc:"List of available cleaners"`
			DefaultExtractionChain []CleanerChainItemResponse `json:"default_extraction_chain" doc:"Default cleaner chain for extraction operations"`
			DefaultAnalysisChain   []CleanerChainItemResponse `json:"default_analysis_chain" doc:"Default cleaner chain for analysis operations"`
		}{
			Cleaners:               cleaners,
			DefaultExtractionChain: extractionChainResp,
			DefaultAnalysisChain:   analysisChainResp,
		},
	}, nil
}
