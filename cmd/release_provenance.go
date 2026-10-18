package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/fireflyframework/fireflyframework-cli/internal/git"
)

var releaseCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func resolveReleaseTagCommit(repository, tag string, readAPI func(string) ([]byte, error)) (string, error) {
	endpoint := "/repos/" + repository + "/git/ref/tags/" + tag
	for attempt := 0; attempt < 10; attempt++ {
		data, err := readAPI(endpoint)
		if err != nil {
			return "", fmt.Errorf("resolve remote release tag: %w", err)
		}
		var object struct {
			Object struct {
				Type string `json:"type"`
				SHA  string `json:"sha"`
			} `json:"object"`
		}
		if err := json.Unmarshal(data, &object); err != nil {
			return "", err
		}
		if !releaseCommitPattern.MatchString(object.Object.SHA) {
			return "", fmt.Errorf("invalid remote release object SHA")
		}
		switch object.Object.Type {
		case "commit":
			return object.Object.SHA, nil
		case "tag":
			endpoint = "/repos/" + repository + "/git/tags/" + object.Object.SHA
		default:
			return "", fmt.Errorf("release tag points to unsupported object type %q", object.Object.Type)
		}
	}
	return "", fmt.Errorf("release tag annotation chain is too deep")
}

func validateReleaseCommit(evidence []byte, expected string) error {
	if !releaseCommitPattern.MatchString(expected) || strings.TrimSpace(string(evidence)) != expected {
		return fmt.Errorf("release-commit.txt does not match the remote release tag commit")
	}
	return nil
}

func cloneReleaseSource(repositoryURL, tag, expectedSHA, directory string) error {
	command := exec.Command("git", "clone", "--depth", "1", "--single-branch", "--branch", tag,
		"--", repositoryURL, directory)
	if out, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("clone exact tagged release: %w: %s", err, strings.TrimSpace(string(out)))
	}
	sha, err := git.HeadSHA(directory)
	if err != nil {
		return err
	}
	if sha != expectedSHA {
		return fmt.Errorf("cloned release tag commit %s differs from verified commit %s", sha, expectedSHA)
	}
	return nil
}
