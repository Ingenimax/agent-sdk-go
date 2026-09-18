package eval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

const DefaultMaxDatasetBytes int64 = 10 << 20

const maxCanonicalNumberDigits = 100_000

// LoadDataset reads and validates a dataset using the built-in evaluators.
func LoadDataset(r io.Reader) (Dataset, error) {
	return LoadDatasetWithEvaluators(r, BuiltinEvaluators())
}

// LoadDatasetWithEvaluators reads a dataset and validates custom check types
// against the supplied evaluator registry.
func LoadDatasetWithEvaluators(r io.Reader, evaluators Evaluators) (Dataset, error) {
	if r == nil {
		return Dataset{}, errors.New("eval: dataset reader is nil")
	}
	limited := io.LimitReader(r, DefaultMaxDatasetBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return Dataset{}, fmt.Errorf("eval: read dataset: %w", err)
	}
	if int64(len(data)) > DefaultMaxDatasetBytes {
		return Dataset{}, fmt.Errorf("eval: dataset exceeds %d bytes", DefaultMaxDatasetBytes)
	}
	return ParseDatasetWithEvaluators(data, evaluators)
}

// ParseDataset parses and validates a JSON dataset with built-in evaluators.
func ParseDataset(data []byte) (Dataset, error) {
	return ParseDatasetWithEvaluators(data, BuiltinEvaluators())
}

