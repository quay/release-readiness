# Release Readiness Dashboard

A release-readiness dashboard for Red Hat Quay and OMR. It brings JIRA release
scope, ART builds, Konflux promotion state, Prow CI evidence, and shipment merge
requests into one place for engineering, product, and leadership teams.

The portfolio groups minor-version streams into cards and shows concrete patch
releases in stages. Each release has a briefing, validation workbench, and issue
scope. Validation distinguishes a job's outcome from evidence that it tested the
selected FBC. The dashboard observes existing CI runs; it does not launch jobs
or approve shipments.

One Go binary serves the embedded React/PatternFly UI and API, with SQLite for
persistence. Background collectors prioritize current releases over history.

## Prerequisites

- Docker Engine with the Docker Compose plugin, and GNU Make for task shortcuts.
- Node.js 24 and the Go version specified in `go.mod` for source builds/tests.
- JIRA read credentials for release discovery and a Konflux reader kubeconfig
  for build/promotion data. GitLab read access enables private shipment MR lookup.
- `oc` and Python 3.9+ for preparing the OpenShift Kustomize deployment.

## Run locally

Create a repository-root `.env` containing `JIRA_EMAIL`, `JIRA_TOKEN`, and, for
private shipment MRs, `GITLAB_TOKEN`. GitHub credentials are optional. The Compose
setup mounts `~/.kube/config` read-only; its selected context must be the Konflux
reader context. The app needs network access to its configured source services.

```bash
touch .env
chmod 600 .env
# Fill in the credentials before starting.
make up
make logs
```

Open [http://localhost:8088](http://localhost:8088). Use `RR_PORT=8089` before the
Compose command to choose another host port. The named volume preserves SQLite
data; JIRA writes are disabled. An empty database without working JIRA reads
will not yet have releases to display.

```bash
make down
```

Use `make up ENV_FILE=deploy/.env` to reuse an existing environment file.
`KONFLUX_KUBECONFIG` in that file can select a reader kubeconfig instead of
`~/.kube/config`. Use the same `ENV_FILE` for `make logs`, `make ps`, and `make down`.
The Makefile uses your existing Docker configuration. Run `make help` for all targets.

`make down` removes containers and the network while preserving the database volume.
Do not add `--volumes` unless you intend to delete its data.

## Deploy to OpenShift

Use an image built from the current code. Replace `your-org` in the example
`quay.io/your-org/release-readiness:v2.2` with your registry organization, and
`your-namespace` with your target OpenShift project. Follow the
[deployment guide](deploy/README.md) for credentials, cluster selection, storage,
and access controls. In the intended application-cluster context:

```bash
# Create the Secrets from deploy/README.md, set the image in deploy/app.yaml,
# then:
oc apply -n your-namespace -f deploy/app.yaml
oc -n your-namespace rollout status deployment/release-readiness --timeout=300s
oc -n your-namespace get route release-readiness -o jsonpath='https://{.spec.host}{"\n"}'
```

The deployment includes the app, persistent storage, Service, TLS Route, and
Secrets. The app has no built-in login layer; use your organization's access
controls. Back up the PVC before upgrades; supported database migrations run
in place.

## Build

Build the frontend first: Go embeds `web/dist/`.

```bash
npm --prefix web ci
npm --prefix web run build
go build -o release-readiness ./cmd/release-readiness/
```

To build a container image:

```bash
make build IMAGE=quay.io/your-org/release-readiness:latest VERSION=v2.2
```

`make build` builds the Docker image without publishing it;
`make push IMAGE=quay.io/your-org/release-readiness:latest` pushes the existing tag.
The default image is local-only `release-readiness:latest`; `VERSION` controls
the application version independently of the image tag.
Use a unique tag or digest for reproducible production rollouts.

## Test

For a fresh checkout, run `make deps` once. `make test` builds the embedded web
bundle and runs Go tests/vet, web unit tests/lint, and Playwright. Individual
targets are `test-go`, `test-web`, and `test-e2e`; `make lint` also runs
golangci-lint. The equivalent direct commands
are:

```bash
npm --prefix web run build
go test ./...
go vet ./...
golangci-lint run
npm --prefix web test
npm --prefix web run lint
npm --prefix web run typecheck:e2e
(cd web && npx playwright install chromium && npm run test:e2e)
python3 -m unittest discover -s deploy -p 'test_*.py'
```

Use a Go-compatible `golangci-lint` version. Deployment render tests need `oc` and
PyYAML. Browser tests use mocked APIs and a temporary frontend, without backend
credentials. See the [Playwright guide](web/e2e/README.md) for screenshots,
accessibility checks, and comparison tools.

## Documentation

- [Architecture](docs/architecture.md): components, workflow diagram, persistence.
- [How readiness works](docs/how-it-works.md): portfolio, releases, FBC matching,
  validation, shipment status, and API semantics.
- [Synchronization](docs/syncing.md): sources, scheduling, caching, history,
  retention, and freshness.
- [CLI and configuration reference](docs/cli-reference.md): flags, environment
  mappings, defaults, and deployment configuration boundaries.
- [OpenShift deployment](deploy/README.md): Kustomize, credentials, upgrades.
- [Frontend tests and review tools](web/e2e/README.md): browser testing and visuals.
