// Package launchoptions projects the existing Catalog Question contract for
// typed launch input. Defaults and human answers are never public metadata.
package launchoptions

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/PastureStack/catalog-service/model"
)

var ErrUnsupported = errors.New("CatalogLaunchUnsupported")
var ErrInputInvalid = errors.New("CatalogLaunchInputInvalid")
var ErrOptionUnknown = errors.New("CatalogLaunchOptionUnknown")
var ErrHumanInput = errors.New("CatalogLaunchNeedsHumanInputs")

const MaxQuestions = 100

// This is the complete safe output contract. In particular it has no default,
// answer, file, source URL or arbitrary nested configuration field.
type Question struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	SourceType    string   `json:"sourceType"`
	Label         string   `json:"label"`
	Description   string   `json:"description"`
	Required      bool     `json:"required"`
	RequiresHuman bool     `json:"requiresHuman"`
	ReferenceType string   `json:"referenceType,omitempty"`
	Options       []string `json:"options,omitempty"`
	MinLength     int      `json:"minLength,omitempty"`
	MaxLength     int      `json:"maxLength,omitempty"`
	Min           int      `json:"min,omitempty"`
	Max           int      `json:"max,omitempty"`
}

var sensitive = regexp.MustCompile(`(?i)(password|passwd|secret|token|credential|private[_-]?key|api[_-]?key|access[_-]?key|authorization|(^|[^a-z0-9])(pass|pwd|auth)($|[^a-z0-9]))`)

func units(value string) int { return len(utf16.Encode([]rune(value))) }

func clip(value string, limit int) string {
	n := 0
	for offset, char := range value {
		size := 1
		if char > 0xffff {
			size = 2
		}
		if n+size > limit {
			return value[:offset]
		}
		n += size
	}
	return value
}

func Build(input []model.Question) ([]Question, error) {
	if len(input) > MaxQuestions {
		return nil, ErrUnsupported
	}
	result := make([]Question, 0, len(input))
	seen := map[string]bool{}
	for _, source := range input {
		name := source.Variable
		if name == "" || strings.TrimSpace(name) != name || units(name) > 128 || seen[name] ||
			name == "__proto__" || name == "prototype" || name == "constructor" || strings.ContainsAny(name, "\x00\r\n") {
			return nil, ErrUnsupported
		}
		seen[name] = true
		kind := strings.ToLower(source.Type)
		q := Question{Name: name, Type: kind, SourceType: clip(source.Type, 64), Label: clip(source.Label, 256),
			Description: clip(source.Description, 1024), Required: source.Required, MaxLength: 1024}
		q.MinLength = source.MinLength
		q.Min, q.Max = source.Min, source.Max
		if source.MaxLength > 0 {
			q.MaxLength = source.MaxLength
		}
		if q.MinLength < 0 || q.MinLength > q.MaxLength || q.MaxLength > 4096 || source.Min < -9007199254740991 || source.Min > 9007199254740991 || source.Max < -9007199254740991 || source.Max > 9007199254740991 || len(source.ValidChars) > 256 || len(source.InvalidChars) > 256 {
			return nil, ErrUnsupported
		}
		if q.Max != 0 && q.Min > q.Max {
			return nil, ErrUnsupported
		}
		for _, constraint := range []string{source.ValidChars, source.InvalidChars} {
			if constraint != "" {
				if _, err := regexp.Compile("[" + constraint + "]"); err != nil {
					return nil, ErrUnsupported
				}
			}
		}
		if kind == "service" || kind == "certificate" || kind == "secret" {
			// These widgets represent resource identity, never plaintext. A
			// sensitive-looking variable name cannot turn an ID into a value.
			q.Type = "reference"
			q.ReferenceType = kind
		} else if sensitive.MatchString(name) || kind == "password" || kind == "masked" {
			q.Type = "human"
			q.RequiresHuman = true
		} else {
			switch kind {
			case "string", "int", "float", "boolean", "enum":
				q.MinLength = source.MinLength
				if source.MaxLength > 0 {
					q.MaxLength = source.MaxLength
				}
				if q.MinLength < 0 || q.MinLength > q.MaxLength || q.MaxLength > 4096 {
					return nil, ErrUnsupported
				}
				q.Min, q.Max = source.Min, source.Max
				if q.Max != 0 && q.Min > q.Max {
					return nil, ErrUnsupported
				}
				if kind == "enum" {
					if len(source.Options) == 0 || len(source.Options) > 50 {
						return nil, ErrUnsupported
					}
					for _, option := range source.Options {
						if units(option) > 512 {
							return nil, ErrUnsupported
						}
					}
					q.Options = append([]string(nil), source.Options...)
				}
			default:
				// Unsupported widgets remain a real human-input dependency. A
				// configured default never silently makes one model-answerable.
				q.Type = "human"
				q.RequiresHuman = true
			}
		}
		result = append(result, q)
	}
	return result, nil
}

