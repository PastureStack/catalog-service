package git

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/PastureStack/catalog-service/outbound"
	log "github.com/sirupsen/logrus"
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

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
	cmd := gitCommand("git", "-C", path, "rev-parse", "HEAD")
	output, err := cmd.Output()
	return strings.Trim(string(output), "\n"), err
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
	cmd := gitCommand("git", "check-ref-format", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
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
	cmd := gitCommand(name, arg...)
	if log.GetLevel() >= log.DebugLevel {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func gitCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
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
