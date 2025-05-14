package embedding

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MetadataFilter represents a filter condition for document metadata.
// It defines a single condition to be applied on a specific field.
// Example: field="word_count", operator=">", value=10
type MetadataFilter struct {
	// Field is the metadata field to filter on
	Field string

	// Operator is the comparison operator
	// Supported operators: "=", "!=", ">", ">=", "<", "<=", "contains", "in", "not_in"
	Operator string

	// Value is the value to compare against
	Value interface{}
}

// MetadataFilterGroup represents a group of filters with a logical operator.
// It allows for complex nested conditions with AND/OR logic.
// Example: (word_count > 10 AND type = "article") OR (category IN ["news", "blog"])
type MetadataFilterGroup struct {
	// Filters is the list of filters in this group
	Filters []MetadataFilter

	// SubGroups is the list of sub-groups in this group
	SubGroups []MetadataFilterGroup

	// Operator is the logical operator to apply between filters
	// Supported operators: "and", "or"
	Operator string
}

// NewMetadataFilter creates a new metadata filter
func NewMetadataFilter(field, operator string, value interface{}) MetadataFilter {
	return MetadataFilter{
		Field:    field,
		Operator: operator,
		Value:    value,
	}
}

// NewMetadataFilterGroup creates a new metadata filter group
func NewMetadataFilterGroup(operator string, filters ...MetadataFilter) MetadataFilterGroup {
	return MetadataFilterGroup{
		Filters:  filters,
		Operator: strings.ToLower(operator),
	}
}

// AddFilter adds a filter to the group
func (g *MetadataFilterGroup) AddFilter(filter MetadataFilter) {
	g.Filters = append(g.Filters, filter)
}

// AddSubGroup adds a sub-group to the group
func (g *MetadataFilterGroup) AddSubGroup(subGroup MetadataFilterGroup) {
	g.SubGroups = append(g.SubGroups, subGroup)
}

