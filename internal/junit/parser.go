package junit

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/quay/build-dashboard/internal/model"
)

type xmlTestSuites struct {
	XMLName    xml.Name       `xml:"testsuites"`
	TestSuites []xmlTestSuite `xml:"testsuite"`
}

type xmlTestSuite struct {
	XMLName   xml.Name      `xml:"testsuite"`
	Name      string        `xml:"name,attr"`
	Tests     int           `xml:"tests,attr"`
	Failures  int           `xml:"failures,attr"`
	Errors    int           `xml:"errors,attr"`
	Skipped   int           `xml:"skipped,attr"`
	Time      float64       `xml:"time,attr"`
	TestCases []xmlTestCase `xml:"testcase"`
}

type xmlTestCase struct {
	Name      string      `xml:"name,attr"`
	ClassName string      `xml:"classname,attr"`
	Time      float64     `xml:"time,attr"`
	Failure   *xmlFailure `xml:"failure"`
	Error     *xmlFailure `xml:"error"`
	Skipped   *xmlSkipped `xml:"skipped"`
}

type xmlFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

type xmlSkipped struct {
	Message string `xml:"message,attr"`
}

type Result struct {
	Total       int
	Passed      int
	Failed      int
	Skipped     int
	DurationSec float64
	TestCases   []model.TestCase
}

// ParseFile parses a JUnit XML file and returns aggregated results.
func ParseFile(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses JUnit XML data. Handles both <testsuites> and bare <testsuite> roots.
func Parse(data []byte) (*Result, error) {
	// Try <testsuites> first
	var suites xmlTestSuites
	if err := xml.Unmarshal(data, &suites); err == nil && len(suites.TestSuites) > 0 {
		return aggregate(suites.TestSuites), nil
	}

	// Try bare <testsuite>
	var suite xmlTestSuite
	if err := xml.Unmarshal(data, &suite); err == nil {
		return aggregate([]xmlTestSuite{suite}), nil
	}

	return nil, fmt.Errorf("unrecognized JUnit XML format")
}

func aggregate(suites []xmlTestSuite) *Result {
	r := &Result{}
	for _, s := range suites {
		r.DurationSec += s.Time
		for _, tc := range s.TestCases {
			c := model.TestCase{
				Name:        tc.Name,
				ClassName:   tc.ClassName,
				DurationSec: tc.Time,
			}

			switch {
			case tc.Failure != nil:
				c.Status = "failed"
				c.FailureMsg = tc.Failure.Message
				c.FailureText = tc.Failure.Text
				r.Failed++
			case tc.Error != nil:
				c.Status = "error"
				c.FailureMsg = tc.Error.Message
				c.FailureText = tc.Error.Text
				r.Failed++
			case tc.Skipped != nil:
				c.Status = "skipped"
				r.Skipped++
			default:
				c.Status = "passed"
				r.Passed++
			}

			r.Total++
			r.TestCases = append(r.TestCases, c)
		}
	}
	return r
}

// MergeResults merges multiple Result objects into one.
func MergeResults(results ...*Result) *Result {
	merged := &Result{}
	for _, r := range results {
		merged.Total += r.Total
		merged.Passed += r.Passed
		merged.Failed += r.Failed
		merged.Skipped += r.Skipped
		merged.DurationSec += r.DurationSec
		merged.TestCases = append(merged.TestCases, r.TestCases...)
	}
	return merged
}
