import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Card, CardHeader, StatusBadge, Skeleton, Icon } from "../components/ui";

export function RunDetail() {
  const { id } = useParams();
  const rid = Number(id);
  const { data, isLoading } = useQuery({
    queryKey: ["run", rid],
    queryFn: () => api.runGet(rid),
    enabled: !!rid,
    refetchInterval: (q) => {
      const s = (q.state.data as any)?.run?.status;
      return s === "queued" || s === "running" ? 3000 : false;
    },
  });

  // Worker emits two rows per tool (one when it starts, one when it ends).
  // Collapse by tool name — keep the most recent entry.
  const collapsed = (() => {
    if (!data) return [];
    const byTool = new Map<string, any>();
    for (const s of data.steps) byTool.set(s.tool, s);
    return Array.from(byTool.values());
  })();

  const scopeValue = data?.run.scope_value;

  if (isLoading || !data) {
    return <div className="space-y-3"><Skeleton className="h-8 w-48" /><Skeleton /><Skeleton /></div>;
  }

  const finishedSteps = collapsed.filter((s: any) => s.status === "success" || s.status === "failed").length;
  const total = collapsed.length;
  const pct = total > 0 ? Math.round((finishedSteps / total) * 100) : 0;

  return (
    <div className="space-y-6">
      <div>
        <Link to={`/programs/${data.run.program_id || ""}`} className="inline-flex items-center gap-1 text-sm text-muted hover:text-ink">
          <Icon name="chevronLeft" className="w-4 h-4" /> Back
        </Link>
        <div className="flex items-center gap-3 mt-3 flex-wrap">
          <h1 className="tracking-tight tabular">Run #{String(data.run.id).padStart(4, "0")}</h1>
          <StatusBadge s={data.run.status} />
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mt-5">
          <Cell label="Scope" value={scopeValue || `#${data.run.scope_id}`} />
          <Cell label="Trigger" value={data.run.trigger} />
          <Cell label="Steps" value={`${finishedSteps}/${total}`} />
          <Cell label="Progress" value={`${pct}%`} accent />
        </div>
        <div className="mt-3 h-1.5 bg-panel2 rounded-full overflow-hidden">
          <div className="h-full bg-accent transition-all" style={{ width: `${pct}%` }} />
        </div>
      </div>

      <div className="space-y-3">
        <div className="flex items-end gap-3 border-b border-border pb-2">
          <h2 className="text-base font-semibold tracking-tight">Pipeline steps</h2>
          <span className="ml-auto section-tag tabular">{total} total</span>
        </div>
        <div className="space-y-3">
          {collapsed.map((s: any, idx: number) => {
            const elapsed =
              s.started_at && s.finished_at
                ? Math.max(0, Math.round((new Date(s.finished_at).getTime() - new Date(s.started_at).getTime()) / 1000))
                : null;
            return (
              <Card key={s.id}>
                <div className="card-header">
                  <div className="flex items-center gap-3 min-w-0">
                    <span className="text-xs text-muted tabular font-mono">{String(idx + 1).padStart(2, "0")}</span>
                    <span className="font-mono text-sm font-semibold">{s.tool}</span>
                    <StatusBadge s={s.status} />
                  </div>
                  <span className="text-xs text-muted tabular">
                    {s.started_at ? new Date(s.started_at).toLocaleTimeString() : "—"}
                    {" → "}
                    {s.finished_at ? new Date(s.finished_at).toLocaleTimeString() : "…"}
                    {elapsed !== null && ` (${elapsed}s)`}
                  </span>
                </div>
                {(s.stats && Object.keys(s.stats).length > 0) || s.error || s.artifact_path ? (
                  <div className="p-4 space-y-3">
                    {s.stats && Object.keys(s.stats).length > 0 && (
                      <pre className="text-xs whitespace-pre-wrap bg-ink text-bg rounded-md p-3 font-mono leading-relaxed scroll-thin overflow-auto max-h-56">
                        {JSON.stringify(s.stats, null, 2)}
                      </pre>
                    )}
                    {s.error && (
                      <div className="text-sm text-danger bg-danger/10 border border-danger/30 rounded-md px-3 py-2 font-mono">
                        {s.error}
                      </div>
                    )}
                    {s.artifact_path && (
                      <ArtifactRow path={s.artifact_path} />
                    )}
                  </div>
                ) : null}
              </Card>
            );
          })}
          {data.run.error && (
            <div className="border border-danger bg-danger/10 text-danger rounded-md p-3 font-mono text-sm">
              {data.run.error}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ArtifactRow shows the pipeline step's output file path as a clickable
// row that toggles an inline preview of the file's contents (capped at
// 1 MiB by the API client to keep the browser responsive).
function ArtifactRow({ path }: { path: string }) {
  const [open, setOpen] = useState(false);
  const [data, setData] = useState<{ text: string; size: number; truncated: boolean } | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const toggle = async () => {
    if (open) { setOpen(false); return; }
    setOpen(true);
    if (data) return;
    setLoading(true);
    setErr(null);
    try {
      const r = await api.artifact(path);
      setData(r);
    } catch (e: any) {
      setErr(e?.message || "Failed to load");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="border border-border-soft bg-panel/40 rounded">
      <button
        onClick={toggle}
        className="w-full flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted font-mono hover:bg-panel2/60 hover:text-ink"
      >
        <Icon name={open ? "chevronDown" : "chevronRight"} className="w-3.5 h-3.5 shrink-0" />
        <Icon name="folder" className="w-3.5 h-3.5 shrink-0" />
        <span className="truncate flex-1 text-left">{path}</span>
      </button>
      {open && (
        <div className="border-t border-border-soft">
          {loading ? (
            <div className="px-3 py-2 text-xs text-muted">Loading…</div>
          ) : err ? (
            <div className="px-3 py-2 text-xs text-danger">{err}</div>
          ) : data ? (
            <>
              <pre className="text-xs font-mono leading-relaxed bg-ink text-bg p-3 max-h-96 overflow-auto scroll-thin whitespace-pre-wrap break-all">
                {data.text || "(empty)"}
              </pre>
              <div className="px-3 py-1.5 text-xs text-muted">
                {(data.size / 1024).toFixed(1)} KB{data.truncated ? " · truncated to first 1 MiB" : ""}
              </div>
            </>
          ) : null}
        </div>
      )}
    </div>
  );
}

function Cell({ label, value, accent }: { label: string; value: any; accent?: boolean }) {
  return (
    <div className={`p-4 rounded-lg border ${accent ? "bg-accent text-bg border-accent" : "bg-bg border-border"}`}>
      <div className={`text-xs font-medium uppercase tracking-wide ${accent ? "text-bg/80" : "text-muted"}`}>{label}</div>
      <div className="text-lg font-semibold mt-1 leading-tight tabular truncate">{value}</div>
    </div>
  );
}
