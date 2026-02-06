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

package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var reader = bufio.NewReader(os.Stdin)

// Prompt asks the user for input with a default value.
func Prompt(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s %s [%s]: ", StylePrimary.Render("?"), label, StyleMuted.Render(defaultVal))
	} else {
		fmt.Printf("  %s %s: ", StylePrimary.Render("?"), label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

// Confirm asks a yes/no question. Returns true for yes.
func Confirm(label string, defaultYes bool) bool {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Printf("  %s %s [%s]: ", StylePrimary.Render("?"), label, StyleMuted.Render(hint))

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}

// Select asks the user to choose from a list of options.
func Select(label string, options []string, defaultIdx int) string {
	fmt.Printf("  %s %s\n", StylePrimary.Render("?"), label)
	for i, opt := range options {
		marker := "  "
		if i == defaultIdx {
			marker = StylePrimary.Render("▸ ")
		}
		fmt.Printf("    %s%s\n", marker, opt)
	}
	fmt.Printf("  %s: ", StyleMuted.Render("Enter number (1-"+fmt.Sprintf("%d", len(options))+")"))

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return options[defaultIdx]
	}
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err == nil && idx >= 1 && idx <= len(options) {
		return options[idx-1]
	}
	return options[defaultIdx]
}
