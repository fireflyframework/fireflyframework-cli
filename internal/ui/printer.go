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
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorPrimary = lipgloss.Color("#FF6B35")
	ColorSuccess = lipgloss.Color("#28A745")
	ColorWarning = lipgloss.Color("#FFC107")
	ColorError   = lipgloss.Color("#DC3545")
	ColorInfo    = lipgloss.Color("#17A2B8")
	ColorMuted   = lipgloss.Color("#6C757D")

	StyleBold    = lipgloss.NewStyle().Bold(true)
	StylePrimary = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError)
	StyleInfo    = lipgloss.NewStyle().Foreground(ColorInfo)
	StyleMuted   = lipgloss.NewStyle().Foreground(ColorMuted)
)

// ─────────────────────────────────────────────────────────────────────────────
// Printer — core output primitives
// ─────────────────────────────────────────────────────────────────────────────

type Printer struct{}

func NewPrinter() *Printer {
	return &Printer{}
}

func (p *Printer) Success(msg string) {
	fmt.Println(StyleSuccess.Render("  ✓ ") + msg)
}

func (p *Printer) Error(msg string) {
	fmt.Println(StyleError.Render("  ✗ ") + msg)
}

func (p *Printer) Warning(msg string) {
	fmt.Println(StyleWarning.Render("  ! ") + msg)
}

func (p *Printer) Info(msg string) {
	fmt.Println(StyleInfo.Render("  ℹ ") + msg)
}

func (p *Printer) Step(msg string) {
	fmt.Println(StylePrimary.Render("  → ") + msg)
}

func (p *Printer) KeyValue(key, value string) {
	padded := fmt.Sprintf("%-20s", key+":")
	fmt.Printf("  %s %s\n", StyleMuted.Render(padded), value)
}

func (p *Printer) Header(title string) {
	fmt.Println()
	fmt.Println(StylePrimary.Render("  " + title))
	fmt.Println(StyleMuted.Render("  " + strings.Repeat("─", len(title)+2)))
}

