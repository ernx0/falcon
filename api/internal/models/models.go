package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StringArray maps Postgres TEXT[] columns. Minimal implementation that handles
// the wire format pgx's stdlib emits ("{a,b,c}") without pulling in lib/pq.
type StringArray []string

func (a *StringArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	var s string
	switch v := src.(type) {
	case []byte:
		s = string(v)
	case string:
		s = v
	default:
		return fmt.Errorf("StringArray: unsupported scan type %T", src)
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		*a = []string{}
		return nil
	}
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return fmt.Errorf("StringArray: malformed array literal: %q", s)
	}
	inner := s[1 : len(s)-1]
	out := []string{}
	var buf strings.Builder
	inQuotes := false
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case c == '"' && (i == 0 || inner[i-1] != '\\'):
			inQuotes = !inQuotes
		case c == ',' && !inQuotes:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteByte(c)
		}
	}
	out = append(out, buf.String())
	*a = out
	return nil
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	if len(a) == 0 {
		return "{}", nil
	}
	parts := make([]string, len(a))
	for i, s := range a {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		parts[i] = `"` + s + `"`
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func (a StringArray) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(a))
}

type User struct {
	ID           int64     `db:"id" json:"id"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type Program struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Slug        string    `db:"slug" json:"slug"`
	Description string    `db:"description" json:"description"`
	Rules       string    `db:"rules" json:"rules"`
	IconURL     string    `db:"icon_url" json:"icon_url"`
	PlatformURL string    `db:"platform_url" json:"platform_url"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type ScopeItem struct {
	ID           int64          `db:"id" json:"id"`
	ProgramID    int64          `db:"program_id" json:"program_id"`
	Value        string         `db:"value" json:"value"`
	Kind         string         `db:"kind" json:"kind"`
	SourceURL    string         `db:"source_url" json:"source_url"`
	ScheduleCron sql.NullString `db:"schedule_cron" json:"-"`
	Enabled      bool           `db:"enabled" json:"enabled"`
	LastRunAt    sql.NullTime   `db:"last_run_at" json:"-"`
	NextRunAt    sql.NullTime   `db:"next_run_at" json:"-"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
}

type ScopeItemJSON struct {
	ScopeItem
	ScheduleCronOut *string    `json:"schedule_cron"`
	LastRunAtOut    *time.Time `json:"last_run_at"`
	NextRunAtOut    *time.Time `json:"next_run_at"`
}

func (s ScopeItem) ToJSON() ScopeItemJSON {
	out := ScopeItemJSON{ScopeItem: s}
	if s.ScheduleCron.Valid {
		v := s.ScheduleCron.String
		out.ScheduleCronOut = &v
	}
	if s.LastRunAt.Valid {
		v := s.LastRunAt.Time
		out.LastRunAtOut = &v
	}
	if s.NextRunAt.Valid {
		v := s.NextRunAt.Time
		out.NextRunAtOut = &v
	}
	return out
}

type OutOfScope struct {
	ID        int64     `db:"id" json:"id"`
	ProgramID int64     `db:"program_id" json:"program_id"`
	Pattern   string    `db:"pattern" json:"pattern"`
	Kind      string    `db:"kind" json:"kind"`
	SourceURL string    `db:"source_url" json:"source_url"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Run struct {
	ID         int64      `db:"id" json:"id"`
	ScopeID    int64      `db:"scope_id" json:"scope_id"`
	ScopeValue string     `db:"scope_value" json:"scope_value"`
	ProgramID  int64      `db:"program_id" json:"program_id"`
	Trigger     string     `db:"trigger" json:"trigger"`
	Status      string     `db:"status" json:"status"`
	StartedAt   *time.Time `db:"started_at" json:"started_at"`
	FinishedAt  *time.Time `db:"finished_at" json:"finished_at"`
	Error       string     `db:"error" json:"error"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
}

type RunStep struct {
	ID           int64           `db:"id" json:"id"`
	RunID        int64           `db:"run_id" json:"run_id"`
	Tool         string          `db:"tool" json:"tool"`
	Status       string          `db:"status" json:"status"`
	StartedAt    *time.Time      `db:"started_at" json:"started_at"`
	FinishedAt   *time.Time      `db:"finished_at" json:"finished_at"`
	ArtifactPath string          `db:"artifact_path" json:"artifact_path"`
	Stats        json.RawMessage `db:"stats" json:"stats"`
	Error        string          `db:"error" json:"error"`
}

type Host struct {
	ID          int64           `db:"id" json:"id"`
	ProgramID   int64           `db:"program_id" json:"program_id"`
	ScopeID    sql.NullInt64   `db:"scope_id" json:"scope_id"`
	Value       string          `db:"value" json:"value"`
	Meta        json.RawMessage `db:"meta" json:"meta"`
	OutOfScope  bool            `db:"out_of_scope" json:"out_of_scope"`
	FirstSeenAt time.Time       `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt  time.Time       `db:"last_seen_at" json:"last_seen_at"`
}

type IP struct {
	ID          int64           `db:"id" json:"id"`
	ProgramID   int64           `db:"program_id" json:"program_id"`
	Value       string          `db:"value" json:"value"`
	Version     int             `db:"version" json:"version"`
	Meta        json.RawMessage `db:"meta" json:"meta"`
	OutOfScope  bool            `db:"out_of_scope" json:"out_of_scope"`
	FirstSeenAt time.Time       `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt  time.Time       `db:"last_seen_at" json:"last_seen_at"`
}

type Service struct {
	ID          int64           `db:"id" json:"id"`
	ProgramID   int64           `db:"program_id" json:"program_id"`
	HostID      sql.NullInt64   `db:"host_id" json:"host_id"`
	IPID        sql.NullInt64   `db:"ip_id" json:"ip_id"`
	Port        int             `db:"port" json:"port"`
	Proto       string          `db:"proto" json:"proto"`
	Scheme      string          `db:"scheme" json:"scheme"`
	StatusCode  sql.NullInt64   `db:"status_code" json:"status_code"`
	Title       string          `db:"title" json:"title"`
	Tech        StringArray     `db:"tech" json:"tech"`
	Meta        json.RawMessage `db:"meta" json:"meta"`
	OutOfScope  bool            `db:"out_of_scope" json:"out_of_scope"`
	FirstSeenAt time.Time       `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt  time.Time       `db:"last_seen_at" json:"last_seen_at"`
}

type Endpoint struct {
	ID          int64           `db:"id" json:"id"`
	ProgramID   int64           `db:"program_id" json:"program_id"`
	HostID      sql.NullInt64   `db:"host_id" json:"host_id"`
	ServiceID   sql.NullInt64   `db:"service_id" json:"service_id"`
	URL         string          `db:"url" json:"url"`
	Method      string          `db:"method" json:"method"`
	StatusCode  sql.NullInt64   `db:"status_code" json:"status_code"`
	Source      string          `db:"source" json:"source"`
	Meta        json.RawMessage `db:"meta" json:"meta"`
	OutOfScope  bool            `db:"out_of_scope" json:"out_of_scope"`
	FirstSeenAt time.Time       `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt  time.Time       `db:"last_seen_at" json:"last_seen_at"`
}

type Report struct {
	ID             int64           `db:"id" json:"id"`
	ProgramID      int64           `db:"program_id" json:"program_id"`
	HostID         sql.NullInt64   `db:"host_id" json:"host_id"`
	ServiceID      sql.NullInt64   `db:"service_id" json:"service_id"`
	EndpointID     sql.NullInt64   `db:"endpoint_id" json:"endpoint_id"`
	Title          string          `db:"title" json:"title"`
	Target         string          `db:"target" json:"target"`
	VRT            string          `db:"vrt" json:"vrt"`
	URL            string          `db:"url" json:"url"`
	Severity       string          `db:"severity" json:"severity"`
	Status         string          `db:"status" json:"status"`
	DescriptionMD  string          `db:"description_md" json:"description_md"`
	PoC            string          `db:"poc" json:"poc"`
	CVSS           string          `db:"cvss" json:"cvss"`
	ExternalURL    string          `db:"external_url" json:"external_url"`
	RewardAmount   sql.NullFloat64 `db:"reward_amount" json:"reward_amount"`
	RewardCurrency string          `db:"reward_currency" json:"reward_currency"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
}
