package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// Catch declaring a complete release after omitting one library or signature.
func TestReleaseInventoryRequiresEveryPublishedFile(t *testing.T) {
	inventory := []byte(`[{"groupId":"org.fireflyframework","artifactId":"fireflyframework-parent","version":"26.10.17","packaging":"pom","publish":true},{"groupId":"org.fireflyframework","artifactId":"fireflyframework-bom","version":"26.10.17","packaging":"pom","publish":true},{"groupId":"org.fireflyframework","artifactId":"fireflyframework-kernel","version":"26.10.17","packaging":"jar","publish":true}]`)
	paths := []string{"fireflyframework-parent/26.10.17/fireflyframework-parent-26.10.17.pom", "fireflyframework-bom/26.10.17/fireflyframework-bom-26.10.17.pom", "fireflyframework-kernel/26.10.17/fireflyframework-kernel-26.10.17.pom", "fireflyframework-kernel/26.10.17/fireflyframework-kernel-26.10.17.jar", "fireflyframework-kernel/26.10.17/fireflyframework-kernel-26.10.17-sources.jar", "fireflyframework-kernel/26.10.17/fireflyframework-kernel-26.10.17-javadoc.jar"}
	var github, central []map[string]string
	for _, p := range paths {
		entry := map[string]string{"path": "org/fireflyframework/" + p, "sha256": strings.Repeat("a", 64)}
		github = append(github, entry)
		central = append(central, entry, map[string]string{"path": entry["path"] + ".asc", "sha256": strings.Repeat("b", 64)})
	}
	gh, _ := json.Marshal(github)
	mc, _ := json.Marshal(central)
	if err := validateReleaseInventory(inventory, inventory, gh, mc, "26.10.17"); err != nil {
		t.Fatalf("valid release rejected: %v", err)
	}
	missing, _ := json.Marshal(github[:len(github)-1])
	if err := validateReleaseInventory(inventory, inventory, missing, mc, "26.10.17"); err == nil {
		t.Fatal("missing Javadoc artifact accepted")
	}
	unsigned, _ := json.Marshal(central[:len(central)-1])
	if err := validateReleaseInventory(inventory, inventory, gh, unsigned, "26.10.17"); err == nil {
		t.Fatal("missing signature accepted")
	}
	if err := validateReleaseInventory(inventory, inventory, gh, mc, "26.10.18"); err == nil {
		t.Fatal("different train accepted")
	}
	var projects []map[string]any
	if err := json.Unmarshal(inventory, &projects); err != nil {
		t.Fatal(err)
	}
	truncatedInventory, _ := json.Marshal(projects[:2])
	truncatedGitHub, _ := json.Marshal(github[:2])
	truncatedCentral, _ := json.Marshal(central[:4])
	if err := validateReleaseInventory(truncatedInventory, inventory, truncatedGitHub, truncatedCentral, "26.10.17"); err == nil {
		t.Fatal("parent/BOM-only release accepted after omitting the kernel and all its files")
	}
	projects[2]["publish"] = false
	suppressedKernel, _ := json.Marshal(projects)
	if err := validateReleaseInventory(suppressedKernel, inventory, truncatedGitHub, truncatedCentral, "26.10.17"); err == nil {
		t.Fatal("downloaded inventory disabled publication of a tagged-source library")
	}
}
