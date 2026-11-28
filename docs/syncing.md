# Source synchronization

[Back to the project README](../README.md)

## Collector overview

ART build history and Konflux ingest independently. JIRA stores release versions
and issues; the shipment collector uses those releases to resolve GitLab MRs.
A source failure does not disable unrelated collectors.

| Source | Starts when | Default cadence |
| --- | --- | --- |
| Konflux snapshots/releases | A Kubernetes client can be built | 30s |
| JIRA release/issues | JIRA read token is set | 5m |
| GitLab shipment MRs | GitLab URL is nonempty | 5m |
| ART image resolution | ART history URL is nonempty | 5m |
| ART current history | ART history URL and configured groups | 2m |
| ART historical backfill | Enabled history interval; after current work | 1h |
| FBC rendering/reconciliation | ART history URL is nonempty | 10m |
| Prow inventory | Dynamic discovery enabled | 10m |
| CI runs | Discovery or legacy jobs/pattern configured | 15m binary; 2m deployment |
| Historical CI | Enabled interval; after successful foreground work | 1h |
| Catalog publication | Catalog URL is nonempty | 1h |
| Image scans | Results URL, ART, and Kubernetes bearer credential | 5m |
| Retention | Positive retention duration | Daily |

Cadences are distinct from per-stream time budgets. Unconfigured ART groups
report an idle state rather than discovering streams from another collector.
Konflux initialization failures are exposed as source failures.

### Konflux sync (default: every 30s)

Lists `Snapshot` and `Release` resources (`appstudio.redhat.com/v1alpha1`) in the `-namespace` Konflux namespace through the Kubernetes API, using `-kubeconfig`/`KUBECONFIG` or, when unset, the in-cluster service account. New snapshots are stored with their component images; releases are upserted. Each record's `created_at` is the resource's `creationTimestamp`.

### JIRA sync (default: every 5m)

Discovers active releases by querying for JIRA issues with the `-area/release` component that are not Closed/Done. Parses the version from the ticket summary (e.g. "Release Quay v3.16.2") and syncs all issues whose Target Version equals the release version (e.g. `quay-v3.16.2`).

### Prow inventory and CI evidence

Dynamic discovery imports periodic job definitions from `openshift/release`.
Sippy supplies matching run indexes; GCS supplies completion and exact tested-image
identity evidence. See the scheduling section below for candidate windows,
caching, retries, and the live-run fallback. The binary polls CI every 15 minutes
by default; Compose and Kustomize set it to 2 minutes. Neither reads the generator
matrix or launches jobs.

Legacy job-name discovery remains available when dynamic discovery is disabled.
For older runs without `tested-images.json`, `olm-csvs.yaml` can provide image
information but cannot prove an exact FBC catalog digest. Installation-step
completion is read when needed to distinguish installation failures.

### ART build history (current releases every 2m)

For each ART group in the union of the explicitly configured groups (`-art-groups` / `ART_GROUPS`, for example `quay-3.18,quay-5.0`) and the groups derived from the applications of stored Konflux snapshots (an application `quay-3-18` names the group `quay-3.18`), lists ART's successful FBC builds day by day: the newest 3 days during current-release passes, with up to 7 older unscanned days in separately scheduled backfill, back 120 days. An FBC is stored once the bundle it holds and the operator and operand images that bundle deploys are stored. A day listing at the 1000-row search cap is split by build name, the operators' FBC names and any the listing shows; a name that alone reaches the cap, or a search warning, is a sync error and the day is scanned again.

The configured groups are the guaranteed floor of that union: ART history keeps scanning them through a Konflux outage and never depends on JIRA releases or Prow definitions for them. Snapshot-derived groups are additive hints — discovery can only add groups, never remove them, and a discovery failure is only logged while configured groups exist. An empty union, with neither configuration nor stored snapshots, reports waiting without claiming a successful scan. JIRA/catalog lifecycle metadata only sets priority; missing priority metadata leaves configured groups active. Compose supplies the Quay group list through `RR_ART_GROUPS`.

### FBC render (every 10m, with `-art-build-history-url`)

Reads up to 20 FBC images a pass anonymously from their registry (Registry v2 API): the catalogs of staged FBC Snapshots, then the newest ART FBC build of each operator, version and OCP version. It keeps, by digest, the image's `com.redhat.art.nvr` label and the head bundle of its `stable-X.Y` channel; a failed read is retried after an hour, except one whose catalog has no `stable-X.Y` channel: the digest pins the catalog, so that read is never retried. Each head bundle is joined to its ART bundle build, read from ART build history when not yet stored.

### FBC JIRA reconciler (every 10m, with `-art-build-history-url`)

