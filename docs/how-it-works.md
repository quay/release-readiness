# How release readiness works

[Back to the project README](../README.md)

## Portfolio and release workspace

The UI groups product streams into cards and concrete z-streams into release stages. `GET /api/v1/releases/overview?portfolio=1` adds FBC preparation verdicts and required-job coverage alongside the base overview response. `shipped` means JIRA shows the release done — a closed release ticket, or the project version marked released; a catalog image push alone does not release a version, and a release never un-happens. `published` means the catalog lists a released version's tag, the confirmation customers can pull it.

`GET /api/v1/releases/{version}/validation` returns the required-job plan and exact-candidate results. With `-prow-job-discovery` (`PROW_JOB_DISCOVERY=true`), the app imports release periodics from both `ci-operator/jobs/quay/quay` and `ci-operator/jobs/quay/quay-operator`, pinned to one openshift/release commit. The complete inventory is stored in SQLite and published atomically. Jobs with no runs remain required. Only current periodic jobs define coverage; removed definitions remain in SQLite for audit but no longer count as configured requirements. Failed or partial imports preserve the last good inventory. Discovery does not read `_generator/matrix.yaml`, generated ci-operator configs, or the old generator-derived JSON snapshot. `-validation-plan` is ignored in discovery mode and remains available only for legacy ingestion. A complete inventory with no jobs for a stream produces “No CI configured” on its card and workbench; an inventory that has never loaded is shown as unavailable. Zero jobs never establishes readiness. No run status is embedded in the plan.

Readiness requires the full declared denominator, passing latest completed exact-FBC runs, successful/current Prow, Konflux, ART-history and FBC-render sources (plus periodic inventory when enabled), and the existing FBC preparation verdict. Equivalent bundles and missing standalone identity evidence do not satisfy the gate. An incomplete or unavailable plan never means ready. The readiness projection is advisory and does not execute a shipment or write to JIRA.

The portfolio starts with all products. Cards stay at `x.y`, with the next `x.y.z`, latest available release, and expandable z-stream records. In Cards view, **Show available releases** includes streams whose releases are all confirmed published; these streams are hidden by default. Streams with pending or publication-unconfirmed releases remain visible, including their latest available z-stream details. The choice is preserved in the URL and does not filter Stages view. Stages display concrete `x.y.z` releases; **Show history** adds older records. Opening either goes to the same exact-release workspace: release briefing, validation workbench, and searchable JIRA issues. Links preserve the portfolio layout, product, and history selection. Motion respects the system reduced-motion preference.

See [the backend workflow diagram](architecture.md) for collection and matching.

Dynamic periodic discovery establishes the expected denominator and schedules CI ingestion for matching release candidates. Streams without matching configured periodics show “No CI configured.” The optional legacy `-validation-plan` mode requires an operator-supplied plan. Exact-FBC identity is the supported gating adapter; standalone/image-only jobs need suitable image-identity evidence. The UI distinguishes missing historical FBC records or stream fallback FBCs from confirmed release-specific publication.

## Release and API data semantics

Each JIRA release maps to a Konflux application by major.minor version: fixVersion `quay-v3.16.2` maps to `quay-3-16`, and `omr-v2.0.10` to `omr-2-0`.

