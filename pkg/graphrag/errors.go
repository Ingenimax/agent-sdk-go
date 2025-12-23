package graphrag

import "errors"

// Common errors for GraphRAG operations
var (
	// ErrEntityNotFound is returned when an entity cannot be found
	ErrEntityNotFound = errors.New("entity not found")

	// ErrRelationshipNotFound is returned when a relationship cannot be found
	ErrRelationshipNotFound = errors.New("relationship not found")

	// ErrInvalidEntityID is returned when an entity ID is invalid or empty
	ErrInvalidEntityID = errors.New("invalid entity ID")

	// ErrInvalidRelationshipID is returned when a relationship ID is invalid or empty
	ErrInvalidRelationshipID = errors.New("invalid relationship ID")

	// ErrMissingEntityName is returned when an entity name is missing
	ErrMissingEntityName = errors.New("entity name is required")

	// ErrMissingEntityType is returned when an entity type is missing
	ErrMissingEntityType = errors.New("entity type is required")

	// ErrMissingSourceID is returned when a relationship source ID is missing
	ErrMissingSourceID = errors.New("relationship source ID is required")

	// ErrMissingTargetID is returned when a relationship target ID is missing
	ErrMissingTargetID = errors.New("relationship target ID is required")

	// ErrMissingRelationshipType is returned when a relationship type is missing
	ErrMissingRelationshipType = errors.New("relationship type is required")

	// ErrNoEmbedder is returned when an embedder is required but not configured
	ErrNoEmbedder = errors.New("embedder not configured")

	// ErrNoLLM is returned when an LLM is required but not provided
	ErrNoLLM = errors.New("LLM is required for extraction")

	// ErrConnectionFailed is returned when connection to the backend fails
	ErrConnectionFailed = errors.New("failed to connect to GraphRAG backend")

	// ErrSchemaValidation is returned when schema validation fails
	ErrSchemaValidation = errors.New("schema validation failed")

	// ErrPathNotFound is returned when no path exists between entities
	ErrPathNotFound = errors.New("no path found between entities")

	// ErrMaxDepthExceeded is returned when traversal depth limit is exceeded
	ErrMaxDepthExceeded = errors.New("maximum traversal depth exceeded")

	// ErrExtractionFailed is returned when entity extraction fails
	ErrExtractionFailed = errors.New("entity extraction failed")

	// ErrDuplicateEntity is returned when trying to create an entity with an existing ID
	ErrDuplicateEntity = errors.New("entity with this ID already exists")

	// ErrDuplicateRelationship is returned when trying to create a relationship with an existing ID
	ErrDuplicateRelationship = errors.New("relationship with this ID already exists")

	// ErrSourceEntityNotFound is returned when the source entity of a relationship doesn't exist
	ErrSourceEntityNotFound = errors.New("source entity not found")

	// ErrTargetEntityNotFound is returned when the target entity of a relationship doesn't exist
	ErrTargetEntityNotFound = errors.New("target entity not found")

	// ErrInvalidStrength is returned when relationship strength is outside 0-1 range
	ErrInvalidStrength = errors.New("relationship strength must be between 0.0 and 1.0")

	// ErrEmptyQuery is returned when a search query is empty
	ErrEmptyQuery = errors.New("search query cannot be empty")

	// ErrInvalidDepth is returned when traversal depth is invalid (negative or too large)
	ErrInvalidDepth = errors.New("invalid traversal depth")
)

// IsNotFoundError returns true if the error is a not-found error
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrEntityNotFound) ||
		errors.Is(err, ErrRelationshipNotFound) ||
		errors.Is(err, ErrPathNotFound) ||
		errors.Is(err, ErrSourceEntityNotFound) ||
		errors.Is(err, ErrTargetEntityNotFound)
}

// IsValidationError returns true if the error is a validation error
func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidEntityID) ||
		errors.Is(err, ErrInvalidRelationshipID) ||
		errors.Is(err, ErrMissingEntityName) ||
		errors.Is(err, ErrMissingEntityType) ||
		errors.Is(err, ErrMissingSourceID) ||
		errors.Is(err, ErrMissingTargetID) ||
		errors.Is(err, ErrMissingRelationshipType) ||
		errors.Is(err, ErrInvalidStrength) ||
		errors.Is(err, ErrEmptyQuery) ||
		errors.Is(err, ErrInvalidDepth) ||
		errors.Is(err, ErrSchemaValidation)
}
