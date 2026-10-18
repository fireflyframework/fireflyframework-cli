package scaffold

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

// Catch generating a client-only adapter with the obsolete server-side dependency.
func TestDomainScaffoldUsesBoot4WebClientStarter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	arch, err := LoadArchetype("domain")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	ctx := &ProjectContext{ProjectName: "Orders", ArtifactId: "orders", GroupId: "com.example", BasePackage: "com.example.orders", PackagePath: "com/example/orders", Version: "1.0.0-SNAPSHOT", ParentVersion: "26.10.17-SNAPSHOT", JavaVersion: "25", ModulePrefix: "Orders", ApplicationClassName: "Application"}
	if err := Generate(arch, ctx, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "orders-infra", "pom.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var pom struct {
		Dependencies []struct {
			ArtifactID string `xml:"artifactId"`
		} `xml:"dependencies>dependency"`
	}
	if err := xml.Unmarshal(b, &pom); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range pom.Dependencies {
		if d.ArtifactID == "spring-boot-starter-webclient" {
			found = true
		}
	}
	if !found {
		t.Fatal("generated client module has no Boot 4 reactive-client starter")
	}
}

// Catch omitting Boot 4's reactive web testing infrastructure from new services.
func TestApplicationScaffoldIncludesReactiveWebTestSupport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	arch, err := LoadArchetype("application")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	ctx := &ProjectContext{ProjectName: "Orders", ArtifactId: "orders", GroupId: "com.example", BasePackage: "com.example.orders", PackagePath: "com/example/orders", Version: "1.0.0-SNAPSHOT", ParentVersion: "26.10.17-SNAPSHOT", JavaVersion: "25", ApplicationClassName: "Application"}
	if err := Generate(arch, ctx, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "pom.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var pom struct {
		Dependencies []struct {
			ArtifactID string `xml:"artifactId"`
			Scope      string `xml:"scope"`
		} `xml:"dependencies>dependency"`
	}
	if err := xml.Unmarshal(b, &pom); err != nil {
		t.Fatal(err)
	}
	for _, d := range pom.Dependencies {
		if d.ArtifactID == "spring-boot-starter-webflux-test" && d.Scope == "test" {
			return
		}
	}
	t.Fatal("generated application has no Boot 4 reactive-web test starter")
}