func (p *Printer) Newline() {
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// StageHeader — prominent section separator for multi-phase operations
// ─────────────────────────────────────────────────────────────────────────────

func (p *Printer) StageHeader(phase int, title string) {
	label := fmt.Sprintf(" Phase %d · %s ", phase, title)
	width := 60
	padding := width - len(label)
	if padding < 4 {
		padding = 4
	}
	left := padding / 2
	right := padding - left

	bar := StylePrimary.Render(strings.Repeat("━", left)) +
		StyleBold.Foreground(ColorPrimary).Render(label) +
		StylePrimary.Render(strings.Repeat("━", right))

	fmt.Println()
	fmt.Println("  " + bar)
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// SummaryBox — bordered box for final stats
// ─────────────────────────────────────────────────────────────────────────────

func (p *Printer) SummaryBox(title string, lines []string) {
	// Compute max content width
	maxLen := len(title)
	for _, l := range lines {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	width := maxLen + 4 // 2 padding each side

	borderStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
	titleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	top := borderStyle.Render("  ╭" + strings.Repeat("─", width) + "╮")
	bot := borderStyle.Render("  ╰" + strings.Repeat("─", width) + "╯")
	sep := borderStyle.Render("  ├" + strings.Repeat("─", width) + "┤")

	pad := func(s string) string {
		gap := width - lipgloss.Width(s) - 2
		if gap < 0 {
			gap = 0
		}
		return borderStyle.Render("  │") + " " + s + strings.Repeat(" ", gap) + " " + borderStyle.Render("│")
	}

	fmt.Println()
	fmt.Println(top)
	fmt.Println(pad(titleStyle.Render(title)))
	fmt.Println(sep)
	for _, l := range lines {
		fmt.Println(pad(l))
	}
	fmt.Println(bot)
}

// ─────────────────────────────────────────────────────────────────────────────
// Spinner — braille spinner with elapsed time display
// ─────────────────────────────────────────────────────────────────────────────

type Spinner struct {
	message   string
	done      chan bool
	frames    []string
	startTime time.Time
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		done:    make(chan bool),
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func (s *Spinner) Start() {
	s.startTime = time.Now()
	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				return
			default:
				elapsed := time.Since(s.startTime).Truncate(time.Second)
				frame := StylePrimary.Render(s.frames[i%len(s.frames)])
				timer := StyleMuted.Render(fmt.Sprintf(" (%s)", elapsed))
				fmt.Printf("\r  %s %s%s   ", frame, s.message, timer)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

func (s *Spinner) Stop(success bool) {
	s.done <- true
	elapsed := time.Since(s.startTime).Truncate(time.Second)
	timer := StyleMuted.Render(fmt.Sprintf(" (%s)", elapsed))
	if success {
		fmt.Printf("\r  %s %s%s   \n", StyleSuccess.Render("✓"), s.message, timer)
	} else {
		fmt.Printf("\r  %s %s%s   \n", StyleError.Render("✗"), s.message, timer)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ProgressBar — inline progress indicator with bar, count and percentage
// ─────────────────────────────────────────────────────────────────────────────

const progressBarWidth = 30

// ProgressBar renders an updating inline progress bar.
type ProgressBar struct {
	Total   int
	Current int
	Label   string
}

// NewProgressBar creates a progress bar with the given total.
func NewProgressBar(total int, label string) *ProgressBar {
	return &ProgressBar{Total: total, Label: label}
}

// Increment advances the progress by one and re-renders.
func (pb *ProgressBar) Increment() {
	pb.Current++
	pb.Render()
}

// Render draws the progress bar to the current line (overwrites).
func (pb *ProgressBar) Render() {
	pct := float64(pb.Current) / float64(pb.Total)
	filled := int(pct * float64(progressBarWidth))
	if filled > progressBarWidth {
		filled = progressBarWidth
	}
	empty := progressBarWidth - filled

	bar := StylePrimary.Render(strings.Repeat("█", filled)) +
		StyleMuted.Render(strings.Repeat("░", empty))

	counter := fmt.Sprintf("%d/%d", pb.Current, pb.Total)
	percent := fmt.Sprintf("%.0f%%", pct*100)

	fmt.Printf("\r  %s %s %s %s   ",
		bar,
		StyleBold.Render(counter),
		StyleMuted.Render(percent),
		StyleMuted.Render(pb.Label),
	)
}

// Finish prints the final state of the progress bar and moves to a new line.
func (pb *ProgressBar) Finish() {
	pb.Render()
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// LayerHeader — small label indicating the current DAG layer
// ─────────────────────────────────────────────────────────────────────────────

func (p *Printer) LayerHeader(layer, totalLayers, reposInLayer int) {
	label := fmt.Sprintf("Layer %d/%d  (%d repos)", layer+1, totalLayers, reposInLayer)
	fmt.Printf("  %s\n", StyleMuted.Render("┄ "+label+" "+strings.Repeat("┄", 40-len(label))))
}

// ─────────────────────────────────────────────────────────────────────────────
// CheckResult — doctor check outcome (unchanged)
// ─────────────────────────────────────────────────────────────────────────────

type CheckResult struct {
	Name   string
	Status string // "pass", "fail", "warn"
	Detail string
}

func (p *Printer) PrintChecks(results []CheckResult) {
	for _, r := range results {
		var icon string
		switch r.Status {
		case "pass":
			icon = StyleSuccess.Render("✓")
		case "fail":
			icon = StyleError.Render("✗")
		case "warn":
			icon = StyleWarning.Render("!")
		}
		line := fmt.Sprintf("  %s %s", icon, r.Name)
		if r.Detail != "" {
			line += StyleMuted.Render(" — " + r.Detail)
		}
		fmt.Println(line)
	}
}