For each successful ART FBC build in a `quay-X.Y` group, of any stream `X.Y`, whose NVR carries an `X.Y.Z` version, finds the PROJQUAY issues the FBC fixes and stores them as actions for the JIRA writer. FBCs are grouped by stream, operator and OCP version; the newest FBC of a group when first seen is its baseline and proposes nothing. Each later FBC is compared with the one before: for each member image whose digest changed and whose ART build is an allowlisted component from its listed repository, the commits between the two builds' commits on the stream's branch (`redhat-X.Y`; for Clair, the one `release-*` branch whose head is each commit, else that contains it, the same for both, or the FBC is uncertain) are read from the GitHub compare API, and each PROJQUAY key in their subject lines is `proposed` for `quay-vX.Y.Z`, or `skipped` when an earlier FBC of the stream already included it. Anything that keeps an FBC's fixes from being known (such as a component not allowlisted, a GitHub error other than a rate limit, 5xx or network error, a revert of a key, a missing ART source, an FBC rendered after a later one was reconciled) is logged as the FBC's uncertainty instead. An FBC with no group (not one ART name, or a failed render) proposes nothing and is logged as uncertain, and so is the next FBC of its stream, operator and OCP version, whose diff would take its fixes. A build not rendered that a later build of the same operator, version and OCP version supersedes, and that ART did not stage, is never rendered and is skipped: the next build of its slot takes its fixes. Any other FBC whose catalog is not rendered yet, or whose ART source records are not yet checked, or not yet stored within 6 hours of its build starting, or whose GitHub read hit a rate limit, 5xx or network error, waits and is retried next pass, and so do the FBCs after it. `-github-token` reads GitHub authenticated; without it anonymous GitHub rate limits apply. Actions are not pruned by `-retention`. Its sync status is `reconcile`.

### FBC JIRA writer (off until `-jira-write`)

After each reconcile pass, applies the proposed actions to their PROJQUAY issues, re-reading each issue before every step: replaces the stream's `quay-vX.Y.z` Target Version with the release's `quay-vX.Y.Z` (keeping every other Target Version, and skipping the issue when that version is missing, archived or released), moves a MODIFIED, ON_QA or Verified issue to Release Pending by a direct transition only, recording an error when none is offered (an issue already there, e.g. moved by release-tool, is never transitioned, and any other status is left alone), then comments "This fix is in ART FBC build `<NVR>` for `quay-vX.Y.Z`." with links to the ART build pages of the FBC and of the quay container in it, once: an issue whose comments carry that FBC build link, or the marker of the earlier comment text, is not commented again. Each step's result is stored on the action, so a failed one is retried alone, up to 5 attempts. It stays off until `-jira-write` is true and its own credential (`-jira-write-email`, `-jira-write-token`), never the read one, is set. Setting `-jira-write` false is the clean stop; revoking that token also stops every write without touching the read sync, but each pass then spends an attempt of every pending action, which stays in error after 5 passes.

### Sync status

`/api/v1/sync-status` lists as `problems` each enabled sync (including inventory, foreground collection, history, and retention) whose last pass failed or that has had no success within 3 poll intervals (at least 10 minutes), and as `sources` every enabled sync with its `last_success`, null before its first success; the masthead shows "Updated N ago" and "N sources stale" on every page, icon-only at narrow widths (image-scans only while CVEs are shown).

## Candidate-driven scheduling and evidence

[`compose.yaml`](../compose.yaml) enables dynamic job discovery and Sippy-assisted ingestion, polls CI every two minutes, and keeps JIRA writing disabled. The binary remains opt-in for these integrations for compatibility with existing deployments:

```sh
release-readiness -prow-job-discovery \
  -sippy-url https://sippy.dptools.openshift.org \
  -prow-interval 2m
```

See [the CLI reference](cli-reference.md) for all intervals, worker counts, and budgets.

Sippy and ART ingestion prioritize streams with upcoming releases using both JIRA lifecycle flags and the dashboard's catalog publication data. A published-only stream is historical even if JIRA still says unreleased. Mixed streams and unknown streams stay foreground. Priority reads use stored lifecycle metadata without waiting for catalog network access; a failed or older-than-three-hours catalog snapshot does not demote work. Published z-streams are excluded from active CI lookup windows when the catalog is fresh.

Known historical-only Prow streams rotate one at a time after successful foreground work, with an independent interval and time budget; no legacy GCS listing is performed for them. History errors are reported separately as `prow-history` and do not invalidate current-release `prow` freshness.

Sippy supplies release-scoped paginated run indexes. The search starts around the earliest dated FBC of active releases in the stream, with the configured lookback and retention boundary. A release without a dated FBC schedules no lookup. Other z-streams with dated FBCs can still proceed, sharing one query per stream. Job inventory remains metadata-only; it does not itself trigger CI fetching. Both timezone-qualified and timezone-less UTC ART timestamps are accepted. The adapter re-reads that bounded window to accommodate late indexing; it does not advance a timestamp-only cursor that could skip late runs. A maximum of 200 pages of 100 runs per stream fails explicitly if exceeded. Sippy's recent history is capped at 90 days even when app retention is disabled; already stored evidence remains governed by app retention. ART timestamps are build start times, not claimed publication timestamps, so they provide a conservative search boundary.

