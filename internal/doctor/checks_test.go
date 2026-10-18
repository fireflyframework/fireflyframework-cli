package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fireflyframework/fireflyframework-cli/internal/config"
)

func fakeCommand(t *testing.T, name, output string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	dir := t.TempDir()
	body := "#!/bin/sh\nprintf '%s\\n' '" + strings.ReplaceAll(output, "'", "'\\''") + "'\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestJavaRejectsOlderJDKWithLegacyConfiguration(t *testing.T) {
	for _, version := range []int{21, 22, 23, 24} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			fakeCommand(t, "java", fmt.Sprintf("openjdk %d.0.1 2026-01-01", version))
			got := checkJava(&config.Config{JavaVersion: "21"})
			if got.Status != "fail" || !strings.Contains(got.Detail, "need 25+") {
				t.Fatalf("unsupported Java %d reported %s: %s", version, got.Status, got.Detail)
			}
		})
	}
}

func TestJavaAcceptsSupportedJDK(t *testing.T) {
	for _, version := range []int{25, 26} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			fakeCommand(t, "java", fmt.Sprintf("openjdk %d.0.1 2026-01-01", version))
			got := checkJava(&config.Config{JavaVersion: "21"})
			if got.Status != "pass" || got.Detail != fmt.Sprintf("Java %d", version) {
				t.Fatalf("supported JDK reported %+v", got)
			}
		})
	}
}

func TestJavaBelowConfiguredPreferenceRemainsCompatible(t *testing.T) {
	fakeCommand(t, "java", "openjdk 25.0.1 2026-01-01")
	got := checkJava(&config.Config{JavaVersion: "26"})
	if got.Status != "warn" || !strings.Contains(got.Detail, "compatible") {
		t.Fatalf("supported JDK below preference reported %+v", got)
	}
}

func TestMavenRejectsUnsupportedRuntime(t *testing.T) {
	for _, output := range []string{"Apache Maven 3.9.16\nJava version: 21.0.1", "Apache Maven 3.9.16\njava version \"24.0.1\""} {
		t.Run(output, func(t *testing.T) {
			fakeCommand(t, "mvn", output)
			got := checkMavenJava()
			if got.Status != "fail" || !strings.Contains(got.Detail, "need 25+") {
				t.Fatalf("unsupported Maven JDK reported %+v", got)
			}
		})
	}
}

func TestMavenReportsSupportedAndUnknownRuntime(t *testing.T) {
	for _, tc := range []struct{ output, status, detail string }{
		{"Apache Maven 3.9.16\nJava version: 25.0.1", "pass", "Java 25"},
		{"Apache Maven 3.9.16\njava version \"26.0.1\"", "pass", "Java 26"},
		{"Apache Maven 3.9.16", "warn", "could not parse Java version"},
	} {
		t.Run(tc.output, func(t *testing.T) {
			fakeCommand(t, "mvn", tc.output)
			got := checkMavenJava()
			if got.Status != tc.status || got.Detail != tc.detail {
				t.Fatalf("Maven runtime reported %+v, want %s/%s", got, tc.status, tc.detail)
			}
		})
	}
}

func TestGlobalDoctorRequiresConfiguredParentAndBOMTrain(t *testing.T) {
	for _, tc := range []struct{ name, installed, want string }{
		{"current train", "26.10.17-SNAPSHOT", "pass"},
		{"legacy artifact only", "1.0.0-SNAPSHOT", "fail"},
		{"different train only", "26.05.08", "fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeCommand(t, "java", "openjdk 25.0.1")
			fakeCommand(t, "mvn", "Apache Maven 3.9.16\nJava version: 25.0.1")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("FLYWORK_HOME", filepath.Join(home, "flywork"))
			t.Setenv("JAVA_HOME", filepath.Join(home, "jdk"))
			for _, artifact := range []string{"fireflyframework-parent", "fireflyframework-bom"} {
				path := filepath.Join(home, ".m2", "repository", "org", "fireflyframework", artifact, tc.installed, artifact+"-"+tc.installed+".pom")
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("<project/>"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.Config{JavaVersion: "25", ParentVersion: "26.10.17-SNAPSHOT", ReposPath: filepath.Join(home, "repos")}
			found := 0
			for _, got := range RunGlobal(cfg) {
				if got.Name != "Parent POM in .m2" && got.Name != "BOM in .m2" {
					continue
				}
				found++
				if got.Status != tc.want || !strings.Contains(got.Detail, "26.10.17-SNAPSHOT") {
					t.Errorf("%s with %s installed: %+v, want %s for configured train", got.Name, tc.installed, got, tc.want)
				}
			}
			if found != 2 {
				t.Fatalf("got %d parent/BOM checks, want 2", found)
			}
		})
	}
}
