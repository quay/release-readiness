# Sippy/GCS mock experiment

The application now has an opt-in production Sippy adapter. This experiment
remains local-only and shares its artifact enrichment implementation; see the
root README for runtime configuration and production integration tests.

Run from the repository root:

```sh
env -u GOROOT GOTOOLCHAIN=auto go test ./internal/prow -run TestSippyExperiment -count=1 -v
```

This experiment uses loopback HTTP servers only. It does not enable Sippy in
the application or fetch live results. Periodic names and targets are a
snapshot of actual release configurations; all runs, results, timestamps,
and image references are synthetic. It assumes Sippy has indexed each
configured release for the measured completed-run scenario; that availability
has not been checked against the deployed service.

`periodics.json` captures 42 jobs across nine streams, including the operator
job, at openshift/release commit
`3ca4fedc6eb1f5780ae883509cf3377d4297b5ea`. Development/master jobs are excluded.

The mock API follows source contracts at openshift/sippy commit
`2faba83d951e3d6cb764e05c8e42f465bd82186b`:

- `pkg/sippyserver/server.go`: `GET /api/jobs/runs`, release-scoped.
- `pkg/sippyserver/parameters.go`: `perPage` and zero-based `page`.
- `pkg/apis/api/types.go`: response `rows`, `page`, `page_size`, `total_rows`;
  run `prow_id` (string), `job`, `url`, `timestamp`, `overall_result`, `succeeded`.
- `pkg/apis/sippyprocessing/v1/types.go`: S/F/I/U/N/n/A/f/R result codes.
- `pkg/dataloader/prowloader/prow.go`: `timestamp` is the run start time.
  Pending and triggered Prow jobs are skipped during preprocessing.
- `pkg/filter/filterable.go`: RFC3339 timestamp bounds via `filter` JSON
  using `columnField: timestamp`, `operatorValue: >=` / `<=`, and
  `linkOperator: and`.

The run API does not expose completion time, ci-operator target, or tested
image/FBC digests. A result code R exists, but this does not establish that
the deployed service indexes all in-progress jobs. The experiment does not
invent a `completed_at`, `target`, or image-digest field in Sippy responses.

The conservative hybrid reads completion/result from `finished.json`, image
identity from `tested-images.json`, and the existing conditional install-step
result. It uses the configured target, falling back to `prowjob.json` if the
target is unknown or the expected artifact is absent. The older CSV fallback
is retained, but does not supply an exact catalog identity. Completed records
without usable identity are not put in the experiment's complete-evidence
cache, so late artifacts can be retried.

This is a request-count experiment, not a latency/load benchmark or a
production-ready adapter. No persistent cursor, scheduler, retry policy, or
live-running-job discovery is implemented here. Counts exclude fetching the
periodic inventory (provided equally to both paths) and any separate live
Prow polling needed to discover runs that Sippy has not indexed. Missing
Sippy rows never mean a validation passed.

The FBC-window scenario uses four configured Quay 3.16 jobs and a mock
3.16.8 candidate. Of three green runs, one predates the window, one is
inside it but used an older FBC, and one used the candidate digest. Only
the last matches; the other three configured jobs retain empty evidence.
This illustrates matching, not a complete implementation of the release
verdict or persistent candidate/job tables.
