package junit

import "testing"

func TestParse_TestSuitesWrapper(t *testing.T) {
	xml := `<testsuites name="Mocha Tests" time="100" tests="3" failures="1">
		<testsuite name="Suite1" tests="2" failures="1" time="80">
			<testcase name="test pass" classname="Suite1" time="30"></testcase>
			<testcase name="test fail" classname="Suite1" time="50">
				<failure message="expected true">assert failed</failure>
			</testcase>
		</testsuite>
		<testsuite name="Suite2" tests="1" failures="0" time="20">
			<testcase name="test skip" classname="Suite2" time="0">
				<skipped/>
			</testcase>
		</testsuite>
	</testsuites>`

	r, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}

	if r.Total != 3 {
		t.Errorf("total: got %d, want 3", r.Total)
	}
	if r.Passed != 1 {
		t.Errorf("passed: got %d, want 1", r.Passed)
	}
	if r.Failed != 1 {
		t.Errorf("failed: got %d, want 1", r.Failed)
	}
	if r.Skipped != 1 {
		t.Errorf("skipped: got %d, want 1", r.Skipped)
	}

	// Check failure details
	for _, tc := range r.TestCases {
		if tc.Name == "test fail" {
			if tc.Status != "failed" {
				t.Errorf("status: got %q, want failed", tc.Status)
			}
			if tc.FailureMsg != "expected true" {
				t.Errorf("failure msg: got %q", tc.FailureMsg)
			}
		}
	}
}

func TestParse_BareTestSuite(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
	<testsuite name="QUAY" tests="2" failures="0">
		<testcase name="test1" classname="C1" time="10"/>
		<testcase name="test2" classname="C2" time="20"/>
	</testsuite>`

	r, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}

	if r.Total != 2 {
		t.Errorf("total: got %d, want 2", r.Total)
	}
	if r.Passed != 2 {
		t.Errorf("passed: got %d, want 2", r.Passed)
	}
}

func TestParse_ErrorElement(t *testing.T) {
	xml := `<testsuite name="S" tests="1" failures="0" errors="1">
		<testcase name="test err" classname="S" time="5">
			<error message="panic">stack trace</error>
		</testcase>
	</testsuite>`

	r, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}

	if r.Failed != 1 {
		t.Errorf("failed: got %d, want 1", r.Failed)
	}
	if r.TestCases[0].Status != "error" {
		t.Errorf("status: got %q, want error", r.TestCases[0].Status)
	}
}

func TestMergeResults(t *testing.T) {
	a := &Result{Total: 5, Passed: 3, Failed: 1, Skipped: 1, DurationSec: 10}
	b := &Result{Total: 3, Passed: 2, Failed: 1, Skipped: 0, DurationSec: 5}
	m := MergeResults(a, b)

	if m.Total != 8 {
		t.Errorf("total: got %d, want 8", m.Total)
	}
	if m.Failed != 2 {
		t.Errorf("failed: got %d, want 2", m.Failed)
	}
	if m.DurationSec != 15 {
		t.Errorf("duration: got %f, want 15", m.DurationSec)
	}
}