// ParseDatasetWithEvaluators parses a single strict JSON document. Duplicate
// object keys and unknown fields are rejected, including inside check configs.
func ParseDatasetWithEvaluators(data []byte, evaluators Evaluators) (Dataset, error) {
	if len(data) == 0 {
		return Dataset{}, errors.New("eval: dataset is empty")
	}
	if len(data) > int(DefaultMaxDatasetBytes) {
		return Dataset{}, fmt.Errorf("eval: dataset exceeds %d bytes", DefaultMaxDatasetBytes)
	}
	if !utf8.Valid(data) {
		return Dataset{}, errors.New("eval: dataset is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return Dataset{}, fmt.Errorf("eval: invalid dataset JSON: %w", err)
	}

	var dataset Dataset
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&dataset); err != nil {
		return Dataset{}, fmt.Errorf("eval: decode dataset: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Dataset{}, fmt.Errorf("eval: decode dataset: %w", err)
	}
	if err := ValidateDataset(&dataset, evaluators); err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}

// ValidateDataset validates a dataset and resolves omitted threshold/required
// values in place. No agent execution occurs during validation.
func ValidateDataset(dataset *Dataset, evaluators Evaluators) error {
	if dataset == nil {
		return errors.New("eval: dataset is nil")
	}
	if dataset.SchemaVersion != DatasetSchemaVersion {
		return fmt.Errorf("eval: unsupported dataset schema_version %q", dataset.SchemaVersion)
	}
	if dataset.ID == "" {
		return errors.New("eval: dataset id is required")
	}
	if !utf8.ValidString(dataset.ID) {
		return errors.New("eval: dataset id is not valid UTF-8")
	}
	if len(dataset.Cases) == 0 {
		return errors.New("eval: dataset must contain at least one case")
	}
	if evaluators == nil {
		return errors.New("eval: evaluator registry is nil")
	}

	retainedBytes := len(dataset.ID)
	caseIDs := make(map[string]struct{}, len(dataset.Cases))
	for caseIndex := range dataset.Cases {
		c := &dataset.Cases[caseIndex]
		path := fmt.Sprintf("cases[%d]", caseIndex)
		if c.ID == "" {
			return fmt.Errorf("eval: %s.id is required", path)
		}
		if !utf8.ValidString(c.ID) {
			return fmt.Errorf("eval: %s.id is not valid UTF-8", path)
		}
		if _, duplicate := caseIDs[c.ID]; duplicate {
			return fmt.Errorf("eval: duplicate case id %q", c.ID)
		}
		caseIDs[c.ID] = struct{}{}
		if !utf8.ValidString(c.Input) {
			return fmt.Errorf("eval: case %q input is not valid UTF-8", c.ID)
		}
		retainedBytes += len(c.ID) + len(c.Input)
		if c.Reference != nil {
			if !utf8.ValidString(*c.Reference) {
				return fmt.Errorf("eval: case %q reference is not valid UTF-8", c.ID)
			}
			retainedBytes += len(*c.Reference)
		}
		for tagIndex, tag := range c.Tags {
			if !utf8.ValidString(tag) {
				return fmt.Errorf("eval: case %q tags[%d] is not valid UTF-8", c.ID, tagIndex)
			}
			retainedBytes += len(tag)
		}
		if len(c.Checks) == 0 {
			return fmt.Errorf("eval: case %q must contain at least one check", c.ID)
		}

		checkIDs := make(map[string]struct{}, len(c.Checks))
		required := 0
		for checkIndex := range c.Checks {
			check := &c.Checks[checkIndex]
			checkPath := fmt.Sprintf("case %q checks[%d]", c.ID, checkIndex)
			if check.ID == "" {
				return fmt.Errorf("eval: %s.id is required", checkPath)
			}
			if !utf8.ValidString(check.ID) || !utf8.ValidString(check.Type) || !utf8.Valid(check.Config) {
				return fmt.Errorf("eval: %s contains invalid UTF-8", checkPath)
			}
			retainedBytes += len(check.ID) + len(check.Type) + len(check.Config)
			if retainedBytes > int(DefaultMaxDatasetBytes) {
				return fmt.Errorf("eval: dataset exceeds %d bytes", DefaultMaxDatasetBytes)
			}
			if _, duplicate := checkIDs[check.ID]; duplicate {
				return fmt.Errorf("eval: case %q has duplicate check id %q", c.ID, check.ID)
			}
			checkIDs[check.ID] = struct{}{}
			if check.Type == "" {
				return fmt.Errorf("eval: %s.type is required", checkPath)
			}
			evaluator, ok := evaluators[check.Type]
			if !ok || evaluator == nil {
				return fmt.Errorf("eval: %s uses unknown evaluator %q", checkPath, check.Type)
			}
			if len(bytes.TrimSpace(check.Config)) == 0 || bytes.Equal(bytes.TrimSpace(check.Config), []byte("null")) {
				return fmt.Errorf("eval: %s.config must be an object", checkPath)
			}
			threshold := check.ThresholdValue()
			if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
				return fmt.Errorf("eval: %s.threshold must be between 0 and 1", checkPath)
			}
			if check.Threshold == nil {
				value := threshold
				check.Threshold = &value
			}
			isRequired := check.RequiredValue()
			if check.Required == nil {
				value := isRequired
				check.Required = &value
			}
			if isRequired {
				required++
			}
			if err := validateEvaluator(evaluator, *check); err != nil {
				return fmt.Errorf("eval: %s (%s): %w", checkPath, check.ID, err)
			}
		}
		if required == 0 {
			return fmt.Errorf("eval: case %q must contain at least one required check", c.ID)
		}
	}
	return nil
}

func validateEvaluator(evaluator Evaluator, check Check) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("evaluator validation panic: %v", recovered)
		}
	}()
	return evaluator.Validate(check)
}

// DatasetDigest returns a stable digest of the validated, resolved dataset.
// encoding/json sorts object keys; numeric spellings inside evaluator configs
// are preserved exactly.
func DatasetDigest(dataset Dataset) (string, error) {
	canonical, err := canonicalDataset(dataset)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func canonicalDataset(dataset Dataset) ([]byte, error) {
	clone := dataset
	clone.Cases = append([]Case(nil), dataset.Cases...)
	for caseIndex := range clone.Cases {
		clone.Cases[caseIndex].Tags = append([]string(nil), dataset.Cases[caseIndex].Tags...)
		clone.Cases[caseIndex].Checks = append([]Check(nil), dataset.Cases[caseIndex].Checks...)
		for checkIndex := range clone.Cases[caseIndex].Checks {
			check := &clone.Cases[caseIndex].Checks[checkIndex]
			threshold := check.ThresholdValue()
			required := check.RequiredValue()
			check.Threshold = &threshold
			check.Required = &required
			var config any
			decoder := json.NewDecoder(bytes.NewReader(check.Config))
			decoder.UseNumber()
			if err := decoder.Decode(&config); err != nil {
				return nil, fmt.Errorf("eval: canonicalize check %q: %w", check.ID, err)
			}
			config, err := canonicalizeJSONValue(config)
			if err != nil {
				return nil, fmt.Errorf("eval: canonicalize check %q: %w", check.ID, err)
			}
			configJSON, err := json.Marshal(config)
			if err != nil {
				return nil, fmt.Errorf("eval: canonicalize check %q: %w", check.ID, err)
			}
			check.Config = configJSON
		}
	}
	data, err := json.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("eval: canonicalize dataset: %w", err)
	}
	return data, nil
}