A Quay version's builds are its application's stream Snapshots (not ART's assemblies or `fbc-ri-*` re-releases) whose images, base image excluded, carry that version in the NVR ART build history resolved for them: `quay-quay-container-3.18.1-...` is 3.18.1. Unresolved images do not count, a build carrying two versions, as while ART switches the stream, is of none, and a build counts only if ART built no lower version after it.

### Candidate and FBC selection

`/api/v1/releases/{version}/candidate` returns the version's operator FBCs up for release in `fbc_candidate`, and as its stream built them in `fbc_stream`; both are null while the version has no FBC built or staged. `fbc_candidate` takes each operator and OCP version slot's FBC from the newest FBC Snapshot ART staged for stage (`source` `stage_snapshots`), else from the newest successful ART build (`stream`). An operator's slots are the OCP versions any of its FBCs of the version was built or staged for.

### Operator coverage and bundle identity

`operators` always holds quay-operator, container-security-operator and quay-bridge-operator, each with the `head_bundle_nvr`, `channel` and `head_csv` of its newest FBC whose head bundle is joined, else, with none joined, of its newest FBC whose head bundle digest the image shipment holds, its slots' FBCs on that bundle in `members` and the others in `conflicts`, the slots without an FBC in `missing_ocp`, and `coverage`. Its `fbc_ri_stage` is `in` when a stage Release holding its head bundle succeeded, an `fbc-ri-*` Snapshot's or the version's image shipment's, else `failed` when one failed, else `none`, or `unknown` without a head bundle; `image_check` compares its head bundle with that of the version's STAGE build, else its newest build: `matches`, `differs`, or `absent` when either is unknown; without a head bundle NVR it `matches` when that build holds the digest its `members` head, given in `bundle_digest`. Each FBC's `head_bundle_digest` is the digest of the bundle heading its release channel, empty until rendered. `stage` counts its `members` whose newest stage Release succeeded, and, on `fbc_candidate`, each FBC's `staged` counts the staged FBC Snapshots of its slot whose newest Release succeeded.

### Images and comparisons

`images` lists its head bundle's relatedImages, by name, from its newest-OCP member FBC that was read, each with its ART build's `nvr`, `commit` and `build_url` (its ART build page), empty for an image ART did not build, and `stage` or `prod` set when a succeeded stage or prod Release holds its digest, and its CVE `scan`, null until the scan sync reads it. `compare` compares them with its FBC's previous channel entry (`previous`) and, on `fbc_candidate`, with the stream's head bundle when that is another (`newer`); each row is `same`, `rebuild` (same commit), `new source`, `new build` (a commit unknown), `added` or `removed`.

### Candidate state and verdict

`state` is `inconsistent` when an operator's FBCs head more than one bundle, `incomplete` when an operator or slot has no FBC, `unknown` while a head bundle or a staged catalog's identity is unresolved (staged catalogs no NVR names are in `unknown`), else `complete`; `errors` names each operator missing or heading more than one bundle. `pullspecs`, release-from-fbc's fbc-pullspecs by operator, then OCP version, is empty unless `complete`. `reading` counts the shown FBCs' reads as `read`, `failed` and `pending`. `verdict.state` is `shipped` for a shipped version, whose `fbc_candidate` is the record of what shipped, with no `missing_ocp` and no `pullspecs`; else `checking` while a read is pending, `not_ready` with a blocker, or `ready`. `verdict.items` lists blockers, then warnings, then info, each with its `operator` (empty for the release), `owner` (`ART`, `ART/Konflux`, `Quay (release-from-fbc)`, `Quay/ART`, `RR` or empty), `code` and `text`. Blockers: `not_staged` on a stream candidate; `missing_operator`, `missing_ocp`, `read_failed`, `no_head_bundle`, `stage_release_failed` and `unknown_fbc` on a staged one; `csv_mismatch`, `mixed_builds`, `image_mismatch` and `stage_push_failed`, when an operator's `fbc_ri_stage` is `failed`, on either; `stream_ahead` is info when the stream's newer bundle only rebuilds images, else a warning. `bundle_not_in_art`, owner `RR`, is one warning per operator whose staged FBCs head, in place of a bundle ART's history knows, the bundle of `image_check.bundle_digest`; such FBCs have no `no_head_bundle`.

### CI evidence and builds

Each FBC's `ci` lists, by job, the periodic runs that installed from its digest (`exact`) and from another FBC of its operator, OCP version and head bundle (`equivalent`). `candidate` is the version's selected STAGE build with its `prod` Release, or null with the `reason`, e.g. `no release candidate yet`, or, for a version without a build while ART's newest build whose version resolves is of a lower one, `no 3.18.2 builds yet: ART still builds 3.18.1`. `catalogs` lists the version's FBC catalogs: each of its assembly's stage FBC Snapshots, `staged`, with its `snapshot`, `pullspec`, newest `release` and `release_plan`, then, for each operator and OCP version, the newest version-matched stream catalog ART did not stage, `built`. `unshipped_builds` lists, with a candidate, the newest image of the version of each component its stage Snapshot leaves out, by component, and is null without one.

### Portfolio dates

On `/api/v1/releases/overview`, a version's `latest_build` is when the newer of its newest build and its STAGE build was created, omitted when it has neither. Its `shipped_at` is when the Red Hat catalog first published the version, the earliest push date across the product's repositories of the images tagged `X.Y.Z` or `X.Y.Z-<build>`, so a respin keeps the first date; it is omitted when the catalog has not published the version or gave no push date, and a version only JIRA marks released has none.

### Release input tags

`/api/v1/releases/{version}/fbc-tags` returns the version's operator FBCs as the tags ART publishes them under in its public `quay.io/redhat-user-workloads/ocp-art-tenant/art-fbc` repository, which release-from-fbc takes as `fbc-pullspecs`, for the Release inputs dialog. The server lists each operator's tags with the unauthenticated Quay tag API (`filter_tag_name=like:<operator>-fbc-<X.Y.Z>`, newest first, up to 10 pages of 100) and keeps them for two minutes; the browser never calls quay.io. It looks up one tag per operator and OCP version slot of the version's `fbc_candidate` (`source` `candidate`), or, with no candidate, of each operator at every `OCP_TARGET_VERSIONS` of the stream's `group.yml` in ocp-build-data (`ocp-build-data`). A tag is always a multi-arch tag matching `<operator>-fbc-<X.Y.Z>-<14-digit timestamp>.ocp<X.Y>`: the per-architecture tags (`-linux-arm64` and so on) and the base tags without a timestamp, which move, are never used. For a slot of a candidate ART staged in the FBC Snapshots (`fbc_candidate.source` `stage_snapshots`), the tag is the one whose manifest `digest` is the slot's staged digest, found by reading pages until it appears (a staged NVR that names such a tag is looked up on its own first, and used only when its digest is the staged one); `basis` is `staged`. If no tag points at the staged digest the slot is an error in `errors` naming the operator, the OCP version and the digest, and the newest tag is never taken in its place. When ART has built the slot again since, `newer_tag` names the newest tag and `info` says so; the tag listed stays the staged one. A slot with no staged digest (a candidate that is only what the stream built, or no candidate) takes the newest tag by start time, `basis` `newest`, with a line in `warnings`: "newest ART build, not staged in RR". `tags` lists the slots in `operators` order, then OCP version, each with its `pullspec` (`repository:tag`), the tag's manifest `digest`, and the slot's `staged_digest` and `staged_nvr` (empty for none). `pullspecs`, the tags' pullspecs in the same order, is empty while `errors` is not. The endpoint answers 404 for a version that is not a concrete `quay-vX.Y.Z` with a Konflux application, 503 when `-art-fbc-quay-url` is empty and 502 when Quay or ocp-build-data can't be read.

### Issues and CI history

`/api/v1/releases/{version}/issues` lists the version's Target Version tickets, by key. Each has its Jira `status` as of the last JIRA sync.

`/api/v1/releases/{version}/ci` lists the version's ART FBC builds, newest first, each with its `state` (`shipped`, else `candidate`, else `staged`, else empty) and the periodic runs that installed from its catalog digest.

## Reading a result

A job's observed outcome and the selected release's readiness are separate facts.
The dashboard can show a successful run while explaining that it used another
FBC. Only evidence matching the selected FBC establishes its validation result.
A matching active run is Running; missing or mismatched evidence is explained
instead of being treated as a pass. No configured jobs is not a green release.

The dashboard observes existing Prow jobs and shipment MRs. It neither launches
CI jobs nor creates/approves shipments. The optional JIRA writer is disabled in
the supplied Compose and OpenShift setups; see [syncing](syncing.md).

## Validation job history

**View history** on a validation row opens that job's stored runs in a side drawer.
**All CI history** opens the same drawer across jobs. The default scope includes
only verified catalog identities matching selected FBC members. **Include other
and unverified FBCs** adds observations from the same product stream, including
other z-streams and runs without identity artifacts. These observations never
change the readiness denominator or verdict. Each FBC group shows its build
identity, digest, run results, UTC start times, and Prow links. Totals describe
runs on the current page, not required checks.

`GET /api/v1/releases/{version}/ci-history` reads SQLite only. It accepts an
optional exact `job`, `scope=selected|all` (default `selected`), `limit` (default
50, maximum 100), and opaque `cursor`. The response contains `runs`,
`selected_fbcs`, optional configured `job` metadata, and `next_cursor` when more
runs exist. Pagination orders by start time, job name, and build ID. Candidate
selection reuses the release selector without loading the entire CI run history.
Missing identity is labeled unverified even when the observed job succeeded.
Existing FBC-cell history links continue to use the build-grouped `/ci` API.
