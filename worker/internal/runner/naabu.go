package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Naabu struct{}

func (Naabu) Name() string { return "naabu" }

type naabuRow struct {
	IP   string `json:"ip"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (Naabu) Run(ctx context.Context, in Input) (*Result, error) {
	if len(in.IPs) == 0 {
		return &Result{}, nil
	}
	listPath := filepath.Join(in.ArtifactsDir, "naabu_input.txt")
	_ = os.MkdirAll(filepath.Dir(listPath), 0o755)
	f, err := os.Create(listPath)
	if err != nil {
		return nil, err
	}
	for _, ip := range in.IPs {
		f.WriteString(ip + "\n")
	}
	f.Close()

	out := filepath.Join(in.ArtifactsDir, "naabu.jsonl")
	_, _ = runCmd(ctx, out, "naabu",
		"-list", listPath,
		"-json", "-silent",
		"-top-ports", "1000",
	)

	lines, err := readLines(out)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var row naabuRow
		if err := json.Unmarshal([]byte(ln), &row); err != nil {
			continue
		}
		if row.IP != "" {
			items = append(items, Item{
				Kind:   "ip",
				Value:  row.IP,
				Host:   row.Host,
				Source: "naabu",
			})
		}
		items = append(items, Item{
			Kind:  "service",
			Host:  row.Host,
			IP:    row.IP,
			Port:  row.Port,
			Proto: "tcp",
		})
	}
	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats:        map[string]any{"open_ports": len(items)},
	}, nil
}
