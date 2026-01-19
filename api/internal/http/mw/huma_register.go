package mw

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// OperationOption is a function that modifies an operation.
type OperationOption func(*huma.Operation)

// WithFeature adds a required feature to the operation metadata.
func WithFeature(feature string) OperationOption {
	return func(op *huma.Operation) {
		if op.Metadata == nil {
			op.Metadata = make(map[string]any)
		}
		op.Metadata[string(MetaKeyRequireFeature)] = feature
	}
}

// WithQuotaCheck marks the operation as requiring quota validation.
func WithQuotaCheck() OperationOption {
	return func(op *huma.Operation) {
		if op.Metadata == nil {
			op.Metadata = make(map[string]any)
		}
		op.Metadata[string(MetaKeyRequireQuota)] = true
	}
}

// WithConcurrencyCheck marks the operation as requiring concurrency limit validation.
func WithConcurrencyCheck() OperationOption {
	return func(op *huma.Operation) {
		if op.Metadata == nil {
			op.Metadata = make(map[string]any)
		}
		op.Metadata[string(MetaKeyRequireConcurrencyLimit)] = true
	}
}

// WithSuperadmin marks the operation as requiring superadmin access.
func WithSuperadmin() OperationOption {
	return func(op *huma.Operation) {
		if op.Metadata == nil {
			op.Metadata = make(map[string]any)
		}
		op.Metadata[string(MetaKeyRequireSuperadmin)] = true
	}
}

// WithTags adds tags to the operation.
func WithTags(tags ...string) OperationOption {
	return func(op *huma.Operation) {
		op.Tags = append(op.Tags, tags...)
	}
}

// WithDescription sets the operation description.
func WithDescription(desc string) OperationOption {
	return func(op *huma.Operation) {
		op.Description = desc
	}
}

// WithSummary sets the operation summary.
func WithSummary(summary string) OperationOption {
	return func(op *huma.Operation) {
		op.Summary = summary
	}
}

// WithOperationID sets a custom operation ID.
func WithOperationID(id string) OperationOption {
	return func(op *huma.Operation) {
		op.OperationID = id
	}
}

// WithHidden hides the operation from OpenAPI documentation.
func WithHidden() OperationOption {
	return func(op *huma.Operation) {
		op.Hidden = true
	}
}

// PublicGet registers a public GET endpoint (no auth required).
func PublicGet[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method: http.MethodGet,
		Path:   path,
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}

// PublicPost registers a public POST endpoint (no auth required).
func PublicPost[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method: http.MethodPost,
		Path:   path,
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}

// ProtectedGet registers a GET endpoint that requires bearer auth.
func ProtectedGet[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method:   http.MethodGet,
		Path:     path,
		Security: []map[string][]string{{SecurityScheme: {}}},
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}

// ProtectedPost registers a POST endpoint that requires bearer auth.
func ProtectedPost[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method:   http.MethodPost,
		Path:     path,
		Security: []map[string][]string{{SecurityScheme: {}}},
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}

// ProtectedPut registers a PUT endpoint that requires bearer auth.
func ProtectedPut[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method:   http.MethodPut,
		Path:     path,
		Security: []map[string][]string{{SecurityScheme: {}}},
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}

// ProtectedDelete registers a DELETE endpoint that requires bearer auth.
func ProtectedDelete[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method:   http.MethodDelete,
		Path:     path,
		Security: []map[string][]string{{SecurityScheme: {}}},
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}

// HiddenGet registers a GET endpoint that won't appear in OpenAPI docs.
// Used for internal endpoints like K8s probes.
func HiddenGet[I, O any](api huma.API, path string, handler func(ctx context.Context, input *I) (*O, error)) {
	huma.Register(api, huma.Operation{
		Method: http.MethodGet,
		Path:   path,
		Hidden: true,
	}, handler)
}

// RegisterProtected is a generic function to register any method with auth.
func RegisterProtected[I, O any](api huma.API, method, path string, handler func(ctx context.Context, input *I) (*O, error), opts ...OperationOption) {
	op := huma.Operation{
		Method:   method,
		Path:     path,
		Security: []map[string][]string{{SecurityScheme: {}}},
	}
	for _, opt := range opts {
		opt(&op)
	}
	huma.Register(api, op, handler)
}
