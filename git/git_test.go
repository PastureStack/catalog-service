package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/outbound"
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

func TestCloneAuthorizedLocalSource(t *testing.T) {
	source := t.TempDir()
	runGitTestCommand(t, source, "init")
	runGitTestCommand(t, source, "config", "user.name", "PastureStack Test")
	runGitTestCommand(t, source, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "fixture.txt"), []byte("safe\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, source, "add", "fixture.txt")
	runGitTestCommand(t, source, "commit", "-m", "fixture")
	branch := runGitTestCommand(t, source, "branch", "--show-current")

	policy, err := outbound.NewSourcePolicy(nil, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := policy.AuthorizeGitSource(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "clone")
	if err := Clone(destination, authorized, branch); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(destination, "fixture.txt")); err != nil || strings.ReplaceAll(string(contents), "\r\n", "\n") != "safe\n" {
		t.Fatalf("cloned fixture = %q, %v", contents, err)
	}
}

func TestUpdateFetchesOnlyTheAuthorizedSourceAndBranch(t *testing.T) {
	source := t.TempDir()
	runGitTestCommand(t, source, "init", "-b", "main")
	runGitTestCommand(t, source, "config", "user.name", "PastureStack Test")
	runGitTestCommand(t, source, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "fixture.txt"), []byte("first\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, source, "add", "fixture.txt")
	runGitTestCommand(t, source, "commit", "-m", "first")

	policy, err := outbound.NewSourcePolicy(nil, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := policy.AuthorizeGitSource(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "clone")
	if err := Clone(destination, authorized, "main"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(source, "fixture.txt"), []byte("second\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, source, "commit", "-am", "second")
	attacker := t.TempDir()
	runGitTestCommand(t, attacker, "init", "-b", "main")
	runGitTestCommand(t, destination, "remote", "set-url", "origin", attacker)

	if err := Update(destination, authorized, "main"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(destination, "fixture.txt"))
	if err != nil || strings.ReplaceAll(string(contents), "\r\n", "\n") != "second\n" {
		t.Fatalf("updated fixture = %q, %v", contents, err)
	}
}

func TestCloneRejectsInvalidBranch(t *testing.T) {
	policy, err := outbound.NewSourcePolicy([]string{"https://github.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := policy.AuthorizeGitSource("https://github.com/PastureStack/catalog-templates.git")
	if err != nil {
		t.Fatal(err)
	}
	if err := Clone(filepath.Join(t.TempDir(), "clone"), source, "../unsafe"); err == nil {
		t.Fatal("invalid branch was accepted")
	}
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

func TestCheckoutCommitRejectsUntrustedRevision(t *testing.T) {
	if err := CheckoutCommit(t.TempDir(), "--help"); err == nil {
		t.Fatal("option-like commit revision was accepted")
	}
	if err := CheckoutCommit(t.TempDir(), "HEAD"); err == nil {
		t.Fatal("symbolic commit revision was accepted")
	}
}