// ApplyFilters filters a list of documents based on metadata filters
func ApplyFilters(docs []interfaces.Document, filterGroup MetadataFilterGroup) []interfaces.Document {
	if len(filterGroup.Filters) == 0 && len(filterGroup.SubGroups) == 0 {
		return docs
	}

	var filtered []interfaces.Document
	for _, doc := range docs {
		if evaluateFilterGroup(doc.Metadata, filterGroup) {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}

// evaluateFilterGroup evaluates a filter group against document metadata
func evaluateFilterGroup(metadata map[string]interface{}, group MetadataFilterGroup) bool {
	if len(group.Filters) == 0 && len(group.SubGroups) == 0 {
		return true
	}

	// Evaluate individual filters
	filterResults := make([]bool, len(group.Filters))
	for i, filter := range group.Filters {
		filterResults[i] = evaluateFilter(metadata, filter)
	}

	// Evaluate sub-groups
	subGroupResults := make([]bool, len(group.SubGroups))
	for i, subGroup := range group.SubGroups {
		subGroupResults[i] = evaluateFilterGroup(metadata, subGroup)
	}

	// Combine results based on operator
	switch strings.ToLower(group.Operator) {
	case "or":
		// OR logic: return true if any filter or sub-group is true
		for _, result := range filterResults {
			if result {
				return true
			}
		}
		for _, result := range subGroupResults {
			if result {
				return true
			}
		}
		return false
	case "and", "": // Default to AND if not specified
		// AND logic: return false if any filter or sub-group is false
		for _, result := range filterResults {
			if !result {
				return false
			}
		}
		for _, result := range subGroupResults {
			if !result {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// evaluateFilter evaluates a single filter against document metadata
func evaluateFilter(metadata map[string]interface{}, filter MetadataFilter) bool {
	// Handle nested fields with dot notation (e.g., "user.name")
	if strings.Contains(filter.Field, ".") {
		return evaluateNestedFilter(metadata, filter)
	}

	value, exists := metadata[filter.Field]
	if !exists {
		// If the field doesn't exist, the filter doesn't match
		return false
	}

	switch strings.ToLower(filter.Operator) {
	case "=", "==", "eq":
		return equals(value, filter.Value)
	case "!=", "<>", "ne":
		return !equals(value, filter.Value)
	case ">", "gt":
		return compare(value, filter.Value) > 0
	case ">=", "gte":
		return compare(value, filter.Value) >= 0
	case "<", "lt":
		return compare(value, filter.Value) < 0
	case "<=", "lte":
		return compare(value, filter.Value) <= 0
	case "contains":
		return contains(value, filter.Value)
	case "in":
		return valueIn(value, filter.Value)
	case "not_in":
		return !valueIn(value, filter.Value)
	default:
		return false
	}
}

// evaluateNestedFilter handles filters with dot notation for nested fields
func evaluateNestedFilter(metadata map[string]interface{}, filter MetadataFilter) bool {
	parts := strings.Split(filter.Field, ".")
	current := metadata

	// Navigate through nested maps until the last part
	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			return false // Field path doesn't exist
		}

		// Check if the next level is a map
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			// Try to convert from map[interface{}]interface{} which might come from YAML/JSON
			if ifaceMap, isIfaceMap := next.(map[interface{}]interface{}); isIfaceMap {
				nextMap = make(map[string]interface{})
				for k, v := range ifaceMap {
					if kStr, ok := k.(string); ok {
						nextMap[kStr] = v
					}
				}
				current = nextMap
				continue
			}
			return false // Not a map, can't continue
		}

		current = nextMap
	}

	// Create a new filter for the final field
	lastField := parts[len(parts)-1]
	newFilter := MetadataFilter{
		Field:    lastField,
		Operator: filter.Operator,
		Value:    filter.Value,
	}

	// Evaluate the filter on the final nested map
	return evaluateFilter(current, newFilter)
}

// equals checks if two values are equal
func equals(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// compare compares two values and returns:
// -1 if a < b
//
//	0 if a == b
//	1 if a > b
func compare(a, b interface{}) int {
	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)

	// Handle nil values
	if !aVal.IsValid() || !bVal.IsValid() {
		if !aVal.IsValid() && !bVal.IsValid() {
			return 0
		}
		if !aVal.IsValid() {
			return -1
		}
		return 1
	}

	// Handle different types
	aType := aVal.Type()
	bType := bVal.Type()

	// Try to convert to comparable types
	switch {
	case isNumeric(aType) && isNumeric(bType):
		// Convert both to float64 for comparison
		aFloat := toFloat64(a)
		bFloat := toFloat64(b)
		if aFloat < bFloat {
			return -1
		} else if aFloat > bFloat {
			return 1
		}
		return 0

	case isString(aType) && isString(bType):
		// Compare as strings
		aStr := toString(a)
		bStr := toString(b)
		return strings.Compare(aStr, bStr)

	case isTime(a) && isTime(b):
		// Compare as time.Time
		aTime := toTime(a)
		bTime := toTime(b)
		if aTime.Before(bTime) {
			return -1
		} else if aTime.After(bTime) {
			return 1
		}
		return 0

	default:
		// For incomparable types, compare string representations
		aStr := fmt.Sprintf("%v", a)
		bStr := fmt.Sprintf("%v", b)
		return strings.Compare(aStr, bStr)
	}
}

// contains checks if a contains b
func contains(a, b interface{}) bool {
	aStr := toString(a)
	bStr := toString(b)
	return strings.Contains(aStr, bStr)
}

// valueIn checks if a is in the collection b
func valueIn(a, b interface{}) bool {
	// If b is not a collection, compare directly
	bVal := reflect.ValueOf(b)
	if bVal.Kind() != reflect.Slice && bVal.Kind() != reflect.Array {
		return equals(a, b)
	}

	// Check if a is in the collection b
	for i := 0; i < bVal.Len(); i++ {
		if equals(a, bVal.Index(i).Interface()) {
			return true
		}
	}
	return false
}

// isNumeric checks if a type is numeric
func isNumeric(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// isString checks if a type is a string
func isString(t reflect.Type) bool {
	return t.Kind() == reflect.String
}

// isTime checks if a value is a time.Time
func isTime(v interface{}) bool {
	_, ok := v.(time.Time)
	return ok
}

// toFloat64 converts a value to float64
func toFloat64(v interface{}) float64 {
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(val.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(val.Uint())
	case reflect.Float32, reflect.Float64:
		return val.Float()
	case reflect.String:
		var f float64
		_, err := fmt.Sscanf(val.String(), "%f", &f)
		if err != nil {
			// Log error and return 0 or handle accordingly
			return 0
		}
		return f
	default:
		return 0
	}
}

// toString converts a value to string
func toString(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

// DefaultTimeFormats provides a list of common time formats for parsing
var DefaultTimeFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// toTime converts a value to time.Time
func toTime(v interface{}) time.Time {
	if t, ok := v.(time.Time); ok {
		return t
	}
	if s, ok := v.(string); ok {
		// Try to parse common time formats
		for _, format := range DefaultTimeFormats {
			if t, err := time.Parse(format, s); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// FilterToMap converts a MetadataFilterGroup to a map for use with vector store filters
// Deprecated: This function produces a format that may not be compatible with all vector stores.
// For Weaviate, use FilterToWeaviateFormat instead.
func FilterToMap(group MetadataFilterGroup) map[string]interface{} {
	result := make(map[string]interface{})

	// Simple case: single filter
	if len(group.Filters) == 1 && len(group.SubGroups) == 0 {
		filter := group.Filters[0]
		result[filter.Field] = map[string]interface{}{
			"operator": operatorToMapKey(filter.Operator),
			"value":    filter.Value,
		}
		return result
	}

	// Complex case: multiple filters or sub-groups
	var conditions []map[string]interface{}

	// Add filters
	for _, filter := range group.Filters {
		condition := map[string]interface{}{
			filter.Field: map[string]interface{}{
				"operator": operatorToMapKey(filter.Operator),
				"value":    filter.Value,
			},
		}
		conditions = append(conditions, condition)
	}

	// Add sub-groups
	for _, subGroup := range group.SubGroups {
		conditions = append(conditions, FilterToMap(subGroup))
	}

	// Combine with operator
	if len(conditions) > 0 {
		result[strings.ToLower(group.Operator)] = conditions
	}

	return result
}

// operatorToMapKey converts a filter operator to a map key
func operatorToMapKey(operator string) string {
	switch strings.ToLower(operator) {
	case "=", "==", "eq":
		return "equals"
	case "!=", "<>", "ne":
		return "notEquals"
	case ">", "gt":
		return "greaterThan"
	case ">=", "gte":
		return "greaterThanEqual"
	case "<", "lt":
		return "lessThan"
	case "<=", "lte":
		return "lessThanEqual"
	case "contains":
		return "contains"
	case "in":
		return "in"
	case "not_in":
		return "notIn"
	default:
		return "equals"
	}
}

// FilterToWeaviateFormat converts a MetadataFilterGroup to a Weaviate-compatible filter format.
// This is the recommended function to use for Weaviate vector store filters.
func FilterToWeaviateFormat(group MetadataFilterGroup) map[string]interface{} {
	// Simple case: single filter
	if len(group.Filters) == 1 && len(group.SubGroups) == 0 {
		filter := group.Filters[0]
		return weaviateFilterFromSingle(filter)
	}

	// Complex case: multiple filters or sub-groups
	var conditions []map[string]interface{}

	// Add filters
	for _, filter := range group.Filters {
		conditions = append(conditions, weaviateFilterFromSingle(filter))
	}

	// Add sub-groups
	for _, subGroup := range group.SubGroups {
		conditions = append(conditions, FilterToWeaviateFormat(subGroup))
	}

	// Combine with operator
	if len(conditions) > 0 {
		op := "And"
		if strings.ToLower(group.Operator) == "or" {
			op = "Or"
		}
		return map[string]interface{}{
			"operator": op,
			"operands": conditions,
		}
	}

	return nil
}

// weaviateFilterFromSingle converts a single MetadataFilter to Weaviate format
func weaviateFilterFromSingle(filter MetadataFilter) map[string]interface{} {
	result := map[string]interface{}{
		"path": []string{filter.Field},
	}

	// Map operator
	switch strings.ToLower(filter.Operator) {
	case "=", "==", "eq":
		result["operator"] = "Equal"
		result["valueString"] = fmt.Sprint(filter.Value)
	case "!=", "<>", "ne":
		result["operator"] = "NotEqual"
		result["valueString"] = fmt.Sprint(filter.Value)
	case ">", "gt":
		result["operator"] = "GreaterThan"
		result["valueNumber"] = toFloat64(filter.Value)
	case ">=", "gte":
		result["operator"] = "GreaterThanEqual"
		result["valueNumber"] = toFloat64(filter.Value)
	case "<", "lt":
		result["operator"] = "LessThan"
		result["valueNumber"] = toFloat64(filter.Value)
	case "<=", "lte":
		result["operator"] = "LessThanEqual"
		result["valueNumber"] = toFloat64(filter.Value)
	case "contains":
		result["operator"] = "Like"
		result["valueString"] = fmt.Sprint(filter.Value)
	case "in":
		// Handle 'in' operator
		result["operator"] = "ContainsAny"
		if values, ok := filter.Value.([]interface{}); ok {
			strValues := make([]string, len(values))
			for i, v := range values {
				strValues[i] = fmt.Sprint(v)
			}
			result["valueString"] = strValues
		} else {
			result["valueString"] = []string{fmt.Sprint(filter.Value)}
		}
	case "not_in":
		// Handle 'not_in' operator
		result["operator"] = "NotContainsAny"
		if values, ok := filter.Value.([]interface{}); ok {
			strValues := make([]string, len(values))
			for i, v := range values {
				strValues[i] = fmt.Sprint(v)
			}
			result["valueString"] = strValues
		} else {
			result["valueString"] = []string{fmt.Sprint(filter.Value)}
		}
	default:
		result["operator"] = "Equal"
		result["valueString"] = fmt.Sprint(filter.Value)
	}

	return result
}

// CreateWeaviateFilter creates a simple Weaviate filter for a single field.
// This is a convenience function for simple filter cases.
// For complex filters, use MetadataFilterGroup and FilterToWeaviateFormat.
func CreateWeaviateFilter(field string, operator string, value interface{}) map[string]interface{} {
	// Create a simple filter
	filter := NewMetadataFilter(field, operator, value)

	// Convert to Weaviate format
	return weaviateFilterFromSingle(filter)
}

// CreateWeaviateAndFilter creates a Weaviate filter with AND logic for multiple conditions.
// This is a convenience function for common filter cases.
func CreateWeaviateAndFilter(conditions ...map[string]interface{}) map[string]interface{} {
	if len(conditions) == 0 {
		return nil
	}

	if len(conditions) == 1 {
		return conditions[0]
	}

	return map[string]interface{}{
		"operator": "And",
		"operands": conditions,
	}
}

// CreateWeaviateOrFilter creates a Weaviate filter with OR logic for multiple conditions.
// This is a convenience function for common filter cases.
func CreateWeaviateOrFilter(conditions ...map[string]interface{}) map[string]interface{} {
	if len(conditions) == 0 {
		return nil
	}

	if len(conditions) == 1 {
		return conditions[0]
	}

	return map[string]interface{}{
		"operator": "Or",
		"operands": conditions,
	}
}
