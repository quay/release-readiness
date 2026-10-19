package reconcile

import (
	"context"
	"time"
)

// Action statuses.
const (
	StatusProposed = "proposed"
	StatusSkipped  = "skipped"
	StatusApplied  = "applied"
	StatusError    = "error"
)

// ActionKey identifies an action: an issue an FBC build fixes.
type ActionKey struct {
	FBCDigest, IssueKey string
	// Release is the FBC build's NVR. Rows from before builds were the input
	// hold the name of the Konflux Release that staged it.
	Release string
}

// Action is what the reconciler decided for an issue in an FBC, with the
// commits behind it.
type Action struct {
	Key                                      ActionKey
	Assembly, FBCNVR, EvidenceJSON           string
	DesiredTargetVersion, Status             string
	SkipReason, Error                        string
	VersionStep, TransitionStep, CommentStep string
	Attempts                                 int
	UpdatedAt                                time.Time
}

// FBCBuild is one successful ART FBC build.
type FBCBuild struct {
	// StartTime is when ART started the build, in UTC with no zone.
	Group, NVR, Digest, StartTime string
	// RenderState is the image's render state, "" before its first render.
	RenderState string
	// Staged is whether an FBC Snapshot ART staged holds the image.
	Staged bool
}

// Source is an ART build, by its ART name, with the public repository and
// commit it was built from.
type Source struct {
	Name, SourceRepo, Commitish string
}

// ARTRecord is an ART build history record of an image digest.
type ARTRecord struct {
	Digest, NVR, RecordID string
}

// Baseline is the first FBC a stream/operator/OCP slot was reconciled from.
type Baseline struct {
	Stream, Operator, OCP, FBCDigest string
}

// Store is the database layer the reconciler and writer need.
type Store interface {
	// ListFBCBuilds returns every FBCBuild, oldest first.
	ListFBCBuilds(ctx context.Context) ([]FBCBuild, error)
	// FBCMemberDigests returns the digests of the FBC's head bundle, its
	// related images and the FBC itself.
	FBCMemberDigests(ctx context.Context, fbcDigest string) ([]string, error)
	// ARTRecords returns the ART records of digests, newest first per
	// digest.
	ARTRecords(ctx context.Context, digests []string) ([]ARTRecord, error)
	// SourcesByDigest returns the records of an image digest that have a
	// source repo.
	SourcesByDigest(context.Context, string) ([]Source, error)
	// SourceState reports whether an image digest has stored ART records,
	// and whether one of them is not checked for a source repo yet.
	SourceState(ctx context.Context, digest string) (stored, unchecked bool, err error)
	// GetBaseline returns the FBC digest of a slot's baseline.
	GetBaseline(ctx context.Context, stream, operator, ocp string) (string, bool, error)
	// InsertBaseline stores b unless its slot has a baseline already.
	InsertBaseline(context.Context, Baseline) error
	// RunDone reports whether the FBC build nvr was reconciled.
	RunDone(ctx context.Context, nvr, fbcDigest string) (bool, error)
	MarkRun(ctx context.Context, nvr, fbcDigest string) error
	// UpsertAction stores a. Without attempts, a leaves an existing
	// proposed or attempted row as it is; with them, a replaces only a
	// proposed or error row, from no later attempt.
	UpsertAction(context.Context, Action) error
	// PendingActions returns the actions still to write: proposed, or
	// error after fewer than maxAttempts attempts.
	PendingActions(ctx context.Context, maxAttempts int) ([]Action, error)
	// FirstIncludedAction returns the earliest proposed, applied or error
	// action for issueKey in an assembly of stream.
	FirstIncludedAction(ctx context.Context, stream, issueKey string) (Action, bool, error)
}
