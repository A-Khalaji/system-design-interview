package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type benchmarkTarget struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Selected bool   `json:"selected"`
}

type result struct {
	Run             int       `json:"run"`
	Round           int       `json:"round"`
	Target          string    `json:"target"`
	URL             string    `json:"url"`
	Requests        int       `json:"requests"`
	Concurrency     int       `json:"concurrency"`
	RequestsPerSec  float64   `json:"requests_per_second"`
	AverageLatency  string    `json:"average_latency"`
	P50Latency      string    `json:"p50_latency"`
	P95Latency      string    `json:"p95_latency"`
	P99Latency      string    `json:"p99_latency"`
	Fastest         string    `json:"fastest"`
	Slowest         string    `json:"slowest"`
	FailedRequests  int       `json:"failed_requests"`
	StatusCodeLines string    `json:"status_code_lines,omitempty"`
	Duration        string    `json:"duration"`
	StartedAt       time.Time `json:"started_at"`
	Error           string    `json:"error,omitempty"`
	RawOutput       string    `json:"raw_output,omitempty"`
}

type benchmarkFinishedMsg struct {
	result result
}

type model struct {
	requests      string
	concurrency   string
	runsPerTarget string
	focus         int
	running       bool
	current       int
	currentTarget string
	position      int
	round         int
	targets       []benchmarkTarget
	runTargets    []benchmarkTarget
	allResults    []result
	results       []result
	logs          []string
	startedAt     time.Time
	width         int
	height        int
	status        string
	quitAfterRun  bool
}

var (
	throughputRE = regexp.MustCompile(`Requests/sec:\s*([0-9.]+)`)
	averageRE    = regexp.MustCompile(`Average:\s*([^\r\n]+)`)
	p50RE        = regexp.MustCompile(`50%\s+in\s+([0-9.]+\s+secs)`)
	p95RE        = regexp.MustCompile(`95%\s+in\s+([0-9.]+\s+secs)`)
	p99RE        = regexp.MustCompile(`99%\s+in\s+([0-9.]+\s+secs)`)
	fastestRE    = regexp.MustCompile(`Fastest:\s*([^\r\n]+)`)
	slowestRE    = regexp.MustCompile(`Slowest:\s*([^\r\n]+)`)
	statusCodeRE = regexp.MustCompile(`(?m)^\s*\[([0-9]{3})\]\s+([0-9]+)\s+responses`)
	errorLineRE  = regexp.MustCompile(`(?m)^\s*\[([^\]]+)\]\s+([0-9]+)\s+errors?`)
)

