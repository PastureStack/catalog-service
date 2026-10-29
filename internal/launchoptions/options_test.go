package launchoptions

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/model"
)

func TestSafeOptionsNeverContainDefaultsAndSensitiveDefaultsNeverMakeAnswersPublic(t *testing.T) {
	private := "QA_PRIVATE_DEFAULT_NEVER_IN_TOOLS"
	input := []model.Question{{Variable: "site_title", Type: "string", Default: private}, {Variable: "db_password", Type: "string", Default: private},
		{Variable: "db_pass", Type: "enum", Options: []string{private}, Default: private}, {Variable: "hidden", Type: "masked", Default: private},
		{Variable: "custom", Type: "unknown-widget", Default: private}}
	questions, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(questions)
	if strings.Contains(string(raw), private) || strings.Contains(string(raw), `"default"`) || strings.Contains(string(raw), `"answer"`) {
		t.Fatal("private source data leaked")
	}
	for _, q := range questions[1:] {
		if !q.RequiresHuman || q.Type != "human" || len(q.Options) != 0 {
			t.Fatal("human dependency hidden by default")
		}
	}
	if input[0].Default != private {
		t.Fatal("source was mutated")
	}
}

func TestTypedPublicAnswersPreserveFalseZeroAndExactEnum(t *testing.T) {
	defs := []model.Question{{Variable: "enabled", Type: "boolean", Required: true}, {Variable: "count", Type: "int", Required: true},
		{Variable: "ratio", Type: "float"}, {Variable: "size", Type: "enum", Options: []string{"small", "large"}}}
	answers, missing, err := ValidatePublic(defs, map[string]interface{}{"enabled": false, "count": json.Number("0"), "ratio": json.Number("0.5"), "size": "small"})
	if err != nil || len(missing) != 0 || answers["enabled"] != false || answers["count"] != json.Number("0") {
		t.Fatal("false or zero treated as missing")
	}
}

func TestHumanAndReferenceQuestionsRejectLiteralModelValuesWithoutEcho(t *testing.T) {
	for _, q := range []model.Question{{Variable: "password", Type: "password"}, {Variable: "custom", Type: "date"}, {Variable: "cert", Type: "certificate"}, {Variable: "service", Type: "service"}, {Variable: "value", Type: "secret"}} {
		_, _, err := ValidatePublic([]model.Question{q}, map[string]interface{}{q.Variable: "QA_PRIVATE_VALUE"})
		if !errors.Is(err, ErrHumanInput) || strings.Contains(err.Error(), "QA_PRIVATE") {
			t.Fatal("raw human/resource answer accepted or echoed")
		}
	}
}

func TestUnknownOptionAndWrongPrimitiveAreClosedBeforeDefaults(t *testing.T) {
	defs := []model.Question{{Variable: "enabled", Type: "boolean", Required: true, Default: "true"}}
	if _, _, err := ValidatePublic(defs, map[string]interface{}{"unknown": "value"}); !errors.Is(err, ErrOptionUnknown) {
		t.Fatal("unknown option accepted")
	}
	if _, _, err := ValidatePublic(defs, map[string]interface{}{"enabled": "false"}); !errors.Is(err, ErrInputInvalid) {
		t.Fatal("coerced boolean")
	}
	_, missing, err := ValidatePublic(defs, nil)
	if err != nil || len(missing) != 1 || missing[0] != "enabled" {
		t.Fatal("default silently filled required public answer")
	}
}

func TestDefinitionsAndValuesHaveFixedBounds(t *testing.T) {
	for _, defs := range [][]model.Question{make([]model.Question, 101), {{Variable: "x", Type: "string"}, {Variable: "x", Type: "string"}}, {{Variable: "__proto__", Type: "string"}}, {{Variable: "bad\nname", Type: "string"}}} {
		if _, err := Build(defs); !errors.Is(err, ErrUnsupported) {
			t.Fatal("invalid question definition accepted")
		}
	}
	for _, value := range []interface{}{math.NaN(), math.Inf(1), 1.5, 9007199254740992.0, map[string]interface{}{"value": 1}} {
		if _, _, err := ValidatePublic([]model.Question{{Variable: "count", Type: "int"}}, map[string]interface{}{"count": value}); !errors.Is(err, ErrInputInvalid) {
			t.Fatal("invalid numeric value accepted")
		}
	}
}

func TestOptionsAreIndependentCopiesAndReferencesContainNoDefaultID(t *testing.T) {
	defs := []model.Question{{Variable: "size", Type: "enum", Options: []string{"small"}}, {Variable: "app_secret", Type: "secret", Default: "raw-existing-id"}}
	result, err := Build(defs)
	if err != nil {
		t.Fatal(err)
	}
	result[0].Options[0] = "changed"
	if defs[0].Options[0] != "small" || result[1].Type != "reference" || result[1].ReferenceType != "secret" {
		t.Fatal("source alias or reference metadata wrong")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "raw-existing-id") {
		t.Fatal("default reference ID exposed")
	}
}