type canonicalJSONNumber string

func (number canonicalJSONNumber) MarshalJSON() ([]byte, error) {
	return []byte(number), nil
}

func canonicalizeJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		number, err := canonicalizeJSONNumber(typed.String())
		if err != nil {
			return nil, err
		}
		return canonicalJSONNumber(number), nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			canonical, err := canonicalizeJSONValue(child)
			if err != nil {
				return nil, err
			}
			result[key] = canonical
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			canonical, err := canonicalizeJSONValue(child)
			if err != nil {
				return nil, err
			}
			result[index] = canonical
		}
		return result, nil
	default:
		return value, nil
	}
}

func canonicalizeJSONNumber(number string) (string, error) {
	if len(number) > maxCanonicalNumberDigits {
		return "", errors.New("JSON number exceeds canonicalization limit")
	}
	if exponentIndex := strings.IndexAny(number, "eE"); exponentIndex >= 0 {
		exponent, err := strconv.ParseInt(number[exponentIndex+1:], 10, 64)
		if err != nil || exponent > maxCanonicalNumberDigits || exponent < -maxCanonicalNumberDigits {
			return "", errors.New("JSON number exponent exceeds canonicalization limit")
		}
	}
	rational, ok := new(big.Rat).SetString(number)
	if !ok {
		return "", fmt.Errorf("invalid JSON number %q", number)
	}
	if rational.Sign() == 0 {
		return "0", nil
	}

	denominator := new(big.Int).Set(rational.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	remainder := new(big.Int)
	twos, fives := 0, 0
	for {
		quotient := new(big.Int)
		quotient.QuoRem(denominator, two, remainder)
		if remainder.Sign() != 0 {
			break
		}
		denominator = quotient
		twos++
	}
	for {
		quotient := new(big.Int)
		quotient.QuoRem(denominator, five, remainder)
		if remainder.Sign() != 0 {
			break
		}
		denominator = quotient
		fives++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return "", fmt.Errorf("JSON number %q is not a finite decimal", number)
	}

	places := twos
	if fives > places {
		places = fives
	}
	scaled := new(big.Int).Abs(new(big.Int).Set(rational.Num()))
	if twos < places {
		scaled.Mul(scaled, new(big.Int).Exp(two, big.NewInt(int64(places-twos)), nil))
	}
	if fives < places {
		scaled.Mul(scaled, new(big.Int).Exp(five, big.NewInt(int64(places-fives)), nil))
	}
	digits := scaled.String()
	if places > 0 {
		if len(digits) <= places {
			digits = strings.Repeat("0", places-len(digits)+1) + digits
		}
		position := len(digits) - places
		digits = digits[:position] + "." + digits[position:]
		digits = strings.TrimRight(digits, "0")
		digits = strings.TrimRight(digits, ".")
	}
	if rational.Sign() < 0 {
		digits = "-" + digits
	}
	return digits, nil
}

func decodeConfig(raw json.RawMessage, target any) error {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values are not allowed")
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkJSONValue(decoder, "$"); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func walkJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", path)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate object key %q at %s", key, path)
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder, path+"."+key); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("unterminated object at %s", path)
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := walkJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("unterminated array at %s", path)
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delim, path)
	}
	return nil
}
