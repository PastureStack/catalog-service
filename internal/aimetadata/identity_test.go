package aimetadata

import (
	"strings"
	"testing"
)

func TestMetadataLogicalIdentityBindsScopeSourceBranchPinAndTemplateLocation(t *testing.T) {
	base := identity{Scope: "1a5", Catalog: "same-name", Repo: "https://example.test/team/repo.git", Branch: "main", Folder: "web", Base: "cattle"}
	id, err := logicalID(base)
	if err != nil || !metadataID.MatchString(id) {
		t.Fatal("logical ID invalid")
	}
	for _, change := range []func(*identity){func(v *identity) { v.Scope = "1a6" }, func(v *identity) { v.Scope = "global" }, func(v *identity) { v.Repo = "https://example.test/other/repo.git" }, func(v *identity) { v.Branch = "other" }, func(v *identity) { v.Pin = strings.Repeat("a", 40) }, func(v *identity) { v.Folder = "other" }, func(v *identity) { v.Base = "k8s" }, func(v *identity) { v.Catalog = "other" }} {
		changed := base
		change(&changed)
		next, err := logicalID(changed)
		if err != nil || next == id {
			t.Fatal("logical identity aliased another source/scope/path")
		}
	}
}

func TestMetadataCredentialRotationAndQuerySecretsDoNotEnterIdentity(t *testing.T) {
	base := identity{Scope: "1a5", Catalog: "repo", Repo: "https://example.test/team/repo.git", Branch: "main", Folder: "web"}
	id, _ := logicalID(base)
	for _, repo := range []string{"https://PRIVATE_USER:PRIVATE_PASSWORD@example.test/team/repo.git?token=PRIVATE_TOKEN#PRIVATE_FRAGMENT", "https://ROTATED_USER:ROTATED_PASSWORD@example.test/team/repo.git?token=ROTATED_TOKEN"} {
		changed := base
		changed.Repo = repo
		got, err := logicalID(changed)
		if err != nil || got != id {
			t.Fatal("credential rotation changed identity or secrets were hashed")
		}
	}
}

func TestMetadataCursorIsBoundToQueryAndProjectWithoutGrantingRowIdentity(t *testing.T) {
	id := template(1).ID
	value := cursor("1a5", "web", 123, id)
	row, got, valid := parseCursor(value, "1a5", "web")
	if !valid || row != 123 || got != id || len(value) > 128 {
		t.Fatal("private cursor roundtrip invalid")
	}
	for _, pair := range [][2]string{{"1a6", "web"}, {"1a5", "other"}} {
		if _, _, valid := parseCursor(value, pair[0], pair[1]); valid {
			t.Fatal("foreign cursor context accepted")
		}
	}
	if _, _, valid := parseCursor("ctm-123", "1a5", "web"); valid {
		t.Fatal("public row-id cursor fallback accepted")
	}
}
