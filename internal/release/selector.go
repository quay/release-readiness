// Package release decides which release versions are shipped and which are
// still in play, and assembles each version's release candidate and verdict,
// for the server and the sync loops alike.
package release

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/catalog"
	"github.com/quay/release-readiness/internal/db"
)

// quayVersion is a concrete version, named after its ART assembly:
// quay-v3.18.1 is assembly 3.18.1.
var quayVersion = regexp.MustCompile(`^quay-v(\d+\.\d+\.\d+)$`)

type Selector struct {
	db      *db.DB
	shipped *catalog.Shipped
	// ArtBaseURL is the ART build history the build pages link to.
	ArtBaseURL string
	// StageReleasePlanPattern matches the ReleasePlan names whose Releases
	// are image STAGE; nil selects no staged build.
	StageReleasePlanPattern *regexp.Regexp
}

// NewSelector builds the selector. A nil shipped leaves JIRA's released flag
// as the only shipped signal.
func NewSelector(database *db.DB, shipped *catalog.Shipped) *Selector {
	return &Selector{db: database, shipped: shipped}
}

// CatalogShipped reports whether the catalog publishes the fixVersion, e.g.
// quay-v3.17.5 as quay 3.17.5.
func (s *Selector) CatalogShipped(name string) bool {
	_, ok := s.CatalogShippedAt(name)
	return ok
}

// CatalogShippedAt returns when the catalog first published the fixVersion,
// and whether it has.
func (s *Selector) CatalogShippedAt(name string) (time.Time, bool) {
	product, version, _ := strings.Cut(name, "-v")
	return s.shipped.ShippedAt(product, version)
}

// ShipmentVersions returns the quay-vX.Y.Z versions whose shipment MRs are
// polled: those the overview shows as not shipped, and those whose stored MR
// is still open, since a version can ship before ART merges its MR.
func (s *Selector) ShipmentVersions(ctx context.Context) ([]string, error) {
	releases, err := s.db.ListAllReleaseVersions(ctx)
	if err != nil {
		return nil, err
	}
	mrs, err := s.db.ShipmentMRs(ctx)
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, rel := range releases {
		open := mrs[rel.Name] != nil && mrs[rel.Name].State == "opened"
		if (open || (!rel.Released && !s.CatalogShipped(rel.Name))) && quayVersion.MatchString(rel.Name) {
			versions = append(versions, rel.Name)
		}
	}
	return versions, nil
}

// IngestionActivity shares the dashboard's confirmed-publication signal while
// keeping mixed and unknown streams in the foreground. JIRA release flags alone
// remain useful when the catalog is disabled or unavailable.
func (s *Selector) IngestionActivity(ctx context.Context) (map[string]bool, error) {
	versions, err := s.db.ListAllReleaseVersions(ctx)
	if err != nil {
		return nil, err
	}
	fresh := s.shipped.Fresh(time.Now())
	activity := map[string]bool{}
	for _, v := range versions {
		if v.KonfluxApplication == "" {
			continue
		}
		active := !v.Archived && !v.Released && (!fresh || !s.CatalogShipped(v.Name))
		activity[v.KonfluxApplication] = activity[v.KonfluxApplication] || active
	}
	return activity, nil
}

// CIWindowStart joins known releases to dated ART FBCs. Upcoming releases
// exclude published z-streams; historical-only streams are eligible for paced backfill.
// Zero means no matched dated FBC, so the scheduler must not query CI.
func (s *Selector) CIWindowStart(ctx context.Context, app string) (time.Time, error) {
	versions, err := s.db.ListAllReleaseVersions(ctx)
	if err != nil {
		return time.Time{}, err
	}
	fresh := s.shipped.Fresh(time.Now())
	var active, historical []string
	for _, v := range versions {
		if v.KonfluxApplication != app {
			continue
		}
		if !v.Archived && !v.Released && (!fresh || !s.CatalogShipped(v.Name)) {
			active = append(active, v.Name)
		} else {
			historical = append(historical, v.Name)
		}
	}
	if len(active) == 0 {
		active = historical
	}
	return s.db.CandidateWindowStartForVersions(ctx, app, active)
}
