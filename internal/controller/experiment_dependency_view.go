package controller

import (
	"strings"

	"github.com/eyupio/zoomies/internal/analysis"
	"github.com/eyupio/zoomies/internal/kennel/workflow"
)

func experimentDependencyView(e *workflow.DependencyInventory, remaining *int) *analysis.RootDependencies {
	if e == nil {
		return nil
	}
	ptr := func(v int) *int { return &v }
	omitted := false
	out := &analysis.RootDependencies{Version: "root-dependencies-v1", Basis: e.Basis, SourceSHA256: e.SourceSHA256, Coverage: e.Coverage, RowsOmitted: &omitted, References: []analysis.RootDependencyReference{}}
	if c := e.Counts; c != nil {
		out.Counts = &analysis.RootDependencyCounts{Declarations: ptr(c.Declarations), CommitPins: ptr(c.CommitPins), DigestPins: ptr(c.DigestPins), Floating: ptr(c.Floating), Local: ptr(c.Local), Expressions: ptr(c.Expressions), Unknown: ptr(c.Unknown)}
		for _, r := range e.References {
			if *remaining == 0 {
				break
			}
			row := analysis.RootDependencyReference{JobIndex: ptr(r.JobIndex), StepIndex: ptr(r.StepIndex), Line: ptr(r.Line), Declaration: r.Declaration, Kind: r.Kind, Pin: r.Pin, ReferenceSHA256: r.ReferenceSHA256}
			if r.Revision != nil {
				v := strings.ToLower(*r.Revision)
				row.Revision = &v
			}
			out.References = append(out.References, row)
			*remaining--
		}
		omitted = c.Declarations > len(out.References)
	}
	return out
}
