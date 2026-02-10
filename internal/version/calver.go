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

package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CalVer holds the parsed components of a CalVer version (YY.MM.Patch).
type CalVer struct {
	Year  int
	Month int
	Patch int
}

// String returns the CalVer as "YY.MM.PP" with zero-padded components.
func (v CalVer) String() string {
	return fmt.Sprintf("%02d.%02d.%02d", v.Year, v.Month, v.Patch)
}

// TagString returns the CalVer as a git tag string "vYY.MM.PP".
func (v CalVer) TagString() string {
	return "v" + v.String()
}

// Parse parses a "YY.MM.Patch" string (with optional "v" prefix) into a CalVer.
func Parse(s string) (CalVer, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return CalVer{}, fmt.Errorf("invalid calver: %q (expected YY.MM.Patch)", s)
	}
	yy, err := strconv.Atoi(parts[0])
	if err != nil {
		return CalVer{}, fmt.Errorf("invalid year in calver %q: %w", s, err)
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil {
		return CalVer{}, fmt.Errorf("invalid month in calver %q: %w", s, err)
	}
	p, err := strconv.Atoi(parts[2])
	if err != nil {
		return CalVer{}, fmt.Errorf("invalid patch in calver %q: %w", s, err)
	}
	return CalVer{Year: yy, Month: mm, Patch: p}, nil
}

// Current returns a CalVer for today's date with patch=1.
func Current() CalVer {
	now := time.Now()
	return CalVer{
		Year:  now.Year() % 100,
		Month: int(now.Month()),
		Patch: 1,
	}
}

// Next computes the next CalVer from the given current version.
// If the current month matches today, patch is incremented.
// Otherwise, a new month starts at patch 1.
func Next(current CalVer) CalVer {
	now := time.Now()
	yr := now.Year() % 100
	mo := int(now.Month())

	if current.Year == yr && current.Month == mo {
		return CalVer{Year: yr, Month: mo, Patch: current.Patch + 1}
	}
	return CalVer{Year: yr, Month: mo, Patch: 1}
}

// Compare returns +1 if a > b, -1 if a < b, 0 if equal.
// Comparison order: Year, Month, Patch.
func Compare(a, b CalVer) int {
	switch {
	case a.Year != b.Year:
		if a.Year > b.Year {
			return 1
		}
		return -1
	case a.Month != b.Month:
		if a.Month > b.Month {
			return 1
		}
		return -1
	case a.Patch != b.Patch:
		if a.Patch > b.Patch {
			return 1
		}
		return -1
	default:
		return 0
	}
}

// CompareStrings parses two version strings and returns their comparison.
func CompareStrings(a, b string) (int, error) {
	av, err := Parse(a)
	if err != nil {
		return 0, err
	}
	bv, err := Parse(b)
	if err != nil {
		return 0, err
	}
	return Compare(av, bv), nil
}
