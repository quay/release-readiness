# CLI and configuration reference

[Back to the project README](../README.md)

## Configuration layers

`cmd/release-readiness/main.go` is the source of truth. Explicit command-line
flags override environment-derived defaults. Environment fallbacks exist only
where listed below; the binary does not load `.env` files itself. Boolean env
switches are enabled by the exact value `true`. For fallback values implemented
with `cmp.Or`, an empty environment variable restores the default; use an explicit
empty CLI flag to disable a URL or pattern where supported.

The three entry points intentionally expose different interfaces:

- **Binary:** all flags below; use `release-readiness -help` for executable help.
- **Local Compose:** loads repository-root `.env`. Its `RR_*` substitutions control
  port, data mount, discovery, Sippy URL, and ART groups. See `compose.yaml`.
- **OpenShift:** [`deploy/app.yaml`](../deploy/app.yaml) sets the deployed
  defaults, mapping values without native environment support into flags;
  Secrets are referenced by name and created separately. See the
  [deployment instructions](../deploy/README.md).

The supplied deployment enables dynamic Prow discovery and Sippy, configures
Quay ART groups, and keeps JIRA read-only by default (the FBC JIRA
writer is opt-in with its separate credential). Binary defaults below are not
automatically the deployed values.

## Flags

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `-addr` | — | `:8080` | Listen address |
| `-db` | — | `dashboard.db` | SQLite database path |
| `-base-path` | `BASE_PATH` | — | URL prefix the app is served under (e.g. `/v2`), stripped from requests before routing; the web bundle must be built with the matching `VITE_BASE` |
| `-kubeconfig` | `KUBECONFIG` | — | Kubeconfig path (empty uses the in-cluster service account) |
| `-namespace` | `KONFLUX_NAMESPACE` | `art-quay-tenant` | Konflux namespace to read Snapshots and Releases from |
| `-konflux-poll-interval` | — | `30s` | Konflux sync poll interval |
| `-konflux-ui-url` | `KONFLUX_UI_URL` | `https://konflux-ui.apps.kflux-ocp-p01.7ayg.p1.openshiftapps.com` | Konflux UI URL for links to `-namespace`'s applications, Snapshots and Releases (empty disables Konflux links) |
| `-art-build-history-url` | — | `https://art-build-history-art-build-history.apps.artc2023.pc3z.p1.openshiftapps.com` | ART build history service; component images are looked up there in the background for build links, and FBC builds synced and rendered for the FBC candidate (empty disables all of them) |
| `-tekton-results-url` | — | `https://tekton-results-tekton-results.apps.kflux-ocp-p01.7ayg.p1.openshiftapps.com` | Tekton Results API; the CVE scans of candidate images are read there in the background with the Konflux credential (empty disables) |
| `-jira-url` | `JIRA_URL` | `https://redhat.atlassian.net` | JIRA Cloud URL |
| `-jira-api-url` | `JIRA_API_URL` | `-jira-url` | JIRA REST API base URL; set to `https://api.atlassian.com/ex/jira/<cloudId>` for scoped API tokens |
| `-jira-email` | `JIRA_EMAIL` | — | JIRA Cloud account email for API token auth |
| `-jira-token` | `JIRA_TOKEN` | — | JIRA Cloud API token (required to enable JIRA sync) |
| `-jira-project` | `JIRA_PROJECT` | `PROJQUAY` | JIRA project key |
| `-jira-poll-interval` | — | `5m` | JIRA sync poll interval |
| `-github-token` | `GITHUB_TOKEN` | — | Read-only GitHub token for inventory and the reconciler's commit range lookups (empty reads GitHub unauthenticated) |
| `-jira-write` | `JIRA_WRITE` | `false` | Write the reconciler's proposed actions to JIRA when `true`; needs `-jira-write-email` and `-jira-write-token`, else stays off |
| `-jira-write-email` | `JIRA_WRITE_EMAIL` | — | JIRA Cloud account email of the write credential |
| `-jira-write-token` | `JIRA_WRITE_TOKEN` | — | JIRA Cloud API token of the write credential, separate from `-jira-token` |
| `-stage-release-plan-pattern` | `STAGE_RELEASE_PLAN_PATTERN` | `^quay-advisory-stage-\d+-\d+$` | Regexp matching the Konflux ReleasePlan names whose successful Releases are image STAGE builds (explicit empty flag selects no staged build) |
| `-prow-interval` | — | `15m` | Prow sync poll interval |
| `-retention` | — | `720h` | Age past which data no release needs is pruned daily and not ingested: Snapshots not ART staged, shipped or the newest of their application, Failed Releases not of an ART staged Snapshot, Prow runs and unreferenced image caches (0 disables) |
| `-catalog-url` | — | `https://catalog.redhat.com/api/containers/v1` | Red Hat container catalog API, checked hourly in the background to mark versions shipped (empty leaves JIRA's released flag as the only shipped signal) |
| `-art-fbc-quay-url` | — | `https://quay.io` | Quay API to list the tags of ART's public `redhat-user-workloads/ocp-art-tenant/art-fbc` repository from, without credentials, for the Release inputs FBC pullspecs; the stream's `group.yml` is read from `-ocp-build-data-url` (empty disables) |
| `-gitlab-url` | — | `https://gitlab.cee.redhat.com` | GitLab holding ART's shipment MRs, polled for each unshipped version and each still-open MR (empty disables) |
| `-gitlab-token` | `GITLAB_TOKEN` | — | GitLab `read_api` token (empty reads anonymously, throttled) |
| `-shipment-project` | — | `116177` | GitLab project ID of ART's shipment MRs (ocp-shipment-data) |
| `-ocp-build-data-url` | — | `https://raw.githubusercontent.com/openshift-eng/ocp-build-data` | Raw ocp-build-data URL, whose `quay-X.Y` `releases.yml` names each assembly's shipment MR |
| `-gitlab-poll-interval` | — | `5m` | Shipment MR sync poll interval |
| `-prow-job-discovery` | `PROW_JOB_DISCOVERY` | `false` | Persist required periodic definitions from GitHub |
| `-prow-inventory-interval` | — | 10m | Check the pinned config revision; unchanged revisions skip file downloads |
| `-sippy-url` | `SIPPY_URL` | empty | Sippy API base; requires discovery enabled |
| `-ci-workers` | — | 4 | Concurrent GCS evidence fetches |
| `-jira-workers` | — | 2 | Concurrent release issue fetches with shared request pacing |
| `-art-workers` | — | 2 | Concurrent ART component resolutions |
| `-ci-stream-budget` | — | 2m | Maximum duration per Sippy stream or live GCS job; foreground starting stream rotates |
| `-ci-candidate-lookback` | — | 24h | Look back before an active release's earliest known ART FBC build start |
| `-prow-live-refresh-interval` | — | 2m | Foreground GCS discovery for pending/triggered runs Sippy may omit; 0 disables |
| `-prow-history-interval` | — | 1h | One historical-only stream per interval, after successful foreground work; 0 disables |
| `-prow-history-budget` | — | 30s | Maximum duration of that best-effort historical pass |
| `-art-current-budget` | — | 2m | Current-release ART history work per pass |
| `-art-history-interval` | — | 1h | One ART backfill stream per interval; 0 disables |
| `-art-history-budget` | — | 30s | Historical ART and source metadata budget |
| `-art-groups` | `ART_GROUPS` | empty | Configured comma-separated ART groups, e.g. `quay-3.18,quay-5.0`: the guaranteed floor, with groups derived from stored Konflux snapshots added; empty waits for configuration or snapshots |
| `-validation-plan` | `VALIDATION_PLAN` | empty | Optional legacy required-job JSON; ignored when dynamic discovery is enabled |

## Validation and source selection

Worker counts must be between 1 and 64. Sippy requires dynamic discovery.
CI/inventory/JIRA intervals and work budgets must be positive; lookback cannot
be negative. Supported zero intervals disable the named optional historical or
live refresh loop. Retention `0` disables pruning but does not remove the Sippy
adapter's maximum 90-day remote search bound.

Empty ART history, catalog, GitLab, image-scan, or ART tag URLs disable their
respective sources. JIRA reads need a read credential; writes additionally need
an explicit enable switch and separate writer credentials. Keep credentials out
of command history and Git; prefer environment variables for binary credentials
and generated Secrets for OpenShift.

See [syncing](syncing.md) for collector prerequisites and freshness semantics.
