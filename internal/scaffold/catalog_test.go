package scaffold

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldsUsePublishedFrameworkCoordinates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	catalog := map[string]bool{
		"fireflyframework-parent": true, "fireflyframework-bom": true,
		"fireflyframework-starter-application": true, "fireflyframework-starter-core": true,
		"fireflyframework-starter-domain": true, "fireflyframework-web": true,
		"fireflyframework-utils": true, "fireflyframework-validators": true,
		"fireflyframework-r2dbc": true, "fireflyframework-cqrs": true,
		"fireflyframework-orchestration": true, "fireflyframework-eda": true,
		"fireflyframework-observability": true,
	}
	for _, name := range []string{"application", "core", "domain", "library"} {
		t.Run(name, func(t *testing.T) {
			arch, err := LoadArchetype(name)
			if err != nil {
				t.Fatal(err)
			}
			out := t.TempDir()
			ctx := &ProjectContext{
				ProjectName: "Orders", ArtifactId: "orders", GroupId: "com.example",
				BasePackage: "com.example.orders", PackagePath: "com/example/orders",
				Version: "1.0.0-SNAPSHOT", ParentVersion: "26.10.17-SNAPSHOT", JavaVersion: "25",
				ParentGroupId: arch.Parent.GroupID, ParentArtifactId: arch.Parent.ArtifactID,
				ModulePrefix: "Orders", ApplicationClassName: "Application",
			}
			if err := Generate(arch, ctx, out); err != nil {
				t.Fatal(err)
			}
			if err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || entry.Name() != "pom.xml" {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				var pom struct {
					Dependencies []struct {
						GroupID    string `xml:"groupId"`
						ArtifactID string `xml:"artifactId"`
					} `xml:"dependencies>dependency"`
				}
				if err := xml.Unmarshal(data, &pom); err != nil {
					return err
				}
				for _, dependency := range pom.Dependencies {
					if dependency.GroupID == "org.fireflyframework" && !catalog[dependency.ArtifactID] {
						t.Errorf("%s references unpublished framework artifact %s", path, dependency.ArtifactID)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBoot4ArchetypesKeepWebAutoConfigurationEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	for _, name := range []string{"application", "core", "domain"} {
		arch, err := LoadArchetype(name)
		if err != nil {
			t.Fatal(err)
		}
		annotations := append([]string{}, arch.Annotations...)
		for _, module := range arch.Modules {
			annotations = append(annotations, module.Annotations...)
		}
		for _, annotation := range annotations {
			if annotation == "@EnableWebFlux" {
				t.Errorf("%s disables Boot's HTTP codec auto-configuration", name)
			}
		}
	}
}
