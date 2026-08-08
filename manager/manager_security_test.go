package manager

import (
	"errors"
	"strings"
	"testing"
)

func TestRepoRefreshErrorDoesNotExposeSourceDetails(t *testing.T) {
	err := (&RepoRefreshError{Errors: []error{
		errors.New("https://private.example/token\r\nforged"),
	}}).Error()
	if err != "catalog refresh failed for 1 source(s)" {
		t.Fatalf("unexpected refresh error: %q", err)
	}
	if strings.Contains(err, "private.example") || strings.ContainsAny(err, "\r\n") {
		t.Fatalf("refresh error exposed source details: %q", err)
	}
}
