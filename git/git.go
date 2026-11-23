package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/PastureStack/catalog-service/outbound"
	log "github.com/sirupsen/logrus"
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

const commandTimeout = 2 * time.Minute

func Clone(path string, source outbound.AuthorizedSource, branch string) error {
	if err := validateBranch(branch); err != nil {
		return err
	}
	args := append(protocolArguments(source), "clone", "-b", branch, "--single-branch", "--", source.String(), path)
	return runcmd("git", args...)
}

func Update(path string, source outbound.AuthorizedSource, branch string) error {
	if err := validateBranch(branch); err != nil {
		return err
	}
	remoteRef := fmt.Sprintf("refs/remotes/origin/%s", branch)
	refspec := fmt.Sprintf("+refs/heads/%s:%s", branch, remoteRef)
	args := append([]string{"-C", path}, protocolArguments(source)...)
	args = append(args, "fetch", "--no-tags", "--", source.String(), refspec)
	if err := runcmd("git", args...); err != nil {
		return err
	}
	return runcmd("git", "-C", path, "checkout", "--detach", remoteRef)
}

func CheckoutCommit(path, commit string) error {
	if !fullCommitSHA.MatchString(commit) {
		return fmt.Errorf("Git catalog commit must be a full 40-character SHA")
	}
	if err := runcmd("git", "-C", path, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return err
	}
	return runcmd("git", "-C", path, "checkout", "--detach", commit)
}

func HeadCommit(path string) (string, error) {
	output, err := runCommand(context.Background(), commandTimeout, "git", "-C", path, "rev-parse", "HEAD")
	return strings.TrimSpace(string(output)), err
}

// CloneBundle is only called with an operator-supplied, SHA256-verified bundle.
// It cannot fetch an external source. The pin must be explicitly advertised;
// the bundle's ref name need not be the catalog's upstream branch name.
func CloneBundle(path, bundle, branch, commit string) error {
	if err := validateBranch(branch); err != nil {
		return err
	}
	if !fullCommitSHA.MatchString(commit) {
		return fmt.Errorf("Git catalog commit must be a full 40-character SHA")
	}
	if err := runcmd("git", "init", "--", path); err != nil {
		return err
	}
	output, err := runCommand(context.Background(), commandTimeout, "git", "-C", path, "bundle", "list-heads", bundle)
	if err != nil {
		return err
	}
	ref := ""
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.EqualFold(fields[0], commit) && strings.HasPrefix(fields[1], "refs/heads/") {
			ref = fields[1]
			break
		}
	}
	if ref == "" {
		return fmt.Errorf("Bundled catalog does not advertise the pinned commit")
	}
	if err := runcmd("git", "-C", path, "-c", "protocol.allow=never", "-c", "protocol.file.allow=always",
		"fetch", "--no-tags", "--", bundle, ref+":refs/remotes/bundled/source"); err != nil {
		return err
	}
	return CheckoutCommit(path, commit)
}

// IsClean checks tracked modifications and untracked files; either can otherwise
// make the catalog index disagree with the commit recorded in the database.
func IsClean(path string) (bool, error) {
	output, err := runCommand(context.Background(), commandTimeout, "git", "-C", path, "status", "--porcelain", "--untracked-files=all", "--ignored")
	if err != nil || len(strings.TrimSpace(string(output))) != 0 {
		return false, err
	}
	// Index flags must not hide modified tracked files from the status check.
	output, err = runCommand(context.Background(), commandTimeout, "git", "-C", path, "ls-files", "-v", "-z")
	if err != nil {
		return false, err
	}
	for _, entry := range strings.Split(string(output), "\x00") {
		if entry != "" && !strings.HasPrefix(entry, "H ") {
			return false, nil
		}
	}
	return true, nil
}

func IsValid(source outbound.AuthorizedSource) bool {
	args := append(protocolArguments(source), "ls-remote", "--", source.String())
	err := runcmd("git", args...)
	return (err == nil)
}

func validateBranch(branch string) error {
	if strings.TrimSpace(branch) != branch || branch == "" {
		return fmt.Errorf("Git catalog branch is invalid")
	}
	if err := runcmd("git", "check-ref-format", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("Git catalog branch %q is invalid", branch)
	}
	return nil
}

func protocolArguments(source outbound.AuthorizedSource) []string {
	arguments := []string{
		"-c", "protocol.allow=never",
		"-c", "credential.helper=",
		"-c", "http.followRedirects=false",
	}
	switch {
	case source.IsLocal():
		arguments = append(arguments, "-c", "protocol.file.allow=always")
	case strings.HasPrefix(strings.ToLower(source.String()), "https://"):
		arguments = append(arguments, "-c", "protocol.https.allow=always")
	case strings.HasPrefix(strings.ToLower(source.String()), "http://"):
		arguments = append(arguments, "-c", "protocol.http.allow=always")
	}
	return arguments
}

func runcmd(name string, arg ...string) error {
	_, err := runCommand(context.Background(), commandTimeout, name, arg...)
	return err
}

func runCommand(parent context.Context, timeout time.Duration, name string, arg ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := gitCommand(ctx, name, arg...)
	cmd.WaitDelay = time.Second
	if log.GetLevel() >= log.DebugLevel {
		cmd.Stderr = os.Stderr
	}
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("Git command did not complete: %w", ctx.Err())
	}
	return output, err
}

func gitCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	environment := make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		key := entry
		if separator := strings.IndexByte(entry, '='); separator >= 0 {
			key = entry[:separator]
		}
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") || strings.EqualFold(key, "SSH_ASKPASS") {
			continue
		}
		environment = append(environment, entry)
	}
	cmd.Env = append(environment,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0="+os.DevNull,
	)
	return cmd
}
