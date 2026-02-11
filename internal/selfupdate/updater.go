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

package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	repoOwner = "fireflyframework"
	repoName  = "fireflyframework-cli"
)

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// UpdateResult holds the result of an update check.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	UpdateAvail    bool
	ReleaseNotes   string
	DownloadURL    string
	AssetName      string
}

// CheckForUpdate queries GitHub for the latest release and compares versions.
// Versions follow CalVer YY.MM.Patch (e.g. 26.02.01 = 2026 Jan patch 1).
func CheckForUpdate(currentVersion string) (*UpdateResult, error) {
	release, err := fetchLatestRelease()
	if err != nil {
		return nil, err
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	updateAvail := false
	if current == "dev" {
		updateAvail = false
	} else if cmp, err := compareCalVer(latest, current); err == nil {
		updateAvail = cmp > 0 // latest is strictly newer
	}

	result := &UpdateResult{
		CurrentVersion: current,
		LatestVersion:  latest,
		UpdateAvail:    updateAvail,
		ReleaseNotes:   release.Body,
	}

	if result.UpdateAvail {
		// Asset names include the 'v' prefix to match Makefile output.
		assetName := platformAssetName("v" + latest)
		for _, a := range release.Assets {
			if a.Name == assetName {
				result.DownloadURL = a.BrowserDownloadURL
				result.AssetName = a.Name
				break
			}
		}
		if result.DownloadURL == "" {
			return nil, fmt.Errorf("no release asset found for %s/%s (%s)", runtime.GOOS, runtime.GOARCH, assetName)
		}
	}

	return result, nil
}

// calVer holds the parsed components of a CalVer version (YY.MM.Patch).
type calVer struct {
	Year  int
	Month int
	Patch int
}

// parseCalVer parses a "YY.MM.Patch" string into its components.
func parseCalVer(v string) (calVer, error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return calVer{}, fmt.Errorf("invalid calver: %q (expected YY.MM.Patch)", v)
	}
	yy, err := strconv.Atoi(parts[0])
	if err != nil {
		return calVer{}, fmt.Errorf("invalid year in calver %q: %w", v, err)
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil {
		return calVer{}, fmt.Errorf("invalid month in calver %q: %w", v, err)
	}
	p, err := strconv.Atoi(parts[2])
	if err != nil {
		return calVer{}, fmt.Errorf("invalid patch in calver %q: %w", v, err)
	}
	return calVer{Year: yy, Month: mm, Patch: p}, nil
}

// compareCalVer returns +1 if a > b, -1 if a < b, 0 if equal.
// Comparison order: Year, Month, Patch.
func compareCalVer(a, b string) (int, error) {
	av, err := parseCalVer(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseCalVer(b)
	if err != nil {
		return 0, err
	}
	switch {
	case av.Year != bv.Year:
		if av.Year > bv.Year {
			return 1, nil
		}
		return -1, nil
	case av.Month != bv.Month:
		if av.Month > bv.Month {
			return 1, nil
		}
		return -1, nil
	case av.Patch != bv.Patch:
		if av.Patch > bv.Patch {
			return 1, nil
		}
		return -1, nil
	default:
		return 0, nil
	}
}

// Apply downloads and installs the update, replacing the current binary.
func Apply(result *UpdateResult) error {
	if !result.UpdateAvail || result.DownloadURL == "" {
		return fmt.Errorf("no update available")
	}

	// Download the archive
	resp, err := http.Get(result.DownloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "flywork-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download write: %w", err)
	}
	tmpFile.Close()

	// Extract binary from archive
	var newBinary string
	if strings.HasSuffix(result.AssetName, ".zip") {
		newBinary, err = extractZip(tmpFile.Name())
	} else {
		newBinary, err = extractTarGz(tmpFile.Name())
	}
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	defer os.Remove(newBinary)

	// Replace current binary
	currentBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current executable: %w", err)
	}
	currentBin, err = filepath.EvalSymlinks(currentBin)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	return replaceBinary(currentBin, newBinary)
}

func fetchLatestRelease() (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}
	return &release, nil
}

func platformAssetName(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	if os == "windows" {
		return fmt.Sprintf("flywork-%s-%s-%s.zip", version, os, arch)
	}
	return fmt.Sprintf("flywork-%s-%s-%s.tar.gz", version, os, arch)
}

func extractTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	binaryName := "flywork"
	if runtime.GOOS == "windows" {
		binaryName = "flywork.exe"
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(hdr.Name) == binaryName && !hdr.FileInfo().IsDir() {
			tmpBin, err := os.CreateTemp("", "flywork-bin-*")
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(tmpBin, tr); err != nil {
				tmpBin.Close()
				return "", err
			}
			tmpBin.Close()
			os.Chmod(tmpBin.Name(), 0755)
			return tmpBin.Name(), nil
		}
	}
	return "", fmt.Errorf("binary not found in archive")
}

func extractZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	binaryName := "flywork"
	if runtime.GOOS == "windows" {
		binaryName = "flywork.exe"
	}

	for _, f := range r.File {
		if filepath.Base(f.Name) == binaryName && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			tmpBin, err := os.CreateTemp("", "flywork-bin-*")
			if err != nil {
				rc.Close()
				return "", err
			}
			if _, err := io.Copy(tmpBin, rc); err != nil {
				tmpBin.Close()
				rc.Close()
				return "", err
			}
			rc.Close()
			tmpBin.Close()
			os.Chmod(tmpBin.Name(), 0755)
			return tmpBin.Name(), nil
		}
	}
	return "", fmt.Errorf("binary not found in zip")
}

func replaceBinary(currentPath, newPath string) error {
	if runtime.GOOS == "windows" {
		// Windows: can't replace running binary directly; rename-swap
		oldPath := currentPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(currentPath, oldPath); err != nil {
			return fmt.Errorf("rename old binary: %w", err)
		}
		if err := copyFile(newPath, currentPath); err != nil {
			// Rollback
			os.Rename(oldPath, currentPath)
			return fmt.Errorf("copy new binary: %w", err)
		}
		os.Remove(oldPath)
		return nil
	}

	// Unix: atomic rename
	tmpDest := currentPath + ".new"
	if err := copyFile(newPath, tmpDest); err != nil {
		return fmt.Errorf("copy new binary: %w", err)
	}
	os.Chmod(tmpDest, 0755)
	if err := os.Rename(tmpDest, currentPath); err != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
