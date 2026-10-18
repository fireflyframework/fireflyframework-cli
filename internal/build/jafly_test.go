package build

import (
	"os"
	"path/filepath"
	"testing"
)

// Catch dispatching old module repositories instead of building one reactor.
func TestJaflyBuildPlansOneReactor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLYWORK_HOME", "")
	for _, path := range []string{"fireflyframework-jafly/pom.xml", "fireflyframework-jafly/build/parent/pom.xml", "fireflyframework-jafly/modules/cache/pom.xml", "fireflyframework-kernel/pom.xml"} {
		p := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("<project/>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	results, layers, err := RunDAGBuild(BuildOptions{ReposDir: root, ForceAll: true, DryRun: true}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Repo != "fireflyframework-jafly" {
		t.Fatalf("build plan = %#v; want one jafly checkout", results)
	}
	if len(layers) != 1 || len(layers[0]) != 1 {
		t.Fatalf("layers = %#v", layers)
	}
}
