package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	"github.com/WuKongIM/WuKongIM/pkg/db/transfer"
)

// Plan binds offline inputs and the target generation before any source scan.
// source_commit is operator-supplied deployment evidence, not binary detection.
type Plan struct {
	Version      int           `json:"version"`
	SourceCommit string        `json:"source_commit"`
	Sources      []NodeOptions `json:"sources"`
	Target       TargetPlan    `json:"target"`
}

func ReadPlan(reader io.Reader, sourceCommit string) (plan Plan, err error) {
	d := json.NewDecoder(io.LimitReader(reader, (1<<20)+1))
	d.DisallowUnknownFields()
	if err := d.Decode(&plan); err != nil {
		return plan, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return plan, errors.New("trailing migration plan data")
	}
	if plan.Version != 1 {
		return plan, errors.New("unsupported migration plan version")
	}
	if plan.SourceCommit != sourceCommit {
		return plan, errors.New("unsupported source commit; use the exact original v2 deployment revision")
	}
	if len(plan.Sources) == 0 || len(plan.Sources) > 1024 {
		return plan, errors.New("migration plan requires complete source node directories")
	}
	for _, source := range plan.Sources {
		if source.NodeID == 0 || source.ShardCount < 1 || source.ShardCount > 1024 || !filepath.IsAbs(source.DataDir) {
			return plan, errors.New("invalid source node identity, shard count or absolute data directory")
		}
	}
	return plan, nil
}

func (p Plan) Digest() string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type OriginalDecoder interface {
	SourceIndexDecoder
	RecordDecoder
	BusinessDecoder
}

// Preflight is evidence of source selection and conversion only. It must never
// be presented as evidence that the target has passed independent verification.
type Preflight struct {
	Status       string              `json:"status"`
	CutoverReady bool                `json:"cutover_ready"`
	PlanDigest   string              `json:"plan_digest"`
	SourceCommit string              `json:"source_commit"`
	Capture      SourceCapture       `json:"capture"`
	Catalog      SourceCatalog       `json:"catalog"`
	Selection    SourceSelection     `json:"selection"`
	Conversion   TargetRecordsReport `json:"conversion"`
}

// Prepare reads stopped original data and publishes a preflight only when all
// authority, source-format and supported business-conversion checks succeed.
func Prepare(ctx context.Context, plan Plan, w Workspace, source Source, decoder OriginalDecoder, progress func(uint64, string)) (result Preflight, err error) {
	result.PlanDigest = plan.Digest()
	result.SourceCommit = plan.SourceCommit
	if result.Capture, err = CaptureSources(ctx, plan.Sources, source, w, progress); err != nil {
		return result, err
	}
	if result.Catalog, err = BuildSourceCatalog(ctx, result.Capture, w, decoder); err != nil {
		return result, err
	}
	if err = ValidateSourceIndexes(ctx, result.Capture, plan.Sources, w, decoder); err != nil {
		return result, err
	}
	if result.Selection, err = SelectSources(ctx, result.Capture, result.Catalog, w, decoder); err != nil {
		return result, err
	}
	if result.Conversion, err = BuildTargetRecords(ctx, result.Selection, w, decoder); err != nil {
		return result, err
	}
	result.Status = "prepared"
	data, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	return result, w.Put(ctx, []transfer.SpoolRow{{Key: []byte("workflow/PREPARED"), Value: data}})
}
