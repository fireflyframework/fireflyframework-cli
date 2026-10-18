package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Catch version updates omitting nested monorepo modules or changing old checkouts.
func TestBumpJaflyNestedPoms(t *testing.T) {
	root := t.TempDir()
	paths := []string{"pom.xml", "build/parent/pom.xml", "build/bom/pom.xml", "modules/plugins/plugin-api/pom.xml", "modules/agentic-bridge/bridge-core/pom.xml"}
	for _, p := range append(paths, "target/generated/pom.xml") {
		path := filepath.Join(root, "fireflyframework-jafly", p)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<project><version>26.10.17-SNAPSHOT</version></project>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(root, "fireflyframework-jafly")
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	script := `import pathlib, sys
root = pathlib.Path(__file__).resolve().parents[1]
for p in root.rglob('pom.xml'):
    if 'target' not in p.relative_to(root).parts:
        p.write_text(p.read_text().replace('26.10.17-SNAPSHOT', sys.argv[1]))
`
	if err := os.WriteFile(filepath.Join(repo, "scripts", "set_version.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "fireflyframework-parent", "pom.xml")
	if err := os.MkdirAll(filepath.Dir(old), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	results, err := BumpAll(BumpOptions{ReposDir: root, OldVersion: "26.10.17-SNAPSHOT", NewVersion: "26.10.17"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Error != nil || results[0].Updated != 5 {
		t.Fatalf("bump results = %#v", results)
	}
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(root, "fireflyframework-jafly", p))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "SNAPSHOT") {
			t.Errorf("unbumped: %s", p)
		}
	}
	b, _ := os.ReadFile(old)
	if string(b) != "legacy" {
		t.Fatal("legacy checkout changed")
	}
	b, _ = os.ReadFile(filepath.Join(root, "fireflyframework-jafly", "target/generated/pom.xml"))
	if !strings.Contains(string(b), "SNAPSHOT") {
		t.Fatal("target directory changed")
	}
}

// Catch using a parent's version instead of a module's explicit version.
func TestReadPomUsesOwnVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pom.xml")
	if err := os.WriteFile(p, []byte("<project><parent><version>3.5.10</version></parent><version>26.10.17</version></project>"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPomVersion(p)
	if err != nil || got != "26.10.17" {
		t.Fatalf("version = %q, %v", got, err)
	}
}

// Catch a monorepo root masking a stale nested module version.
func TestCheckJaflyFindsNestedVersionDrift(t *testing.T) {
	root := t.TempDir()
	for p, v := range map[string]string{"pom.xml": "26.10.17", "build/parent/pom.xml": "26.10.17", "modules/cache/pom.xml": "26.09.01"} {
		path := filepath.Join(root, "fireflyframework-jafly", p)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<project><groupId>org.fireflyframework</groupId><version>"+v+"</version></project>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := CheckAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Consistent || got.TotalRepos != 1 || got.UniqueVersions["26.09.01"] != 1 {
		t.Fatalf("version report = %#v", got)
	}
}

// Catch bypassing the monorepo's semantic version tool and replacing library versions.
func TestBumpJaflyUsesReleaseVersionTool(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "fireflyframework-jafly")
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	pom := `<project><version>26.10.17-SNAPSHOT</version><dependencies><dependency><groupId>example.external</groupId><artifactId>external</artifactId><version>26.10.17-SNAPSHOT</version></dependency></dependencies></project>`
	if err := os.WriteFile(filepath.Join(repo, "pom.xml"), []byte(pom), 0644); err != nil {
		t.Fatal(err)
	}
	script := `import pathlib, sys
p = pathlib.Path(__file__).resolve().parents[1] / 'pom.xml'
p.write_text(p.read_text().replace('<project><version>26.10.17-SNAPSHOT</version>', '<project><version>' + sys.argv[1] + '</version>'))
`
	if err := os.WriteFile(filepath.Join(repo, "scripts", "set_version.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	results, err := BumpAll(BumpOptions{ReposDir: root, OldVersion: "26.10.17-SNAPSHOT", NewVersion: "26.10.17"}, nil)
	if err != nil || len(results) != 1 || results[0].Error != nil {
		t.Fatalf("bump = %#v, %v", results, err)
	}
	b, err := os.ReadFile(filepath.Join(repo, "pom.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<artifactId>external</artifactId><version>26.10.17-SNAPSHOT</version>") {
		t.Fatalf("external dependency changed: %s", b)
	}
}

// Catch silently falling back to global replacement in an incomplete checkout.
func TestBumpJaflyRequiresVersionTool(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "fireflyframework-jafly")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(repo, "pom.xml")
	original := "<project><version>26.10.17-SNAPSHOT</version></project>"
	if err := os.WriteFile(p, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := BumpAll(BumpOptions{ReposDir: root, OldVersion: "26.10.17-SNAPSHOT", NewVersion: "26.10.17"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Error == nil {
		t.Fatalf("expected missing version tool failure, got %#v", got)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != original {
		t.Fatalf("POM changed after failure: %s %v", b, err)
	}
}

func TestCheckJaflyPreservesConsumerProjectVersions(t *testing.T) {
	for _, fixture := range []struct {
		name, extra string
		consistent  bool
	}{
		{"independent-consumer", "", true},
		{"stale-consumer-dependency", "<dependencies><dependency><groupId>org.fireflyframework</groupId><artifactId>fireflyframework-cqrs</artifactId><version>26.09.01</version></dependency></dependencies>", false},
		{"stale-consumer-framework-property", "<properties><fireflyframework.version>26.09.01</fireflyframework.version></properties><dependencies><dependency><groupId>org.fireflyframework</groupId><artifactId>fireflyframework-cqrs</artifactId><version>${fireflyframework.version}</version></dependency></dependencies>", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			repo := filepath.Join(root, "fireflyframework-jafly")
			files := map[string]string{
				"pom.xml":                "<project><groupId>org.fireflyframework</groupId><artifactId>fireflyframework-jafly</artifactId><version>26.10.17</version></project>",
				"modules/kernel/pom.xml": "<project><parent><groupId>org.fireflyframework</groupId><artifactId>fireflyframework-parent</artifactId><version>26.10.17</version></parent><artifactId>kernel</artifactId></project>",
				"docs/examples/pom.xml":  "<project><parent><groupId>org.fireflyframework</groupId><artifactId>fireflyframework-parent</artifactId><version>26.10.17</version></parent><groupId>com.example</groupId><artifactId>hello</artifactId><version>0.1.0-SNAPSHOT</version>" + fixture.extra + "</project>",
			}
			for path, content := range files {
				p := filepath.Join(repo, path)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			report, err := CheckAll(root)
			if err != nil {
				t.Fatal(err)
			}
			if report.Consistent != fixture.consistent || report.UniqueVersions["0.1.0-SNAPSHOT"] != 0 {
				t.Fatalf("consumer version was treated as framework train: %#v", report)
			}
		})
	}
}
