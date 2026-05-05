package runner

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
)

type Subfinder struct{}

func (Subfinder) Name() string { return "subfinder" }

func (Subfinder) Run(ctx context.Context, in Input) (*Result, error) {
	root := RootDomain(in.ScopeValue)
	if root == "" {
		return &Result{}, nil
	}
	out := filepath.Join(in.ArtifactsDir, "subfinder.txt")
	count, err := runCmd(ctx, out, "subfinder", "-silent", "-all", "-d", root)
	if err != nil && count == 0 {
		return nil, err
	}

	subs, err := readLines(out)
	if err != nil {
		return nil, err
	}
	subs = dedupe(subs)

	items := make([]Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, Item{Kind: "host", Value: s})
	}
	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats:        map[string]any{"found": len(subs)},
	}, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}