// ValidatePublic accepts no raw resource reference or human-only value. The
// Engine resolves opaque resource refs separately and rechecks their live scope.
// A missing value is reported by name only; false and numeric zero are values.
func ValidatePublic(definitions []model.Question, answers map[string]interface{}) (map[string]interface{}, []string, error) {
	questions, err := Build(definitions)
	if err != nil {
		return nil, nil, err
	}
	if len(answers) > MaxQuestions {
		return nil, nil, ErrInputInvalid
	}
	byName := map[string]Question{}
	for _, q := range questions {
		byName[q.Name] = q
	}
	validated := map[string]interface{}{}
	for name, value := range answers {
		q, ok := byName[name]
		if !ok {
			return nil, nil, ErrOptionUnknown
		}
		if q.RequiresHuman || q.Type == "reference" {
			return nil, nil, ErrHumanInput
		}
		if !validValue(q, value) {
			return nil, nil, ErrInputInvalid
		}
		validated[name] = value
	}
	missing := []string{}
	for _, q := range questions {
		value, found := validated[q.Name]
		if q.Required && !q.RequiresHuman && q.Type != "reference" && (!found || value == nil || value == "") {
			missing = append(missing, q.Name)
		}
	}
	return validated, missing, nil
}

// ValidateLaunch keeps private Question/default semantics in the Catalog owner.
// Resource IDs in the reference channel still require Engine RBAC/Key checks.
// Every question receives an explicit value so executor defaults cannot return.
func ValidateLaunch(definitions []model.Question, safe, human map[string]interface{}, references map[string]string) (map[string]interface{}, []Question, error) {
	questions, err := Build(definitions)
	if err != nil {
		return nil, nil, err
	}
	public, missing, err := ValidatePublic(definitions, safe)
	if err != nil {
		return nil, nil, err
	}
	if len(missing) > 0 {
		return nil, nil, ErrInputInvalid
	}
	byName := map[string]Question{}
	sourceByName := map[string]model.Question{}
	for i, q := range questions {
		byName[q.Name] = q
		sourceByName[q.Name] = definitions[i]
	}
	if len(human) > MaxQuestions || len(references) > MaxQuestions {
		return nil, nil, ErrInputInvalid
	}
	for name := range human {
		q, ok := byName[name]
		if !ok {
			return nil, nil, ErrOptionUnknown
		}
		if !q.RequiresHuman {
			return nil, nil, ErrInputInvalid
		}
	}
	for name := range references {
		q, ok := byName[name]
		if !ok {
			return nil, nil, ErrOptionUnknown
		}
		if q.Type != "reference" {
			return nil, nil, ErrInputInvalid
		}
	}
	result := map[string]interface{}{}
	pending := []Question{}
	for _, q := range questions {
		value, found := public[q.Name]
		if q.RequiresHuman {
			value, found = human[q.Name]
			if found {
				primitive := q
				primitive.Type = strings.ToLower(q.SourceType)
				switch primitive.Type {
				case "password", "masked", "multiline":
					primitive.Type = "string"
				case "date":
					text, ok := value.(string)
					if !ok {
						return nil, nil, ErrInputInvalid
					}
					if text != "" {
						if _, err := time.Parse("2006-01-02", text); err != nil {
							return nil, nil, ErrInputInvalid
						}
					}
					primitive.Type = "string"
				case "string", "int", "float", "boolean", "enum":
					primitive.Options = append([]string(nil), sourceByName[q.Name].Options...)
				default:
					return nil, nil, ErrUnsupported
				}
				if value != "" || q.Required {
					if !validValue(primitive, value) {
						return nil, nil, ErrInputInvalid
					}
				}
			}
		} else if q.Type == "reference" {
			ref, present := references[q.Name]
			found = present
			value = ref
			if found && (len(ref) > 128 || strings.ContainsAny(ref, "/\\: \t\r\n\x00") || q.Required && ref == "") {
				return nil, nil, ErrInputInvalid
			}
		}
		if q.RequiresHuman || q.Type == "reference" {
			if !found || q.Required && (value == nil || value == "") {
				pending = append(pending, q)
				value = ""
			}
		}
		if !found {
			value = ""
		}
		if text, ok := value.(string); ok && text != "" {
			source := sourceByName[q.Name]
			for _, constraint := range []struct {
				chars  string
				negate bool
			}{{source.ValidChars, true}, {source.InvalidChars, false}} {
				if constraint.chars == "" {
					continue
				}
				expression := "[" + constraint.chars + "]"
				if constraint.negate {
					expression = "[^" + constraint.chars + "]"
				}
				pattern, err := regexp.Compile(expression)
				if err != nil {
					return nil, nil, ErrUnsupported
				}
				if pattern.MatchString(text) {
					return nil, nil, ErrInputInvalid
				}
			}
		}
		result[q.Name] = value
	}
	return result, pending, nil
}

func validValue(q Question, value interface{}) bool {
	switch q.Type {
	case "string", "enum":
		text, ok := value.(string)
		if !ok || strings.ContainsRune(text, '\x00') || units(text) < q.MinLength || units(text) > q.MaxLength {
			return false
		}
		if q.Type == "enum" {
			for _, option := range q.Options {
				if option == text {
					return true
				}
			}
			return false
		}
		return true
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "int", "float":
		var number float64
		switch v := value.(type) {
		case json.Number:
			parsed, err := strconv.ParseFloat(string(v), 64)
			if err != nil {
				return false
			}
			number = parsed
		case float64:
			number = v
		case int:
			number = float64(v)
		case int64:
			number = float64(v)
		default:
			return false
		}
		if math.IsInf(number, 0) || math.IsNaN(number) || q.Type == "int" && math.Trunc(number) != number {
			return false
		}
		if math.Abs(number) > 9007199254740991 {
			return false
		}
		return (q.Min == 0 || number >= float64(q.Min)) && (q.Max == 0 || number <= float64(q.Max))
	}
	return false
}
