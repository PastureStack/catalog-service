package launchoptions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/model"
)

func TestPrivateLaunchValidationSeparatesHumanReferenceAndExplicitNoDefault(t *testing.T) {
	definitions := []model.Question{{Variable: "enabled", Type: "boolean", Required: true}, {Variable: "count", Type: "int", Required: true},
		{Variable: "optional", Type: "string", Default: "never-use-default"}, {Variable: "password", Type: "password", Default: "private-default"},
		{Variable: "target", Type: "service", Default: "1s999"}}
	public := map[string]interface{}{"enabled": false, "count": json.Number("0")}
	values, missing, err := ValidateLaunch(definitions, public, nil, nil)
	if err != nil || len(missing) != 2 || len(values) != 5 || values["optional"] != "" || values["password"] != "" {
		t.Fatal("default filled or missing human dependency hidden")
	}
	raw, _ := json.Marshal(missing)
	if strings.Contains(string(raw), "private-default") || strings.Contains(string(raw), "1s999") {
		t.Fatal("missing-input metadata leaked default")
	}
	values, missing, err = ValidateLaunch(definitions, public, map[string]interface{}{"password": "human-answer"}, map[string]string{"target": "1s7"})
	if err != nil || len(missing) != 0 || values["password"] != "human-answer" || values["target"] != "1s7" || values["enabled"] != false {
		t.Fatal("explicit inputs changed")
	}
	for _, input := range []struct {
		safe, human map[string]interface{}
		refs        map[string]string
	}{
		{map[string]interface{}{"enabled": false, "count": 0, "password": "wrong-channel"}, nil, nil},
		{public, map[string]interface{}{"enabled": true}, nil},
		{public, nil, map[string]string{"password": "1se2"}},
		{public, nil, map[string]string{"target": "http://raw-id"}},
	} {
		if _, _, err := ValidateLaunch(definitions, input.safe, input.human, input.refs); err == nil {
			t.Fatal("channel confusion accepted")
		}
	}
}

func TestPrivateLaunchValidationEnforcesOriginalQuestionConstraintsWithoutEcho(t *testing.T) {
	for _, example := range []struct {
		q     model.Question
		value interface{}
	}{
		{model.Question{Variable: "code", Type: "string", ValidChars: "a-z"}, "123-private"},
		{model.Question{Variable: "password", Type: "password", MinLength: 3}, "x"},
		{model.Question{Variable: "token_count", Type: "int", Min: 2, Max: 5}, json.Number("1")},
		{model.Question{Variable: "when", Type: "date"}, "not-a-date-private"},
	} {
		safe, human := map[string]interface{}{}, map[string]interface{}{}
		built, _ := Build([]model.Question{example.q})
		if built[0].RequiresHuman {
			human[example.q.Variable] = example.value
		} else {
			safe[example.q.Variable] = example.value
		}
		if _, _, err := ValidateLaunch([]model.Question{example.q}, safe, human, nil); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("constraint bypass or private error echo")
		}
	}
}
