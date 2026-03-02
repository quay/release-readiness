package ctrf

// Report is the top-level CTRF JSON structure.
type Report struct {
	Results Results `json:"results"`
}

// Results contains the tool info, summary, and individual test outcomes.
type Results struct {
	Tool    Tool   `json:"tool"`
	Summary Summary `json:"summary"`
	Tests   []Test  `json:"tests"`
}

// Tool identifies the test runner that produced the report.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Summary holds aggregate counts from a CTRF report.
type Summary struct {
	Tests   int   `json:"tests"`
	Passed  int   `json:"passed"`
	Failed  int   `json:"failed"`
	Skipped int   `json:"skipped"`
	Pending int   `json:"pending"`
	Other   int   `json:"other"`
	Flaky   int   `json:"flaky"`
	Start   int64 `json:"start"`
	Stop    int64 `json:"stop"`
}

// Test represents a single test case outcome in a CTRF report.
type Test struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	Duration float64 `json:"duration"`
	Message  string  `json:"message,omitempty"`
	Trace    string  `json:"trace,omitempty"`
	FilePath string  `json:"filePath,omitempty"`
	Suite    string  `json:"suite,omitempty"`
	Retries  int     `json:"retries,omitempty"`
	Flaky    bool    `json:"flaky,omitempty"`
}
