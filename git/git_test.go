package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestCheckoutCommit(t *testing.T) {
	repo, err := os.MkdirTemp("", "pasturestack-catalog-git-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(repo)

	runGitTestCommand(t, repo, "init")
	runGitTestCommand(t, repo, "config", "user.name", "PastureStack Test")
	runGitTestCommand(t, repo, "config", "user.email", "test@example.invalid")
	file := filepath.Join(repo, "fixture.txt")
	if err := os.WriteFile(file, []byte("first\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", "fixture.txt")
	runGitTestCommand(t, repo, "commit", "-m", "first")
	first := runGitTestCommand(t, repo, "rev-parse", "HEAD")

	if err := os.WriteFile(file, []byte("second\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "commit", "-am", "second")

	if err := CheckoutCommit(repo, first); err != nil {
		t.Fatal(err)
	}
	head, err := HeadCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if head != first {
		t.Fatalf("HEAD %s does not match pinned commit %s", head, first)
	}
}
