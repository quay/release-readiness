package prow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// IndexedRun is Sippy's public job-run contract. Timestamp is START time;
// terminal time and exact tested images must still come from Prow artifacts.
type IndexedRun struct {
	ID        string    `json:"prow_id"`
	Job       string    `json:"job"`
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
	Result    string    `json:"overall_result"`
}

type SippyClient struct {
	BaseURL            string
	HTTP               *http.Client
	PageSize, MaxPages int
}

func NewSippyClient(baseURL string) *SippyClient {
	return &SippyClient{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}, PageSize: 100, MaxPages: 200}
}

func (c *SippyClient) Runs(ctx context.Context, release string, from, until time.Time) ([]IndexedRun, error) {
	if c.PageSize < 1 || c.MaxPages < 1 || from.After(until) {
		return nil, fmt.Errorf("invalid Sippy window or pagination")
	}
	filter := map[string]any{"linkOperator": "and", "items": []map[string]string{
		{"columnField": "timestamp", "operatorValue": ">=", "value": from.UTC().Format(time.RFC3339Nano)},
		{"columnField": "timestamp", "operatorValue": "<=", "value": until.UTC().Format(time.RFC3339Nano)},
	}}
	f, _ := json.Marshal(filter)
	var rows []IndexedRun
	total := -1
	for page := 0; page < c.MaxPages; page++ {
		q := url.Values{"release": {release}, "perPage": {strconv.Itoa(c.PageSize)}, "page": {strconv.Itoa(page)}, "sortField": {"timestamp"}, "sort": {"desc"}, "filter": {string(f)}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/jobs/runs?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		countRequest(ctx, true)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("sippy %s page %d: HTTP %d", release, page, resp.StatusCode)
		}
		if readErr != nil {
			return nil, readErr
		}
		if len(b) > 16<<20 {
			return nil, fmt.Errorf("sippy page too large")
		}
		var result struct {
			Rows  []IndexedRun `json:"rows"`
			Page  *int         `json:"page"`
			Size  *int         `json:"page_size"`
			Total *int         `json:"total_rows"`
		}
		if err = json.Unmarshal(b, &result); err != nil {
			return nil, err
		}
		if result.Page == nil || result.Size == nil || result.Total == nil || *result.Page != page || *result.Size != c.PageSize || *result.Total < 0 || len(result.Rows) > c.PageSize || (total >= 0 && total != *result.Total) {
			return nil, fmt.Errorf("inconsistent Sippy pagination for %s", release)
		}
		total = *result.Total
		rows = append(rows, result.Rows...)
		if len(rows) == total {
			return rows, nil
		}
		if len(rows) > total || len(result.Rows) == 0 {
			return nil, fmt.Errorf("incomplete Sippy pagination for %s", release)
		}
	}
	return nil, fmt.Errorf("sippy page budget exceeded for %s; narrow the retention/window", release)
}

func validateIndexedRun(row IndexedRun) error {
	if _, err := strconv.ParseUint(row.ID, 10, 64); err != nil {
		return fmt.Errorf("invalid Prow run ID")
	}
	u, err := url.Parse(row.URL)
	if err != nil || u.Scheme != "https" || u.Host != "prow.ci.openshift.org" || row.Timestamp.IsZero() {
		return fmt.Errorf("invalid Sippy run identity")
	}
	valid := false
	for _, prefix := range []string{"/view/gcs/", "/view/gs/"} {
		if u.Path == prefix+bucket+"/logs/"+row.Job+"/"+row.ID {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("sippy URL does not match job/build identity")
	}
	return nil
}
