package preprocessor

// Noop is a no-operation preprocessor that returns empty hints.
// Use this when you want to skip preprocessing but maintain the interface.
type Noop struct{}

// NewNoop creates a new no-op preprocessor.
func NewNoop() *Noop {
	return &Noop{}
}

// Process returns empty hints without analyzing content.
func (n *Noop) Process(content string) (*Hints, error) {
	return NewHints(), nil
}

// Name returns the preprocessor identifier.
func (n *Noop) Name() string {
	return "noop"
}