For indexed runs, `tested-images.json` still establishes catalog/bundle/component identity; `finished.json` supplies authoritative terminal result and completion time. A run inside the time window is not proof that it tested the candidate. Unknown/changed targets use `prowjob.json` fallback; older CSV evidence remains supported but cannot establish an exact catalog match. Missing artifacts are retried rather than permanently cached. Complete exact-identity evidence is cached in SQLite across restarts. Fetch caching is separate from readiness: completed runs under 24 hours old retry incomplete evidence after 2 minutes; older legacy/image evidence without a catalog identity is revisited after 24 hours, and older missing/invalid evidence after 6 hours. Running/unknown runs remain eligible each pass. The persisted fetch time preserves these delays across restarts, and late evidence is eventually revisited. A deferred fetch never makes a job pass. Artifact writes use transactions of at most 20 runs; network fetches occur outside those transactions.

The normal Sippy path avoids GCS directory listings. The separately paced live refresh runs immediately and every 2 minutes by default, across foreground jobs using the bounded worker pool. It discovers queued/running jobs Sippy may omit, using GCS listings scoped to the matched candidate window and retention; stored completed runs are skipped and out-of-window run artifacts are not read. Both readers check `tested-images.json` while a job is active, so a published exact FBC identity can show Running before completion. If CI has not uploaded that artifact yet, identity remains unverified; job names or mutable image tags cannot establish the selected digest. A newer confirmed-FBC rerun shows Running until it finishes; an older unfinished run does not hide a later result. Sippy failures leave stored evidence intact and report the `prow` source as unhealthy. They do not silently fall back to a green result. Source errors and freshness appear in `/api/v1/sync-status`. When no current FBC has configured jobs, the source reports an `idle_reason` without inventing a successful fetch or generating a stale-source warning.

Worker queues are bounded and cancellation-aware, with serial persistence within each source batch. ART resolver/history/scan calls share a one-request-per-second client limit. Current ART history runs every 2 minutes, scanning the newest 3 days of foreground streams with a 2-minute budget. Starting stream rotates to avoid starvation. Historical work runs afterward, rotating one stream per hour with a 30-second budget and at most 7 days; source metadata backfill shares that budget. Its `art-backfill` status is separate from current-release `art-history`. An intentional time-budget pause is logged as paused, without claiming success or creating a source error; real upstream failures remain visible. Partial days are never marked complete. FBC rendering and operand-resolution failures remain explicit and are not converted into success. JIRA keeps full per-version issue reconciliation and existing 429 retry behavior. No page or partial JIRA response is treated as a complete issue scope.

Each Prow stream logs total duration, Sippy duration/pages, GCS request count, cached/deferred runs and persisted results. ART logs phase durations, current days completed, and backfill failures. The validation API includes an `evidence` explanation on each job row. Missing results distinguish no ingested run, unavailable artifacts, unverified identity, a different FBC, and incomplete candidate correlation, with an observed-run link when available. Separately paced historical sources do not determine the header's foreground “Updated” time. Release pages show pending background history separately from current FBC loading, so history alone does not keep a loading spinner active.

The GitHub API is used for the commit and directory indexes; public YAML files are fetched from commit-pinned raw URLs without forwarding the GitHub token. `GITHUB_TOKEN` is optional for public inventory access. In discovery mode the imported inventory selects the jobs.

For local startup, run `docker compose up --build`. To keep dynamic configuration but use the legacy GCS reader, set `RR_SIPPY_URL=''`. These overrides apply to local Compose only.

Verification uses only loopback API mocks: `go test -race ./internal/prow ./internal/db ./internal/release ./internal/server ./internal/jira ./internal/artbuild ./internal/workpool`. Integration tests cover batching, persistent caches, late artifacts, pagination failures, candidate windows and inventory freshness. Local runtime source health is tracked separately from these deterministic mock tests.

### Collection and matching order

JIRA release tickets identify release versions and their issues. The shipment MR
sync uses those stored release versions to resolve the assembly's GitLab MR;
GitLab failures do not prevent JIRA updates. ART history and Konflux stay separate:
ART's configured groups supply build/FBC identities, while Konflux supplies
snapshots and promotion outcomes. No source network call happens in an HTTP
page request to schedule CI.

The periodic inventory sync stores job names, metadata and configuration links.
On each background CI pass, the selector joins known releases to ART FBC versions
and dates before scheduling matching stream jobs. Sippy indexes the resulting
window and GCS `tested-images.json` confirms the tested identity. A timestamp or
matching version alone never passes the exact-FBC gate; observed CI results and
different/newer FBC warnings remain separate. Historical-only streams are lower
priority. Nothing in this workflow triggers new Prow executions.