var (
	accent        = lipgloss.Color("86")
	muted         = lipgloss.Color("245")
	good          = lipgloss.Color("42")
	bad           = lipgloss.Color("203")
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(accent)
	labelStyle    = lipgloss.NewStyle().Foreground(muted)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	goodStyle     = lipgloss.NewStyle().Foreground(good)
	badStyle      = lipgloss.NewStyle().Foreground(bad)
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1)
)

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func initialModel() model {
	return model{
		requests:      "10000",
		concurrency:   "50",
		runsPerTarget: envOrDefault("BENCHMARK_RUNS", "5"),
		focus:         0,
		targets: []benchmarkTarget{
			{Name: "Plane (single API)", Selected: true, URL: envOrDefault("PLANE_URL", "http://localhost:8080/api/v1/items")},
			{Name: "Distributed (Nginx)", Selected: true, URL: envOrDefault("DISTRIBUTED_URL", "http://localhost:8081/api/v1/items")},
			{Name: "Distributed API 1 (direct)", Selected: true, URL: envOrDefault("DISTRIBUTED_API1_URL", "http://172.21.0.5:8080/api/v1/items")},
			{Name: "Distributed API 2 (direct)", Selected: true, URL: envOrDefault("DISTRIBUTED_API2_URL", "http://172.21.0.4:8080/api/v1/items")},
			{Name: "Distributed API 3 (direct)", Selected: true, URL: envOrDefault("DISTRIBUTED_API3_URL", "http://172.21.0.3:8080/api/v1/items")},
		},
		status: "Ready. Enter runs all targets sequentially.",
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case benchmarkFinishedMsg:
		m.running = false
		m.allResults = append(m.allResults, msg.result)
		m.logs = append(m.logs, formatOneLine(msg.result))
		if msg.result.Error != "" {
			m.status = msg.result.Target + " failed: " + msg.result.Error
		} else {
			m.status = fmt.Sprintf("Round %d/%s: %s finished.", m.round+1, m.runsPerTarget, msg.result.Target)
		}
		if m.quitAfterRun {
			m.results = summarizeResults(m.allResults)
			return m, tea.Quit
		}

		m.position++
		if m.position >= len(m.runTargets) {
			m.position = 0
			m.round++
		}
		if m.round < mustInt(m.runsPerTarget) {
			targetIndex := (m.position + m.round) % len(m.runTargets)
			target := m.runTargets[targetIndex]
			m.current = targetIndex + 1
			m.currentTarget = target.Name
			m.running = true
			m.status = fmt.Sprintf("Round %d/%s: running %s...", m.round+1, m.runsPerTarget, target.Name)
			return m, runBenchmark(target, mustInt(m.requests), mustInt(m.concurrency), m.round+1)
		}

		m.current = 0
		m.currentTarget = ""
		m.results = summarizeResults(m.allResults)
		m.status = fmt.Sprintf("Complete: %d runs collected. Press Ctrl+S to save.", len(m.allResults))
	case tea.KeyMsg:
		if m.running {
			switch msg.String() {
			case "ctrl+c":
				m.quitAfterRun = true
				m.status = "Finishing current run, then exiting without saving..."
			case "ctrl+s":
				if len(m.allResults) == 0 {
					m.status = "No completed results to save yet."
				} else {
					m.results = summarizeResults(m.allResults)
					if err := saveResults(m.allResults, m.results, maxInt(1, mustInt(m.runsPerTarget))); err != nil {
						m.status = "Save failed: " + err.Error()
					} else {
						m.status = "Saved current results to benchmark-results.json."
					}
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "tab", "down", "j":
			m.focus = (m.focus + 1) % (3 + len(m.targets))
		case "shift+tab", "up", "k":
			m.focus = (m.focus - 1 + 3 + len(m.targets)) % (3 + len(m.targets))
		case "1", "2", "3", "4", "5":
			idx, _ := strconv.Atoi(msg.String())
			idx--
			if idx < len(m.targets) {
				m.targets[idx].Selected = !m.targets[idx].Selected
				if m.targets[idx].Selected {
					m.status = "Selected " + m.targets[idx].Name
				} else {
					m.status = "Deselected " + m.targets[idx].Name
				}
				m.focus = idx + 3
			}
		case "x":
			if m.focus >= 3 && m.focus-3 < len(m.targets) {
				idx := m.focus - 3
				m.targets[idx].Selected = !m.targets[idx].Selected
				if m.targets[idx].Selected {
					m.status = "Selected " + m.targets[idx].Name
				} else {
					m.status = "Deselected " + m.targets[idx].Name
				}
			}
		case "left", "h":
			if m.focus == 0 {
				m.requests = adjustNumber(m.requests, -1000, 100, 1000000)
			} else if m.focus == 1 {
				m.concurrency = adjustNumber(m.concurrency, -10, 1, 1000)
			} else if m.focus == 2 {
				m.runsPerTarget = adjustNumber(m.runsPerTarget, -1, 1, 100)
			} else {
				idx := m.focus - 3
				m.targets[idx].Selected = !m.targets[idx].Selected
			}
		case "right", "l":
			if m.focus == 0 {
				m.requests = adjustNumber(m.requests, 1000, 100, 1000000)
			} else if m.focus == 1 {
				m.concurrency = adjustNumber(m.concurrency, 10, 1, 1000)
			} else if m.focus == 2 {
				m.runsPerTarget = adjustNumber(m.runsPerTarget, 1, 1, 100)
			} else {
				idx := m.focus - 3
				m.targets[idx].Selected = !m.targets[idx].Selected
			}
		case " ":
			if m.focus >= 3 && m.focus-3 < len(m.targets) {
				idx := m.focus - 3
				m.targets[idx].Selected = !m.targets[idx].Selected
				if m.targets[idx].Selected {
					m.status = "Selected " + m.targets[idx].Name
				} else {
					m.status = "Deselected " + m.targets[idx].Name
				}
			} else {
				m.status = "Focus a target first; press Space or x to toggle it."
			}
			return m, nil
		case "enter":
			n, err := strconv.Atoi(m.requests)
			if err != nil || n < 1 || n > 1000000 {
				m.status = "Requests must be between 1 and 1,000,000."
				return m, nil
			}
			c, err := strconv.Atoi(m.concurrency)
			if err != nil || c < 1 || c > 1000 {
				m.status = "Concurrency must be between 1 and 1,000."
				return m, nil
			}
			runs, err := strconv.Atoi(m.runsPerTarget)
			if err != nil || runs < 1 || runs > 100 {
				m.status = "Runs per target must be between 1 and 100."
				return m, nil
			}
			selectedTargets := make([]benchmarkTarget, 0, len(m.targets))
			for _, target := range m.targets {
				if target.Selected {
					selectedTargets = append(selectedTargets, target)
				}
			}
			if len(selectedTargets) == 0 {
				m.status = "Select at least one target before starting."
				return m, nil
			}
			if _, err := exec.LookPath("hey"); err != nil {
				m.status = "hey is not installed or is not in PATH."
				return m, nil
			}
			m.runTargets = selectedTargets
			m.allResults = nil
			m.results = nil
			m.logs = nil
			m.round = 0
			m.position = 0
			m.current = 1
			m.currentTarget = m.runTargets[0].Name
			m.running = true
			m.quitAfterRun = false
			m.startedAt = time.Now()
			target := m.runTargets[0]
			m.status = fmt.Sprintf("Round 1/%d: running %s...", runs, target.Name)
			return m, runBenchmark(target, n, c, 1)
		case "ctrl+s":
			if len(m.allResults) == 0 {
				m.status = "No results to save yet."
				return m, nil
			}
			m.results = summarizeResults(m.allResults)
			if err := saveResults(m.allResults, m.results, maxInt(1, mustInt(m.runsPerTarget))); err != nil {
				m.status = "Save failed: " + err.Error()
			} else {
				m.status = "Saved individual runs and median summaries to benchmark-results.json."
			}
		case "r":
			m.allResults = nil
			m.results = nil
			m.logs = nil
			m.status = "Results cleared."
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("SYSTEM DESIGN / BENCHMARK TUI"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Configure the run, choose targets, then press Enter to start."))
	b.WriteString("\n\n")

	requestControl := compactField("Requests", m.requests, m.focus == 0)
	concurrencyControl := compactField("Concurrency", m.concurrency, m.focus == 1)
	runsControl := compactField("Runs/target", m.runsPerTarget, m.focus == 2)
	controls := requestControl + "   │   " + concurrencyControl + "   │   " + runsControl
	controlsHint := labelStyle.Render("Focus a value with Tab/↑/↓; ←/→ changes it.")
	b.WriteString(boxStyle.Render(titleStyle.Render("RUN SETTINGS") + "\n" + controls + "\n" + controlsHint))
	b.WriteString("\n\n")

	b.WriteString(titleStyle.Render("TARGETS"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Press 1–5 to toggle a target directly, or focus one and press Space/x."))
	b.WriteString("\n")
	selectedCount := 0
	for i, target := range m.targets {
		marker := "[ ]"
		if target.Selected {
			marker = "[x]"
			selectedCount++
		}
		prefix := "  "
		if m.focus == i+3 {
			prefix = "> "
		}
		line := fmt.Sprintf("%s[%d] %s %-29s %s", prefix, i+1, marker, target.Name, labelStyle.Render(target.URL))
		if m.focus == i+3 {
			line = selectedStyle.Render(fmt.Sprintf("%s[%d] %s %-29s %s", prefix, i+1, marker, target.Name, target.URL))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(labelStyle.Render(fmt.Sprintf("%d of %d targets selected", selectedCount, len(m.targets))))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Enter run selected  •  Ctrl+S save JSON  •  r clear results  •  q quit"))
	b.WriteString("\n\n")

	if m.running {
		name := m.currentTarget
		b.WriteString(goodStyle.Render("● RUNNING: " + name))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("The benchmark runs in the background; the TUI remains responsive."))
		b.WriteString("\n\n")
	}

	if len(m.results) > 0 {
		b.WriteString(titleStyle.Render("MEDIAN RESULTS (individual runs are saved too)"))
		b.WriteString("\n")
		const (
			targetW = 25
			rpsW    = 9
			avgW    = 12
			p50W    = 12
			p95W    = 12
			p99W    = 12
			failedW = 7
		)
		columns := []int{targetW, rpsW, avgW, p50W, p95W, p99W, failedW}
		headers := []string{"Target", "Req/sec", "Avg", "P50", "P95", "P99", "Failed"}
		renderGridLine := func(left, middle, right, fill string) string {
			var line strings.Builder
			line.WriteString(left)
			for i, width := range columns {
				line.WriteString(strings.Repeat(fill, width+2))
				if i < len(columns)-1 {
					line.WriteString(middle)
				}
			}
			line.WriteString(right)
			return line.String()
		}
		renderGridRow := func(cells []string) string {
			var line strings.Builder
			line.WriteString("│")
			for i, width := range columns {
				value := cells[i]
				if lipgloss.Width(value) > width {
					value = truncate(value, width)
				}
				cell := lipgloss.NewStyle().Width(width)
				if i > 0 {
					cell = cell.Align(lipgloss.Right)
				}
				line.WriteString(" " + cell.Render(value) + " │")
			}
			return line.String()
		}
		b.WriteString(labelStyle.Render(renderGridLine("┌", "┬", "┐", "─")) + "\n")
		b.WriteString(titleStyle.Render(renderGridRow(headers)) + "\n")
		b.WriteString(labelStyle.Render(renderGridLine("├", "┼", "┤", "─")) + "\n")
		for i, r := range m.results {
			cells := []string{r.Target, metricRPS(r), emptyDash(r.AverageLatency), emptyDash(r.P50Latency), emptyDash(r.P95Latency), emptyDash(r.P99Latency), strconv.Itoa(r.FailedRequests)}
			line := renderGridRow(cells)
			if r.Error != "" {
				b.WriteString(badStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
			if i < len(m.results)-1 {
				b.WriteString(labelStyle.Render(renderGridLine("├", "┼", "┤", "─")) + "\n")
			}
		}
		b.WriteString(labelStyle.Render(renderGridLine("└", "┴", "┘", "─")) + "\n")
		if len(m.results) > 1 && m.results[0].RequestsPerSec > 0 {
			b.WriteString("\n" + titleStyle.Render("THROUGHPUT VS FIRST SELECTED TARGET") + "\n")
			baseline := m.results[0].RequestsPerSec
			for _, r := range m.results[1:] {
				if r.RequestsPerSec <= 0 {
					continue
				}
				pct := (r.RequestsPerSec/baseline - 1) * 100
				style := goodStyle
				if pct < 0 {
					style = badStyle
				}
				b.WriteString(style.Render(fmt.Sprintf("  %-30s %+.1f%%", r.Target, pct)) + "\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(labelStyle.Render("STATUS: ") + m.status)
	if m.width > 0 && lipgloss.Width(b.String()) > m.width {
		return lipgloss.NewStyle().Width(m.width).Render(b.String())
	}
	return b.String()
}

func compactField(name, value string, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	line := fmt.Sprintf("%s%s: %s", prefix, name, value)
	if selected {
		return selectedStyle.Render(line)
	}
	return line
}

func runBenchmark(target benchmarkTarget, requests, concurrency, runNumber int) tea.Cmd {
	return func() tea.Msg {
		started := time.Now()
		args := []string{"-n", strconv.Itoa(requests), "-c", strconv.Itoa(concurrency), target.URL}
		cmd := exec.Command("hey", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		out := stdout.String() + stderr.String()
		r := parseResult(target, requests, concurrency, started, time.Since(started), out)
		r.Run = runNumber
		r.Round = runNumber
		if err != nil {
			r.Error = err.Error()
			if strings.TrimSpace(out) == "" {
				r.Error += " (no output)"
			}
		}
		return benchmarkFinishedMsg{result: r}
	}
}

func parseResult(target benchmarkTarget, requests, concurrency int, started time.Time, duration time.Duration, output string) result {
	r := result{
		Target: target.Name, URL: target.URL, Requests: requests,
		Concurrency: concurrency, Duration: duration.Round(time.Millisecond).String(),
		StartedAt: started, RawOutput: output,
	}
	if m := throughputRE.FindStringSubmatch(output); len(m) > 1 {
		r.RequestsPerSec, _ = strconv.ParseFloat(m[1], 64)
	}
	r.AverageLatency = capture(averageRE, output)
	r.P50Latency = capture(p50RE, output)
	r.P95Latency = capture(p95RE, output)
	r.P99Latency = capture(p99RE, output)
	r.Fastest = capture(fastestRE, output)
	r.Slowest = capture(slowestRE, output)

	// Status-code responses are completed requests, not failed requests.
	successResponses := 0
	var statuses []string
	for _, row := range statusCodeRE.FindAllStringSubmatch(output, -1) {
		status, _ := strconv.Atoi(row[1])
		count, _ := strconv.Atoi(row[2])
		if status >= 200 && status < 300 {
			successResponses += count
		}
		statuses = append(statuses, fmt.Sprintf("%d: %d", status, count))
	}
	r.StatusCodeLines = strings.Join(statuses, ", ")
	if successResponses > requests {
		successResponses = requests
	}
	r.FailedRequests = requests - successResponses

	// Account for explicitly reported transport/request errors as well.
	explicitErrors := 0
	for _, row := range errorLineRE.FindAllStringSubmatch(output, -1) {
		count, _ := strconv.Atoi(row[2])
		explicitErrors += count
	}
	if explicitErrors > r.FailedRequests {
		r.FailedRequests = explicitErrors
	}
	if r.FailedRequests < 0 {
		r.FailedRequests = 0
	}
	if r.RequestsPerSec == 0 && strings.TrimSpace(output) == "" {
		r.Error = "hey returned no output"
	}
	return r
}

func capture(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return "—"
	}
	return strings.TrimSpace(m[1])
}
func mustInt(s string) int { n, _ := strconv.Atoi(s); return n }
func adjustNumber(s string, delta, min, max int) string {
	n := mustInt(s) + delta
	if n < min {
		n = min
	}
	if n > max {
		n = max
	}
	return strconv.Itoa(n)
}
func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
func metricRPS(r result) string {
	if r.RequestsPerSec == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f", r.RequestsPerSec)
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
func formatOneLine(r result) string {
	if r.Error != "" {
		return r.Target + ": ERROR " + r.Error
	}
	return fmt.Sprintf("%s: %.1f req/s, avg %s, p95 %s", r.Target, r.RequestsPerSec, r.AverageLatency, r.P95Latency)
}

type savedBenchmark struct {
	SavedAt        time.Time `json:"saved_at"`
	RunsPerTarget  int       `json:"runs_per_target"`
	IndividualRuns []result  `json:"individual_runs"`
	MedianSummary  []result  `json:"median_summary"`
}

func saveResults(allResults, summaries []result, runsPerTarget int) error {
	payload := savedBenchmark{
		SavedAt:        time.Now(),
		RunsPerTarget:  runsPerTarget,
		IndividualRuns: allResults,
		MedianSummary:  summaries,
	}
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	jsonData = append(jsonData, '\n')
	return writeFileAtomic("benchmark-results.json", jsonData)
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func summarizeResults(all []result) []result {
	byTarget := make(map[string][]result)
	order := make([]string, 0)
	for _, r := range all {
		if _, exists := byTarget[r.Target]; !exists {
			order = append(order, r.Target)
		}
		byTarget[r.Target] = append(byTarget[r.Target], r)
	}
	out := make([]result, 0, len(order))
	for _, name := range order {
		runs := byTarget[name]
		if len(runs) == 0 {
			continue
		}
		summary := runs[0]
		summary.Run = len(runs)
		summary.Error = ""
		summary.RawOutput = ""
		summary.StatusCodeLines = "median of " + strconv.Itoa(len(runs)) + " runs"
		var rps []float64
		var avg, p50, p95, p99, fastest, slowest []float64
		var failed []int
		for _, r := range runs {
			rps = append(rps, r.RequestsPerSec)
			failed = append(failed, r.FailedRequests)
			if v, ok := latencySeconds(r.AverageLatency); ok {
				avg = append(avg, v)
			}
			if v, ok := latencySeconds(r.P50Latency); ok {
				p50 = append(p50, v)
			}
			if v, ok := latencySeconds(r.P95Latency); ok {
				p95 = append(p95, v)
			}
			if v, ok := latencySeconds(r.P99Latency); ok {
				p99 = append(p99, v)
			}
			if v, ok := latencySeconds(r.Fastest); ok {
				fastest = append(fastest, v)
			}
			if v, ok := latencySeconds(r.Slowest); ok {
				slowest = append(slowest, v)
			}
		}
		summary.RequestsPerSec = medianFloat(rps)
		summary.FailedRequests = int(medianInt(failed))
		summary.AverageLatency = formatSecondsMedian(avg)
		summary.P50Latency = formatSecondsMedian(p50)
		summary.P95Latency = formatSecondsMedian(p95)
		summary.P99Latency = formatSecondsMedian(p99)
		summary.Fastest = formatSecondsMedian(fastest)
		summary.Slowest = formatSecondsMedian(slowest)
		summary.Duration = "median of " + strconv.Itoa(len(runs)) + " runs"
		out = append(out, summary)
	}
	return out
}

func latencySeconds(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "—" || s == "-" {
		return 0, false
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	if len(fields) > 1 {
		switch strings.ToLower(fields[1]) {
		case "ms", "msec", "milliseconds":
			return v / 1000, true
		case "us", "µs", "usec":
			return v / 1000000, true
		case "ns":
			return v / 1000000000, true
		}
	}
	return v, true
}

func formatSecondsMedian(values []float64) string {
	if len(values) == 0 {
		return "—"
	}
	return fmt.Sprintf("%.4f secs", medianFloat(values))
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	mid := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[mid]
	}
	return (copyValues[mid-1] + copyValues[mid]) / 2
}

func medianInt(values []int) float64 {
	floats := make([]float64, len(values))
	for i, v := range values {
		floats[i] = float64(v)
	}
	return medianFloat(floats)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		fmt.Fprintln(os.Stderr, "benchmark-tui:", err)
		os.Exit(1)
	}
}
