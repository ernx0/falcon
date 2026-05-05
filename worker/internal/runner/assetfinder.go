package runner

import (
	"context"
	"path/filepath"
	"strings"
)

type Assetfinder struct{}

func (Assetfinder) Name() string { return "assetfinder" }

func (Assetfinder) Run(ctx context.Context, in Input) (*Result, error) {
	root := RootDomain(in.ScopeValue)
	if root == "" {
		return &Result{}, nil
	}
	out := filepath.Join(in.ArtifactsDir, "assetfinder.txt")
	_, _ = runCmd(ctx, out, "assetfinder", "--subs-only", root)
	subs, err := readLines(out)
	if err != nil {
		return nil, err
	}
	// keep only those ending with the root domain
	filtered := subs[:0]
	for _, s := range subs {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && (s == root || strings.HasSuffix(s, "."+root)) {
			filtered = append(filtered, s)
		}
	}
	filtered = dedupe(filtered)
	items := make([]Item, 0, len(filtered))
	for _, s := range filtered {
		items = append(items, Item{Kind: "host", Value: s})
	}
	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats:        map[string]any{"found": len(filtered)},
	}, nil
}
