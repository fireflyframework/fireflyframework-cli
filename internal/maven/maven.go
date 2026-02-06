// Copyright 2024-2026 Firefly Software Solutions Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package maven

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// IsInstalled checks if mvn is available on PATH.
func IsInstalled() bool {
	_, err := exec.LookPath("mvn")
	return err == nil
}

// Version returns the Maven version string (e.g., "3.9.6").
func Version() (string, error) {
	out, err := exec.Command("mvn", "--version").Output()
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`Apache Maven (\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) >= 2 {
		return matches[1], nil
	}
	return strings.TrimSpace(string(out)), nil
}

// Install runs mvn clean install in the given directory.
// If skipTests is true, -DskipTests is appended.
func Install(dir string, skipTests bool) error {
	cmd := exec.Command("mvn", buildInstallArgs(skipTests)...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InstallQuiet runs mvn clean install silently.
func InstallQuiet(dir string, skipTests bool) error {
	cmd := exec.Command("mvn", buildInstallArgs(skipTests)...)
	cmd.Dir = dir
	return cmd.Run()
}

// InstallWithJava runs mvn clean install with a specific JAVA_HOME.
func InstallWithJava(dir, javaHome string, skipTests bool) error {
	cmd := exec.Command("mvn", buildInstallArgs(skipTests)...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if javaHome != "" {
		cmd.Env = appendJavaHome(os.Environ(), javaHome)
	}
	return cmd.Run()
}

// InstallQuietWithJava runs mvn clean install silently with a specific JAVA_HOME.
func InstallQuietWithJava(dir, javaHome string, skipTests bool) error {
	cmd := exec.Command("mvn", buildInstallArgs(skipTests)...)
	cmd.Dir = dir
	if javaHome != "" {
		cmd.Env = appendJavaHome(os.Environ(), javaHome)
	}
	return cmd.Run()
}

// buildInstallArgs returns the Maven arguments for clean install.
func buildInstallArgs(skipTests bool) []string {
	args := []string{"clean", "install", "-q"}
	if skipTests {
		args = append(args, "-DskipTests")
	}
	return args
}

func appendJavaHome(env []string, javaHome string) []string {
	filtered := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, "JAVA_HOME=") {
			filtered = append(filtered, e)
		}
	}
	return append(filtered, "JAVA_HOME="+javaHome)
}

// ArtifactExistsInM2 checks if a given artifact exists in the local .m2 repository.
func ArtifactExistsInM2(groupID, artifactID, version string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	groupPath := strings.ReplaceAll(groupID, ".", string(filepath.Separator))
	pomPath := filepath.Join(home, ".m2", "repository", groupPath, artifactID, version, artifactID+"-"+version+".pom")
	_, err = os.Stat(pomPath)
	return err == nil
}
