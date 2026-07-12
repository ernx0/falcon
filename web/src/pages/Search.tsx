import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, priorityFromSeverity } from "../lib/api";
import { Card, EmptyState, Skeleton, Icon, PriorityBadge } from "../components/ui";

type Group = "all" | "programs" | "scope" | "hosts" | "services" | "endpoints" | "reports";

const GROUPS: { key: Group; label: string; icon: string }[] = [
  { key: "all",       label: "All",       icon: "search" },
  { key: "programs",  label: "Programs",  icon: "folder" },
  { key: "scope",     label: "Scope",     icon: "target" },
  { key: "hosts",     label: "Hosts",     icon: "activity" },
  { key: "services",  label: "Services",  icon: "grid" },
  { key: "endpoints", label: "Endpoints", icon: "external" },
  { key: "reports",   label: "Reports",   icon: "bug" },
];

export function Search() {
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [group, setGroup] = useState<Group>("all");
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(q.trim()), 250);
    return () => clearTimeout(t);
  }, [q]);

  // Cmd/Ctrl + K focuses the input — matches the convention researchers expect.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const { data, isFetching } = useQuery({
    queryKey: ["search", debounced],
    queryFn: () => api.search(debounced),
    enabled: debounced.length >= 2,
  });

  const counts = useMemo(() => ({
    programs:  data?.programs?.length  || 0,
    scope:    data?.scope?.length   || 0,
    hosts:     data?.hosts?.length     || 0,
    services:  data?.services?.length  || 0,
    endpoints: data?.endpoints?.length || 0,
    reports:   data?.reports?.length   || 0,
  }), [data]);
  const totalHits = Object.values(counts).reduce((a, b) => a + b, 0);

  const showGroup = (g: Group) => group === "all" || group === g;

  return (
    <div className="space-y-5">
      <Card shadow>
        <div className="px-4 py-3 flex items-center gap-3 border-b border-border-soft">
          <Icon name="search" className="w-4 h-4 text-muted shrink-0" />
          <input
            ref={inputRef}
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search programs, scope, hosts, services, endpoints, reports…"
            className="flex-1 bg-transparent outline-none text-sm placeholder-muted"
          />
          <span className="text-xs text-muted shrink-0">
            {isFetching ? "Searching…" : debounced.length >= 2 ? `${totalHits} hits` : <span className="kbd">⌘K</span>}
          </span>
        </div>

        <div className="px-2 py-1.5 flex items-center gap-1 overflow-x-auto scroll-thin">
          {GROUPS.map((g) => {
            const c = g.key === "all" ? totalHits : counts[g.key as keyof typeof counts];
            const active = group === g.key;
            return (
              <button
                key={g.key}
                onClick={() => setGroup(g.key)}
                disabled={debounced.length < 2}
                className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-md text-xs font-medium transition-colors whitespace-nowrap disabled:opacity-50 ${
                  active ? "bg-ink text-bg" : "text-subtle hover:bg-panel2"
                }`}
              >
                <Icon name={g.icon} className="w-3 h-3" />
                {g.label}
                {debounced.length >= 2 && (
                  <span className={`tabular ${active ? "opacity-70" : "opacity-60"}`}>{c}</span>
                )}
              </button>
            );
          })}
        </div>
      </Card>

      {debounced.length < 2 ? (
        <Card>
          <EmptyState
            icon={<Icon name="search" className="w-7 h-7" />}
            title="Search across everything"
            hint="Type 2+ characters to find programs, scope, hosts, services, endpoints and reports."
          />
        </Card>
      ) : !data ? (
        <div className="space-y-2">
          <Card className="p-4 space-y-2"><Skeleton /><Skeleton /><Skeleton /></Card>
        </div>
      ) : totalHits === 0 ? (
        <Card>
          <EmptyState
            icon={<Icon name="search" className="w-7 h-7" />}
            title="No matches"
            hint={`Nothing matched "${debounced}". Try a different query.`}
          />
        </Card>
      ) : (
        <div className="space-y-4">
          {showGroup("programs") && counts.programs > 0 && (
            <ResultGroup title="Programs" count={counts.programs}>
              {(data.programs as any[]).map((p) => (
                <ResultRow
                  key={`p-${p.id}`}
                  to={`/programs/${p.id}`}
                  primary={p.name}
                  secondary={p.platform_url || ""}
                  meta={p.slug}
                  badge={<span className="badge bg-panel/60 text-subtle border-border-soft">P-{String(p.id).padStart(3, "0")}</span>}
                />
              ))}
            </ResultGroup>
          )}

          {showGroup("scope") && counts.scope > 0 && (
            <ResultGroup title="Scope" count={counts.scope}>
              {(data.scope as any[]).map((t) => (
                <ResultRow
                  key={`t-${t.id}`}
                  to={`/programs/${t.program_id}?tab=scopes`}
                  primary={t.value}
                  primaryMono
                  secondary={t.kind}
                  badge={<span className="badge bg-bg text-ink">P-{String(t.program_id).padStart(3, "0")}</span>}
                />
              ))}
            </ResultGroup>
          )}

          {showGroup("hosts") && counts.hosts > 0 && (
            <ResultGroup title="Hosts" count={counts.hosts}>
              {(data.hosts as any[]).map((h) => (
                <ResultRow
                  key={`h-${h.id}`}
                  to={`/programs/${h.program_id}?tab=hosts&q=${encodeURIComponent(h.value)}&host=${h.id}`}
                  primary={h.value}
                  primaryMono
                  badge={<span className="badge bg-bg text-ink">P-{String(h.program_id).padStart(3, "0")}</span>}
                />
              ))}
            </ResultGroup>
          )}

          {showGroup("services") && counts.services > 0 && (
            <ResultGroup title="Services" count={counts.services}>
              {(data.services as any[]).map((s) => {
                const host = s.host_value?.Valid ? s.host_value.String : "";
                const label = host ? `${host}:${s.port}` : `(ip):${s.port}`;
                return (
                  <ResultRow
                    key={`s-${s.id}`}
                    to={
                      host
                        ? `/programs/${s.program_id}?tab=hosts&q=${encodeURIComponent(host)}&host=${s.host_id?.Int64 ?? ""}`
                        : `/programs/${s.program_id}?tab=hosts`
                    }
                    primary={label}
                    primaryMono
                    secondary={s.title || s.scheme || ""}
                    badge={<span className="badge bg-bg text-ink">P-{String(s.program_id).padStart(3, "0")}</span>}
                  />
                );
              })}
            </ResultGroup>
          )}

          {showGroup("endpoints") && counts.endpoints > 0 && (
            <ResultGroup title="Endpoints" count={counts.endpoints}>
              {(data.endpoints as any[]).map((e) => {
                const host = e.host_value?.Valid ? e.host_value.String : "";
                return (
                  <ResultRow
                    key={`e-${e.id}`}
                    to={
                      host
                        ? `/programs/${e.program_id}?tab=hosts&q=${encodeURIComponent(host)}&host=${e.host_id?.Int64 ?? ""}`
                        : `/programs/${e.program_id}?tab=hosts`
                    }
                    primary={e.url}
                    primaryMono
                    badge={<span className="badge bg-bg text-ink">P-{String(e.program_id).padStart(3, "0")}</span>}
                  />
                );
              })}
            </ResultGroup>
          )}

          {showGroup("reports") && counts.reports > 0 && (
            <ResultGroup title="Reports" count={counts.reports}>
              {(data.reports as any[]).map((f) => (
                <ResultRow
                  key={`f-${f.id}`}
                  to={`/reports/${f.id}`}
                  primary={f.title}
                  secondary={f.status}
                  badge={
                    <span className="flex items-center gap-1.5">
                      <PriorityBadge p={priorityFromSeverity(f.severity)} />
                      <span className="badge bg-bg text-ink">R-{String(f.id).padStart(3, "0")}</span>
                    </span>
                  }
                />
              ))}
            </ResultGroup>
          )}
        </div>
      )}
    </div>
  );
}

function ResultGroup({
  title, count, children,
}: { title: string; count: number; children: React.ReactNode }) {
  return (
    <Card>
      <div className="card-header">
        <div className="flex items-center gap-2">
          <span className="card-header-title">{title}</span>
          <span className="text-xs text-muted tabular">{count}</span>
        </div>
      </div>
      <div className="divide-y divide-border-soft">{children}</div>
    </Card>
  );
}

function ResultRow({
  to, primary, primaryMono, secondary, meta, badge,
}: {
  to: string;
  primary: string;
  primaryMono?: boolean;
  secondary?: string;
  meta?: string;
  badge?: React.ReactNode;
}) {
  return (
    <Link
      to={to}
      className="flex items-center gap-3 px-4 py-2.5 hover:bg-panel2/60 transition-colors min-w-0"
    >
      {badge && <span className="shrink-0">{badge}</span>}
      <span className={`flex-1 truncate text-sm ${primaryMono ? "font-mono text-[13px]" : ""}`}>
        {primary}
      </span>
      {secondary && (
        <span className="hidden md:inline text-xs text-muted truncate max-w-[180px]">{secondary}</span>
      )}
      {meta && (
        <span className="hidden md:inline text-xs text-muted font-mono truncate max-w-[180px]">{meta}</span>
      )}
      <Icon name="chevronRight" className="w-3.5 h-3.5 text-muted shrink-0" />
    </Link>
  );
}
