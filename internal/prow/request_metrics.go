package prow

import (
	"context"
	"sync/atomic"
)

type requestMetricsKey struct{}
type requestMetrics struct{ sippy, gcs atomic.Int64 }

func countRequest(ctx context.Context, sippy bool) {
	m, _ := ctx.Value(requestMetricsKey{}).(*requestMetrics)
	if m == nil {
		return
	}
	if sippy {
		m.sippy.Add(1)
	} else {
		m.gcs.Add(1)
	}
}
