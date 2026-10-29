package parse

import "testing"

func TestQuestionsFromFilesKeepsOriginalOwnerPrecedenceAndDefaultsPrivate(t *testing.T) {
	files := map[string]string{
		"template-version.yml": "questions:\n- variable: preferred\n  type: password\n  default: PRIVATE_DEFAULT\n",
		"compose.yml":          "catalog:\n  questions:\n  - variable: compose\n    type: string\n",
		"rancher-compose.yml":  ".catalog:\n  questions:\n  - variable: legacy\n    type: boolean\n",
	}
	for _, name := range []string{"preferred", "compose", "legacy"} {
		questions, err := QuestionsFromFiles(files)
		if err != nil || len(questions) != 1 || questions[0].Variable != name {
			t.Fatalf("wrong question precedence: %s", name)
		}
		switch name {
		case "preferred":
			if questions[0].Default != "PRIVATE_DEFAULT" {
				t.Fatal("private source semantics changed")
			}
			delete(files, "template-version.yml")
		case "compose":
			delete(files, "compose.yml")
		}
	}
}

func TestMalformedSelectedQuestionSourceNeverFallsBack(t *testing.T) {
	if _, err := QuestionsFromFiles(map[string]string{"template-version.yml": "questions: [", "rancher-compose.yml": ".catalog: {}"}); err == nil {
		t.Fatal("malformed primary source fell back")
	}
	questions, err := QuestionsFromFiles(nil)
	if err != nil || questions != nil {
		t.Fatal("empty version changed behavior")
	}
}
