package interfaces

import "context"

// contextKey is a private type to avoid key collisions across packages.
type contextKey string

const multimodalContentPartsKey contextKey = "interfaces.multimodal.content_parts"

// WithContextContentParts attaches multimodal content parts to the context.
// This enables passing multimodal input through existing APIs without
// changing method signatures (backward compatible).
func WithContextContentParts(ctx context.Context, parts ...ContentPart) context.Context {
	if len(parts) == 0 {
		return ctx
	}
	return context.WithValue(ctx, multimodalContentPartsKey, parts)
}

// GetContextContentParts retrieves multimodal content parts from the context.
func GetContextContentParts(ctx context.Context) ([]ContentPart, bool) {
	parts, ok := ctx.Value(multimodalContentPartsKey).([]ContentPart)
	return parts, ok
}

