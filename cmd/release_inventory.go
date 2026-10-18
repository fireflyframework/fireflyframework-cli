package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
)

type releaseProject struct {
	GroupID    string `json:"groupId"`
	ArtifactID string `json:"artifactId"`
	Version    string `json:"version"`
	Packaging  string `json:"packaging"`
	Directory  string `json:"directory"`
	Publish    bool   `json:"publish"`
}

// validateReleaseInventory anchors the downloaded inventory to the tagged
// source before checking every publication file and signature.
func validateReleaseInventory(inventory, sourceInventory, githubManifest, centralManifest []byte, version string) error {
	var projects, sourceProjects []releaseProject
	if err := json.Unmarshal(inventory, &projects); err != nil {
		return err
	}
	if err := json.Unmarshal(sourceInventory, &sourceProjects); err != nil {
		return err
	}
	if len(sourceProjects) == 0 || len(projects) != len(sourceProjects) {
		return fmt.Errorf("release inventory project count differs from tagged source")
	}
	source := make(map[string]releaseProject, len(sourceProjects))
	for _, project := range sourceProjects {
		key := project.GroupID + ":" + project.ArtifactID
		if _, duplicate := source[key]; duplicate {
			return fmt.Errorf("duplicate tagged-source project: %s", key)
		}
		source[key] = project
	}
	for _, project := range projects {
		key := project.GroupID + ":" + project.ArtifactID
		if expected, exists := source[key]; !exists || expected != project {
			return fmt.Errorf("release inventory project differs from tagged source: %s", key)
		}
		delete(source, key)
	}
	expected := make(map[string]bool)
	artifacts := make(map[string]bool)
	validID := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	for _, project := range projects {
		if !project.Publish {
			continue
		}
		if project.GroupID != "org.fireflyframework" || !validID.MatchString(project.ArtifactID) || project.Version != version {
			return fmt.Errorf("invalid release project: %s:%s:%s", project.GroupID, project.ArtifactID, project.Version)
		}
		if artifacts[project.ArtifactID] {
			return fmt.Errorf("duplicate artifact: %s", project.ArtifactID)
		}
		artifacts[project.ArtifactID] = true
		prefix := path.Join("org/fireflyframework", project.ArtifactID, version, project.ArtifactID+"-"+version)
		expected[prefix+".pom"] = true
		switch project.Packaging {
		case "jar":
			expected[prefix+".jar"] = true
			expected[prefix+"-sources.jar"] = true
			expected[prefix+"-javadoc.jar"] = true
		case "pom":
		default:
			return fmt.Errorf("unsupported release packaging: %s", project.Packaging)
		}
	}
	if !artifacts["fireflyframework-parent"] || !artifacts["fireflyframework-bom"] {
		return fmt.Errorf("release inventory must include parent and BOM")
	}
	for _, manifest := range []struct {
		data   []byte
		signed bool
	}{{githubManifest, false}, {centralManifest, true}} {
		required := make(map[string]bool, len(expected)*2)
		for p := range expected {
			required[p] = true
			if manifest.signed {
				required[p+".asc"] = true
			}
		}
		var files []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(manifest.data, &files); err != nil {
			return err
		}
		for _, file := range files {
			if !required[file.Path] {
				return fmt.Errorf("unexpected or duplicate release file: %s", file.Path)
			}
			digest, err := hex.DecodeString(file.SHA256)
			if err != nil || len(digest) != 32 {
				return fmt.Errorf("invalid SHA256 for %s", file.Path)
			}
			delete(required, file.Path)
		}
		if len(required) > 0 {
			return fmt.Errorf("release manifest omits %d expected files", len(required))
		}
	}
	return nil
}
