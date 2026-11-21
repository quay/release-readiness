# Deploying Release Readiness on OpenShift

`app.yaml` is a generic sample manifest for the whole setup: the data
PVC, the injected trusted CA, Service, HTTPS Route, and the Deployment
the app runs with in production — dynamic Prow job discovery with
Sippy, explicit ART groups, accelerated history backfill, JIRA read-only
by default (the FBC JIRA writer is opt-in), and a mounted Konflux reader
kubeconfig. It contains no
credentials and no cluster-specific settings: Secrets are referenced by
name only, and storage class, namespace, and route host stay with the
deploying cluster.

## What you need before deploying

1. **An image.** Build and push it from this repository:

   ```bash
   make build IMAGE=quay.io/your-org/release-readiness:latest
   make push  IMAGE=quay.io/your-org/release-readiness:latest
   ```

   Then set that reference as `image:` in `app.yaml`. To serve the app
   under a URL prefix, build with `VITE_BASE=<prefix>` and set `BASE_PATH`
   in `app.yaml` to the same value.

2. **JIRA read credentials** (required — releases are discovered from
   JIRA; without them the portfolio stays empty):

   ```bash
   oc create secret generic release-readiness-jira \
     --from-literal=JIRA_EMAIL=<email> \
     --from-literal=JIRA_TOKEN=<api-token>
   ```

3. **A Konflux reader kubeconfig** (required for snapshots, releases and
   image scans). A self-contained, minified kubeconfig whose identity can
   list Snapshots and Releases in `KONFLUX_NAMESPACE` and read Tekton
   Results:

   ```bash
   oc --kubeconfig=<reader-context> config view --minify --flatten --raw \
     > konflux-reader.kubeconfig
   oc create secret generic release-readiness-konflux-kubeconfig \
     --from-file=kubeconfig=konflux-reader.kubeconfig
   rm konflux-reader.kubeconfig
   ```

   [rbac.yaml](rbac.yaml) documents the permissions that identity needs,
   in the Konflux namespace. Bind them to whatever the kubeconfig holds —
   a User, a Group, or a ServiceAccount.

4. **Optional tokens:** `release-readiness-gitlab` and
   `release-readiness-github`, each with a `token` key, for shipment MRs
   and GitHub rate limits.

5. **Optional, the FBC JIRA writer:** production runs it. Create
   `release-readiness-jira-write` with `email` and `token` keys (a
   credential separate from the read one), then set `JIRA_WRITE=true` on
   the deployment — `app.yaml` already wires the optional secret refs.
   Without it, the reconciler's proposed actions are logged only.

## Deploy

```bash
oc apply -n <namespace> -f deploy/app.yaml
oc rollout status deployment/release-readiness -n <namespace>
oc get route release-readiness -n <namespace> -o jsonpath='{.spec.host}{"\n"}'
```

## What to verify after deploying

Work through this list; every item is observable without cluster-admin
access. An AI agent can check all of them with `oc` and `curl` alone.

1. **The pod is healthy.** `oc get pods -n <namespace>` shows the pod
   `Running` and ready; `curl https://<route>/api/v1/health` returns 200.
   A crash loop here is usually a bad JIRA secret or an unreadable
   kubeconfig volume.

2. **JIRA sync works.** `curl https://<route>/api/v1/sync-status` lists
   the `jira` source with a recent `last_success`. Then
   `curl https://<route>/api/v1/releases/overview` returns releases —
   an empty list with a healthy JIRA source means the JIRA project has
   no open release tickets.

3. **Konflux sync works.** The `konflux` source shows a recent
   `last_success` and the pod log (`oc logs deployment/release-readiness`)
   has no `Unauthorized` lines. A 401/403 here means the reader identity
   lacks the RBAC in [rbac.yaml](rbac.yaml) — the Konflux namespace admin
   must grant it.

4. **The Prow inventory loaded.** The `prow-inventory` source has a
   `last_success`, and the log line `imported periodic jobs` (or the
   validation workbench on any release page) shows the discovered job
   count. Zero jobs for a stream renders as "No CI configured" — that is
   a data state, not an error.

5. **ART history is scanning.** The `art-history` source succeeds and
   the log shows `ART scheduling` with the group list from `ART_GROUPS`
   plus any snapshot-derived groups. An `ART waiting for stream
   discovery` message means `ART_GROUPS` is empty and no snapshots are
   stored yet.

6. **The catalog and shipment sources.** `catalog` and `shipment-mrs`
   (the latter only with a GitLab token) show successes. Failures here
   degrade specific fields — shipped dates, MR links — without breaking
   the rest.

7. **The route and theme.** Load `https://<route>/` once in a browser:
   the portfolio table renders, the theme toggle works, and the browser
   console is free of errors.

## Things to adjust per cluster

- **Storage class** — `app.yaml` leaves the PVC on the cluster default;
  patch `storageClassName` where the default is unsuitable (some PaaS
  clusters only admit specific classes).
- **`KONFLUX_NAMESPACE`** — defaults to `art-quay-tenant`; set it to the
  tenant your Konflux apps live in.
- **`ART_GROUPS`** — the guaranteed floor of ART stream groups; snapshot
  discovery adds more automatically, so this only needs the streams you
  want scanned even before any snapshot exists.
- **Route host** — leave it unset for the cluster default, or set
  `spec.host` to share a hostname with path-based routing.
