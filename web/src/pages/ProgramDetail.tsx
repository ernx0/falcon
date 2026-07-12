import { useEffect, useState } from "react";
import { Link, useParams, useNavigate, useSearchParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient, keepPreviousData } from "@tanstack/react-query";
import { api, priorityFromSeverity } from "../lib/api";
import { Button, Card, CardHeader, Field, Input, Textarea, EmptyState, Skeleton, Icon, PriorityBadge, StatusBadge, Pagination } from "../components/ui";

const tabs = ["overview", "rules", "scopes", "hosts", "reports", "runs"] as const;
type Tab = typeof tabs[number];

const tabLabels: Record<Tab, string> = {
  overview: "Overview",
  rules: "Rules",
  scopes: "Scope",
  hosts: "Hosts",
  reports: "Reports",
  runs: "Runs",
};

export function ProgramDetail() {
  const { id } = useParams();
  const pid = Number(id);
  const [searchParams, setSearchParams] = useSearchParams();
  // Deep links from /search land here with ?tab=hosts&q=foo&host=123. Read
  // them on every render so they stay in URL when the user shares the page.
  const urlTab = searchParams.get("tab") as Tab | null;
  // Old "targets" links (from search results pre-merge) redirect to scope.
  const normalizedUrlTab = urlTab === ("targets" as any) ? "scopes" : urlTab;
  const tab: Tab = normalizedUrlTab && tabs.includes(normalizedUrlTab) ? normalizedUrlTab : "overview";
  const setTab = (t: Tab) => {
    const next = new URLSearchParams(searchParams);
    if (t === "overview") next.delete("tab"); else next.set("tab", t);
    // dropping per-tab focus params when changing tabs keeps the URL tidy
    next.delete("q");
    next.delete("host");
    setSearchParams(next, { replace: true });
  };
  const program = useQuery({ queryKey: ["program", pid], queryFn: () => api.programGet(pid), enabled: !!pid });

  if (!pid) return null;
  const p = program.data;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 flex-wrap text-sm min-w-0">
        <Link to="/programs" className="inline-flex items-center gap-1 text-muted hover:text-ink shrink-0">
          <Icon name="chevronLeft" className="w-4 h-4" /> Programs
        </Link>
        <span className="text-border-soft">/</span>
        <span className="text-xs text-muted font-mono shrink-0">P-{p ? String(p.id).padStart(3, "0") : "…"}</span>
        <span className="text-border-soft">/</span>
        {p?.icon_url ? (
          <img
            src={p.icon_url}
            alt=""
            className="w-6 h-6 rounded border border-border-soft object-cover bg-panel/40 shrink-0"
            loading="lazy"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        ) : null}
        <span className="text-lg font-semibold tracking-tight truncate">
          {p?.name || <span className="skeleton inline-block h-5 w-40 align-middle" />}
        </span>
        {p && (
          <>
            {p.platform_url ? (
              <a
                href={p.platform_url}
                target="_blank"
                rel="noreferrer"
                className="badge bg-panel/60 text-subtle border-border-soft shrink-0 hover:text-accent"
              >
                Platform ↗
              </a>
            ) : null}
            <span className="text-muted font-mono text-xs truncate">{p.slug}</span>
          </>
        )}
      </div>

      <div className="border-b border-border">
        <div className="flex overflow-x-auto scroll-thin -mb-px">
          {tabs.map((t) => {
            const active = tab === t;
            return (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
                  active ? "border-accent text-ink" : "border-transparent text-muted hover:text-ink"
                }`}
              >
                {tabLabels[t]}
              </button>
            );
          })}
        </div>
      </div>

      {tab === "overview" && <Overview p={p} loading={program.isLoading} />}
      {tab === "rules" && <Rules pid={pid} program={p} loading={program.isLoading} />}
      {tab === "scopes" && <Scopes pid={pid} />}
      {tab === "hosts" && <Hosts pid={pid} />}
      {tab === "reports" && <Reports pid={pid} />}
      {tab === "runs" && <Runs pid={pid} />}
    </div>
  );
}

function Overview({ p, loading }: { p: any; loading: boolean }) {
  const qc = useQueryClient();
  const nav = useNavigate();
  const [form, setForm] = useState<any>(null);
  const [confirmDelete, setConfirmDelete] = useState("");

  useEffect(() => {
    if (p && form === null) {
      setForm({
        name: p.name || "",
        platform_url: p.platform_url || "",
        description: p.description || "",
        icon_url: p.icon_url || "",
      });
    }
  }, [p, form]);

  const save = useMutation({
    mutationFn: () => api.programUpdate(p.id, {
      name: form.name,
      platform_url: form.platform_url,
      description: form.description,
      icon_url: form.icon_url,
    }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["program", p.id] }),
  });

  const del = useMutation({
    mutationFn: () => api.programDelete(p.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["programs"] });
      nav("/programs");
    },
  });

  if (loading || !p || !form) {
    return <Card className="p-5 space-y-2"><Skeleton /><Skeleton className="w-3/4" /></Card>;
  }

  const dirty =
    form.name !== (p.name || "") ||
    form.platform_url !== (p.platform_url || "") ||
    form.description !== (p.description || "") ||
    form.icon_url !== (p.icon_url || "");

  const reset = () => setForm({
    name: p.name || "",
    platform_url: p.platform_url || "",
    description: p.description || "",
    icon_url: p.icon_url || "",
  });

  const canDelete = confirmDelete.trim() === p.name;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="Program details"
          right={<span className={dirty ? "text-warn" : "text-muted"}>{dirty ? "Unsaved changes" : "Saved"}</span>}
        />
        <div className="p-5 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <Field label="Name">
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Acme Inc" />
            </Field>
            <Field label="Platform URL" hint="Link to the program page (e.g. HackerOne / Bugcrowd).">
              <Input value={form.platform_url} onChange={(e) => setForm({ ...form, platform_url: e.target.value })} placeholder="https://hackerone.com/acme" />
            </Field>
          </div>
          <Field label="Icon URL" hint="Optional logo / image shown next to the program name.">
            <div className="flex gap-3 items-start">
              <Input
                value={form.icon_url}
                onChange={(e) => setForm({ ...form, icon_url: e.target.value })}
                placeholder="https://example.com/logo.png"
                className="flex-1"
              />
              {form.icon_url ? (
                <img
                  src={form.icon_url}
                  alt=""
                  className="w-10 h-10 rounded-md border border-border-soft object-cover bg-panel/40 shrink-0"
                  onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
                />
              ) : null}
            </div>
          </Field>
          <Field label="Description">
            <Textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder="Short description, scope notes…"
              className="min-h-[160px]"
            />
          </Field>
          <div className="flex items-center gap-2 pt-2 border-t border-border">
            <Button variant="primary" onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
              <Icon name="save" className="w-4 h-4" />
              {save.isPending ? "Saving…" : "Save"}
            </Button>
            {dirty && <Button variant="ghost" onClick={reset}>Reset</Button>}
          </div>
        </div>
      </Card>

      <Card className="border-danger/40">
        <div className="px-4 py-2.5 border-b border-danger/40 bg-danger/5 flex items-center justify-between rounded-t-lg">
          <span className="text-sm font-semibold text-danger">Danger zone</span>
          <span className="text-xs text-muted">Irreversible actions</span>
        </div>
        <div className="p-5">
          <div className="flex items-start justify-between gap-6 flex-wrap">
            <div className="max-w-md">
              <div className="text-sm font-medium text-ink">Delete this program</div>
              <p className="text-sm text-muted mt-1 leading-relaxed">
                Permanently removes the program along with all scope items, OOS rules, runs, hosts, services, endpoints and reports. This cannot be undone.
              </p>
            </div>
            <div className="flex flex-col gap-2 w-full md:w-80">
              <Field label={`Type "${p.name}" to confirm`}>
                <Input
                  value={confirmDelete}
                  onChange={(e) => setConfirmDelete(e.target.value)}
                  placeholder={p.name}
                />
              </Field>
              <Button
                variant="danger"
                disabled={!canDelete || del.isPending}
                onClick={() => del.mutate()}
              >
                <Icon name="trash" className="w-4 h-4" />
                {del.isPending ? "Deleting…" : "Delete program"}
              </Button>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}

function Rules({ pid, program, loading }: { pid: number; program: any; loading: boolean }) {
  const qc = useQueryClient();
  const [value, setValue] = useState<string | null>(null);

  useEffect(() => {
    if (program && value === null) setValue(program.rules || "");
  }, [program, value]);

  const save = useMutation({
    mutationFn: () => api.programUpdate(pid, {
      name: program.name,
      description: program.description,
      platform_url: program.platform_url,
      rules: value ?? "",
    }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["program", pid] }),
  });

  if (loading || value === null) {
    return <Card className="p-5 space-y-2"><Skeleton /><Skeleton className="w-3/4" /></Card>;
  }

  const dirty = value !== (program.rules || "");

  return (
    <Card>
      <CardHeader
        title="Program rules"
        meta="Free-form notes — rate limits, allowed scope, testing windows, etc."
        right={
          <span className={dirty ? "text-warn" : "text-muted"}>
            {dirty ? "Unsaved changes" : "Saved"}
          </span>
        }
      />
      <div className="p-5 space-y-3">
        <Textarea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={`e.g.\n- Rate limit: 10 req/sec per host\n- Only test stores under shop.example.com\n- No automated scanning between 22:00–06:00 UTC\n- Report duplicates within 24h`}
          className="min-h-[280px]"
        />
        <div className="flex items-center gap-2">
          <Button variant="primary" onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
            <Icon name="save" className="w-4 h-4" />
            {save.isPending ? "Saving…" : "Save"}
          </Button>
          {dirty && (
            <Button variant="ghost" onClick={() => setValue(program.rules || "")}>
              Reset
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
}

// InScopePanel is the operational view of in-scope items: scannable rows
// get Run/Run-all/cron + last-run, descriptive rows (mobile apps, repos,
// labels) display kind + open-URL but no run controls. Replaces the old
// "Targets" tab — scope is the single source of truth now.
function InScopePanel({ pid }: { pid: number }) {
  const qc = useQueryClient();
  const PAGE_SIZE = 50;
  const [page, setPage] = useState(1);
  const [val, setVal] = useState("");
  const [cron, setCron] = useState("");

  const [queueing, setQueueing] = useState<Set<number>>(new Set());
  const [queued, setQueued] = useState<Set<number>>(new Set());

  const { data, isLoading } = useQuery({
    queryKey: ["scope-items", pid, page],
    queryFn: () => api.scope(pid, { page, page_size: PAGE_SIZE, scope: "all" }),
    placeholderData: keepPreviousData,
  });

  const create = useMutation({
    mutationFn: () => api.scopeCreate(pid, { value: val, schedule_cron: cron || null }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["scope-items", pid] }); setVal(""); setCron(""); setPage(1); },
  });
  const del = useMutation({
    mutationFn: (id: number) => api.scopeDelete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["scope-items", pid] }),
  });

  const items = data?.items || [];
  const total = data?.total || 0;
  const SCANNABLE_KINDS_SET = new Set(["domain", "wildcard", "ip", "cidr", "host", ""]);
  const scannableCount = items.filter((t: any) => t.enabled !== false && SCANNABLE_KINDS_SET.has(t.kind)).length;

  const markQueueing = (ids: number[]) => {
    setQueueing((prev) => {
      const next = new Set(prev);
      ids.forEach((id) => next.add(id));
      return next;
    });
  };
  const unmarkQueueing = (ids: number[]) => {
    setQueueing((prev) => {
      const next = new Set(prev);
      ids.forEach((id) => next.delete(id));
      return next;
    });
  };
  const flashQueued = (id: number) => {
    setQueued((prev) => new Set(prev).add(id));
    setTimeout(() => {
      setQueued((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }, 2200);
  };

  const runOne = async (id: number) => {
    if (queueing.has(id)) return;
    markQueueing([id]);
    try {
      await api.scopeRun(id);
      flashQueued(id);
      qc.invalidateQueries({ queryKey: ["runs", pid] });
      qc.invalidateQueries({ queryKey: ["scope-items", pid] });
    } finally {
      unmarkQueueing([id]);
    }
  };

  // Run-all kicks off every enabled target in the program (across all pages),
  // not just the currently visible page. Backend handles enqueueing in one
  // request and returns the list of (run_id, scope_id) it queued.
  const [runAllSummary, setRunAllSummary] = useState<{ queued: number; failed: number; total: number } | null>(null);
  const [runAllPending, setRunAllPending] = useState(false);
  const runAll = async () => {
    if (runAllPending) return;
    setRunAllPending(true);
    setRunAllSummary(null);
    // Optimistically mark visible rows as queueing while the bulk request runs.
    const visible = items.map((t: any) => t.id);
    markQueueing(visible);
    try {
      const r = await api.scopeRunAll(pid);
      setRunAllSummary({ queued: r.queued, failed: r.failed, total: r.total });
      // Flash visible rows whose targets were queued; off-page targets aren't
      // in the DOM but their runs are still queued.
      const queuedIds = new Set(r.run_ids.map((x) => x.scope_id));
      visible.forEach((id) => { if (queuedIds.has(id)) flashQueued(id); });
    } finally {
      unmarkQueueing(visible);
      setRunAllPending(false);
      qc.invalidateQueries({ queryKey: ["runs", pid] });
      qc.invalidateQueries({ queryKey: ["scope-items", pid] });
    }
  };

  const anyQueueing = queueing.size > 0 || runAllPending;

  return (
    <Card>
      <CardHeader
        title="In scope"
        right={
          <span className="flex items-center gap-3">
            <span>{total} total · {scannableCount} scannable</span>
            <Button
              size="sm"
              variant="primary"
              onClick={runAll}
              disabled={anyQueueing || scannableCount === 0}
              title="Run every scannable item in this program"
            >
              <Icon name="play" className="w-3.5 h-3.5" />
              {runAllPending ? "Queueing…" : `Run all (${scannableCount})`}
            </Button>
          </span>
        }
      />
      <div className="p-3 grid grid-cols-1 md:grid-cols-[1fr_180px_auto] gap-2 border-b border-border-soft">
        <Input
          placeholder="example.com  ·  *.example.com  ·  iOS app  ·  github.com/org"
          value={val}
          onChange={(e) => setVal(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && val.trim()) create.mutate(); }}
          className="font-mono text-[13px]"
        />
        <Input
          placeholder="cron (opt: 0 */6 * * *)"
          value={cron}
          onChange={(e) => setCron(e.target.value)}
          className="font-mono text-xs"
        />
        <Button variant="primary" onClick={() => create.mutate()} disabled={!val || create.isPending}>
          <Icon name="plus" className="w-4 h-4" /> Add
        </Button>
      </div>
      {(runAllPending || runAllSummary) && (
        <div className="px-4 py-1.5 text-xs text-muted bg-panel/40 border-b border-border-soft">
          {runAllPending
            ? `Queueing ${scannableCount} scannable item${scannableCount === 1 ? "" : "s"}…`
            : runAllSummary
            ? `Queued ${runAllSummary.queued}/${runAllSummary.total}${runAllSummary.failed ? ` · ${runAllSummary.failed} failed` : ""}`
            : null}
        </div>
      )}
      {isLoading && !data ? (
        <div className="p-4 space-y-2"><Skeleton /><Skeleton /><Skeleton /></div>
      ) : items.length === 0 ? (
        <EmptyState icon={<Icon name="target" className="w-7 h-7" />} title="No scope yet" hint="Add a domain, wildcard, mobile app or repo to begin." />
      ) : (
          <>
            <div className="table-wrap">
              <table className="table-base">
                <thead>
                  <tr><th className="whitespace-nowrap">Value</th><th>Kind</th><th>Schedule</th><th>Last run</th><th></th></tr>
                </thead>
                <tbody>
                  {items.map((t: any) => {
                    const isQueueing = queueing.has(t.id);
                    const isQueued = queued.has(t.id);
                    const scannable = SCANNABLE_KINDS_SET.has(t.kind) && t.enabled !== false;
                    return (
                      <tr key={t.id} className={isQueueing ? "bg-panel/40" : isQueued ? "bg-success/5" : !scannable ? "opacity-70" : ""}>
                        <td className={`whitespace-nowrap ${scannable ? "font-mono text-[13px]" : "text-sm"}`} title={t.source_url || undefined}>
                          {t.value}
                        </td>
                        <td>
                          <span className={`badge whitespace-nowrap ${
                            scannable ? "bg-bg text-ink" : "bg-accent/10 text-accent border-accent/30"
                          }`}>
                            {t.kind}
                          </span>
                        </td>
                        <td className="text-muted font-mono text-xs whitespace-nowrap">{t.schedule_cron || "—"}</td>
                        <td className="text-muted text-xs tabular whitespace-nowrap">{t.last_run_at ? new Date(t.last_run_at).toLocaleString() : "—"}</td>
                        <td className="text-right whitespace-nowrap">
                          <div className="inline-flex items-center gap-1">
                            {t.source_url && (
                              <a
                                href={t.source_url}
                                target="_blank"
                                rel="noreferrer"
                                onClick={(e) => e.stopPropagation()}
                                className="text-muted hover:text-accent px-1"
                                title={t.source_url}
                              >
                                <Icon name="external" className="w-3.5 h-3.5" />
                              </a>
                            )}
                            <button
                              onClick={() => runOne(t.id)}
                              disabled={isQueueing || !scannable}
                              title={scannable ? "Run now" : `Not scannable (${t.kind})`}
                              className={`btn-sm inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md border transition-colors ${
                                !scannable
                                  ? "border-border-soft text-muted cursor-not-allowed opacity-50"
                                  : isQueued
                                  ? "border-success text-success bg-success/10"
                                  : isQueueing
                                  ? "border-accent text-accent bg-accent/10"
                                  : "border-border text-subtle hover:bg-panel2 hover:text-ink"
                              }`}
                            >
                              {isQueued ? (
                                <>
                                  <Icon name="activity" className="w-3.5 h-3.5" /> Queued
                                </>
                              ) : isQueueing ? (
                                <>
                                  <span className="w-3.5 h-3.5 border-2 border-accent border-t-transparent rounded-full animate-spin inline-block" />
                                  Queueing…
                                </>
                              ) : (
                                <>
                                  <Icon name="play" className="w-3.5 h-3.5" /> Run
                                </>
                              )}
                            </button>
                            <Button size="sm" variant="ghost" onClick={() => del.mutate(t.id)} title="Delete" className="text-danger hover:text-danger">
                              <Icon name="trash" className="w-3.5 h-3.5" />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} label="items" />
          </>
        )}
    </Card>
  );
}

function Scopes({ pid }: { pid: number }) {
  return (
    <div className="space-y-4">
      <InScopePanel pid={pid} />
      <ScopeList
        pid={pid}
        title="Out of scope"
        emptyHint="Anything out of scope — host pattern or descriptive label."
        queryKey={["scope-oos", pid]}
        load={() => api.oos(pid, { page: 1, page_size: 500 })}
        create={(pattern) => api.oosCreate(pid, { pattern })}
        remove={(id) => api.oosDelete(id)}
        getValue={(s) => s.pattern}
        getKind={(s) => s.kind}
        getURL={(s) => s.source_url}
        placeholder="admin.example.com · *.staging.example.com"
        accent="danger"
      />
    </div>
  );
}


// SCANNABLE_KINDS distinguishes host-level rows we render in mono ("real"
// recon targets) from non-host descriptive items (mobile apps, repos, labels).
const SCANNABLE_KINDS = new Set(["domain", "wildcard", "ip", "cidr", "host", "exact", "regex", ""]);

function ScopeList({
  pid: _pid, title, emptyHint, queryKey, load, create, remove,
  getValue, getKind, getURL, placeholder, accent,
}: {
  pid: number;
  title: string;
  emptyHint: string;
  queryKey: any[];
  load: () => Promise<{ items: any[]; total: number }>;
  create: (value: string) => Promise<any>;
  remove: (id: number) => Promise<void>;
  getValue: (item: any) => string;
  getKind?: (item: any) => string | undefined;
  getURL?: (item: any) => string | undefined;
  placeholder: string;
  accent?: "danger";
}) {
  const qc = useQueryClient();
  const [val, setVal] = useState("");

  const { data, isLoading } = useQuery({
    queryKey,
    queryFn: load,
    placeholderData: keepPreviousData,
  });

  const add = useMutation({
    mutationFn: () => create(val.trim()),
    onSuccess: () => { qc.invalidateQueries({ queryKey }); setVal(""); },
  });
  const del = useMutation({
    mutationFn: (id: number) => remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey }),
  });

  const items = data?.items || [];
  const total = data?.total || 0;

  const submit = () => {
    if (!val.trim() || add.isPending) return;
    add.mutate();
  };

  return (
    <Card>
      <CardHeader title={title} right={total ? `${total}` : undefined} />
      <div className="p-3 flex gap-2 border-b border-border-soft">
        <Input
          placeholder={placeholder}
          value={val}
          onChange={(e) => setVal(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") submit(); }}
          className="flex-1 font-mono text-[13px]"
        />
        <Button
          variant={accent === "danger" ? "danger" : "primary"}
          onClick={submit}
          disabled={!val.trim() || add.isPending}
        >
          <Icon name="plus" className="w-4 h-4" /> Add
        </Button>
      </div>
      {isLoading && !data ? (
        <div className="p-4 space-y-2"><Skeleton /><Skeleton /></div>
      ) : items.length === 0 ? (
        <div className="px-4 py-8 text-center text-sm text-muted">{emptyHint}</div>
      ) : (
        <div className="divide-y divide-border-soft max-h-[520px] overflow-auto scroll-thin">
          {items.map((s: any) => {
            const value = getValue(s);
            const kind  = getKind ? (getKind(s) || "") : "";
            const url   = getURL  ? (getURL(s)  || "") : "";
            const scannable = SCANNABLE_KINDS.has(kind);
            return (
              <div key={s.id} className="px-4 py-2 flex items-center gap-2 hover:bg-panel2/60 group">
                {kind && (
                  <span className={`badge shrink-0 ${
                    scannable
                      ? "bg-panel/60 text-subtle border-border-soft"
                      : "bg-accent/10 text-accent border-accent/30"
                  }`}>
                    {kind}
                  </span>
                )}
                <span className={`flex-1 truncate ${scannable ? "font-mono text-[13px]" : "text-sm"}`} title={value}>
                  {value}
                </span>
                {url && (
                  <a
                    href={url}
                    target="_blank"
                    rel="noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="text-muted hover:text-accent shrink-0"
                    title={url}
                  >
                    <Icon name="external" className="w-3.5 h-3.5" />
                  </a>
                )}
                <button
                  onClick={() => del.mutate(s.id)}
                  className="opacity-0 group-hover:opacity-100 text-muted hover:text-danger transition-opacity shrink-0"
                  title="Remove"
                  aria-label="Remove"
                >
                  <Icon name="trash" className="w-3.5 h-3.5" />
                </button>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}

function Hosts({ pid }: { pid: number }) {
  const PAGE_SIZE = 50;
  const [params] = useSearchParams();
  const initialQ = params.get("q") || "";
  const initialHost = params.get("host") ? Number(params.get("host")) : null;

  const [page, setPage] = useState(1);
  const [q, setQ] = useState(initialQ);
  const [debouncedQ, setDebouncedQ] = useState(initialQ);
  const [oos, setOOS] = useState(false);
  const [expanded, setExpanded] = useState<number | null>(initialHost);

  // Discrete attribute filters. Mirrors the recon dimensions surfaced by
  // the backend: presence flags, count thresholds, port/status/tech, CDN.
  type Filter = {
    has_ips?: boolean;
    has_services?: boolean;
    has_endpoints?: boolean;
    cdn?: "any" | "yes" | "no";
    min_ips?: number;
    min_services?: number;
    min_endpoints?: number;
    tech?: string;
    status?: string;
    port?: string;
  };
  const [filter, setFilter] = useState<Filter>({});
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [techDebounced, setTechDebounced] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setTechDebounced(filter.tech || ""), 300);
    return () => clearTimeout(t);
  }, [filter.tech]);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQ(q), 300);
    return () => clearTimeout(t);
  }, [q]);

  useEffect(() => { setPage(1); }, [debouncedQ, oos, filter, techDebounced]);

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ["hosts", pid, debouncedQ, oos, filter, techDebounced, page],
    queryFn: () => api.hosts(pid, {
      q: debouncedQ || undefined,
      oos,
      has_ips:       filter.has_ips,
      has_services:  filter.has_services,
      has_endpoints: filter.has_endpoints,
      cdn:           filter.cdn === "yes" ? true : filter.cdn === "no" ? false : undefined,
      min_ips:       filter.min_ips || undefined,
      min_services:  filter.min_services || undefined,
      min_endpoints: filter.min_endpoints || undefined,
      tech:          techDebounced || undefined,
      status:        filter.status || undefined,
      port:          filter.port ? Number(filter.port) : undefined,
      page,
      page_size: PAGE_SIZE,
    }),
    placeholderData: keepPreviousData,
  });

  const items = data?.items || [];
  const total = data?.total || 0;

  const activeFilterCount =
    (filter.has_ips ? 1 : 0) +
    (filter.has_services ? 1 : 0) +
    (filter.has_endpoints ? 1 : 0) +
    (filter.cdn && filter.cdn !== "any" ? 1 : 0) +
    (filter.min_ips ? 1 : 0) +
    (filter.min_services ? 1 : 0) +
    (filter.min_endpoints ? 1 : 0) +
    (filter.tech ? 1 : 0) +
    (filter.status ? 1 : 0) +
    (filter.port ? 1 : 0);

  const toggleChip = (key: keyof Filter) => () =>
    setFilter((f) => ({ ...f, [key]: !f[key] || undefined }));

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="Host search"
          right={activeFilterCount > 0 ? `${activeFilterCount} filter${activeFilterCount === 1 ? "" : "s"}` : undefined}
        />
        <div className="p-4 space-y-3">
          <div className="flex flex-col md:flex-row gap-2">
            <Input placeholder="Search hosts…" value={q} onChange={(e) => setQ(e.target.value)} className="flex-1" />
            <label className="flex items-center gap-2 px-3 py-2 bg-bg border border-border rounded-md cursor-pointer hover:bg-panel2 text-sm whitespace-nowrap">
              <input type="checkbox" checked={oos} onChange={(e) => setOOS(e.target.checked)} className="accent-accent w-3.5 h-3.5" />
              <span>Show OOS</span>
            </label>
            <button
              onClick={() => setShowAdvanced((v) => !v)}
              className="px-3 py-2 text-sm border border-border rounded-md hover:bg-panel2 whitespace-nowrap"
            >
              {showAdvanced ? "Hide filters" : "Filters"}
            </button>
          </div>

          {/* Quick chips for the most common pivots. Always visible. */}
          <div className="flex flex-wrap gap-1.5">
            <FilterChip active={!!filter.has_services} onClick={toggleChip("has_services")} label="Has services" />
            <FilterChip active={!!filter.has_endpoints} onClick={toggleChip("has_endpoints")} label="Has endpoints" />
            <FilterChip active={!!filter.has_ips} onClick={toggleChip("has_ips")} label="Has IPs" />
            <FilterChip active={filter.cdn === "yes"} onClick={() => setFilter((f) => ({ ...f, cdn: f.cdn === "yes" ? undefined : "yes" }))} label="CDN" />
            <FilterChip active={filter.cdn === "no"}  onClick={() => setFilter((f) => ({ ...f, cdn: f.cdn === "no"  ? undefined : "no"  }))} label="No CDN" />
            {activeFilterCount > 0 && (
              <button
                onClick={() => setFilter({})}
                className="ml-auto text-xs text-muted hover:text-danger px-2"
              >
                Clear
              </button>
            )}
          </div>

          {showAdvanced && (
            <div className="grid grid-cols-2 md:grid-cols-3 gap-2 pt-3 border-t border-border-soft">
              <Field label="Tech contains">
                <Input placeholder="wordpress, nginx…" value={filter.tech || ""} onChange={(e) => setFilter({ ...filter, tech: e.target.value })} />
              </Field>
              <Field label="HTTP status">
                <Input placeholder="200, 401, 403…" value={filter.status || ""} onChange={(e) => setFilter({ ...filter, status: e.target.value })} />
              </Field>
              <Field label="Open port">
                <Input type="number" placeholder="80, 8080…" value={filter.port || ""} onChange={(e) => setFilter({ ...filter, port: e.target.value })} />
              </Field>
              <Field label="Min services">
                <Input type="number" placeholder="0" value={filter.min_services || ""} onChange={(e) => setFilter({ ...filter, min_services: Number(e.target.value) || undefined })} />
              </Field>
              <Field label="Min endpoints">
                <Input type="number" placeholder="0" value={filter.min_endpoints || ""} onChange={(e) => setFilter({ ...filter, min_endpoints: Number(e.target.value) || undefined })} />
              </Field>
              <Field label="Min IPs">
                <Input type="number" placeholder="0" value={filter.min_ips || ""} onChange={(e) => setFilter({ ...filter, min_ips: Number(e.target.value) || undefined })} />
              </Field>
            </div>
          )}
        </div>
      </Card>
      <Card>
        <CardHeader title="Hosts" right={isFetching ? "Loading…" : `${total} total`} />
        {isLoading && !data ? (
          <div className="p-4 space-y-2"><Skeleton /><Skeleton /><Skeleton /></div>
        ) : items.length === 0 ? (
          <EmptyState icon={<Icon name="activity" className="w-7 h-7" />} title="No hosts" hint="Trigger a target to populate the asset graph." />
        ) : (
          <>
            <div className="divide-y divide-border-soft">
              {items.map((h: any) => (
                <HostRow
                  key={h.id}
                  pid={pid}
                  host={h}
                  open={expanded === h.id}
                  onToggle={() => setExpanded(expanded === h.id ? null : h.id)}
                />
              ))}
            </div>
            <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} label="hosts" />
          </>
        )}
      </Card>
    </div>
  );
}

function FilterChip({
  active, onClick, label,
}: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`px-2.5 py-1 rounded-md text-xs font-medium border transition-colors whitespace-nowrap ${
        active
          ? "bg-ink text-bg border-ink"
          : "bg-bg text-subtle border-border hover:bg-panel2"
      }`}
    >
      {label}
    </button>
  );
}

function ExternalLink({ href, title }: { href: string; title?: string }) {
  if (!href) return null;
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      title={title || href}
      onClick={(e) => e.stopPropagation()}
      className="shrink-0 text-muted hover:text-accent transition-colors"
    >
      <Icon name="external" className="w-3.5 h-3.5" />
    </a>
  );
}

function serviceURL(s: any, hostValue: string): string {
  if (s?.meta?.http_url) return s.meta.http_url;
  if (s?.meta?.url) return s.meta.url;
  const scheme = s?.scheme || (s?.port === 443 ? "https" : "http");
  if (!hostValue) return "";
  const portPart =
    (scheme === "https" && s?.port === 443) || (scheme === "http" && s?.port === 80)
      ? ""
      : `:${s?.port}`;
  return `${scheme}://${hostValue}${portPart}`;
}

function HostRow({
  pid, host, open, onToggle,
}: { pid: number; host: any; open: boolean; onToggle: () => void }) {
  const qc = useQueryClient();
  const detail = useQuery({
    queryKey: ["host", pid, host.id],
    queryFn: () => api.hostDetail(pid, host.id),
    enabled: open,
  });
  const ips: string[] = host.ips || [];

  const toggleOOS = useMutation({
    mutationFn: () => api.hostUpdate(pid, host.id, { out_of_scope: !host.out_of_scope }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["hosts", pid] });
      qc.invalidateQueries({ queryKey: ["host", pid, host.id] });
      qc.invalidateQueries({ queryKey: ["scope-oos", pid] });
    },
  });

  const tech: string[] = Array.isArray(host.tech) ? host.tech : [];
  const topStatus = host.top_status as number | undefined;
  const statusClass =
    topStatus === undefined ? "" :
    topStatus >= 200 && topStatus < 300 ? "bg-success/10 text-success border-success/30" :
    topStatus >= 300 && topStatus < 400 ? "bg-warn/10 text-warn border-warn/30" :
    topStatus >= 400 && topStatus < 500 ? "bg-accent/10 text-accent border-accent/30" :
    topStatus >= 500                    ? "bg-danger/10 text-danger border-danger/30" :
    "bg-bg text-ink";

  return (
    <div className={host.out_of_scope ? "bg-panel/30" : ""}>
      <div className="w-full flex items-start gap-3 px-4 py-2.5 hover:bg-panel2/60">
        <button onClick={onToggle} className="flex flex-col gap-0.5 flex-1 min-w-0 text-left pt-0.5">
          <span className="flex items-center gap-2 min-w-0">
            <Icon name={open ? "chevronDown" : "chevronRight"} className="w-3.5 h-3.5 text-muted shrink-0" />
            <span className={`font-mono text-[13px] truncate ${host.out_of_scope ? "text-muted line-through" : ""}`} title={host.value}>
              {host.value}
            </span>
          </span>
          {host.title && (
            <span className="text-xs text-muted truncate pl-5" title={host.title}>
              {host.title}
            </span>
          )}
        </button>
        {host.out_of_scope && (
          <span className="badge bg-danger/10 text-danger border-danger/30 whitespace-nowrap">OOS</span>
        )}
        {topStatus !== undefined && (
          <span className={`badge tabular whitespace-nowrap ${statusClass}`} title={`HTTP ${topStatus}`}>
            {topStatus}
          </span>
        )}
        {host.has_cdn && (
          <span className="badge bg-panel/60 text-subtle border-border-soft whitespace-nowrap">CDN</span>
        )}
        {tech.slice(0, 2).map((t: string) => (
          <span key={t} className="badge bg-panel/60 text-subtle border-border-soft whitespace-nowrap hidden xl:inline-flex" title={t}>
            {t}
          </span>
        ))}
        <ExternalLink href={`https://${host.value}`} title={`Open https://${host.value}`} />
        <span className="badge bg-bg text-ink whitespace-nowrap" title="IPs">{host.ip_count} IP</span>
        <span className="badge bg-bg text-ink whitespace-nowrap" title="Services">{host.service_count} svc</span>
        <span className="badge bg-bg text-ink whitespace-nowrap" title="Endpoints">{host.endpoint_count} ep</span>
        <span className="text-xs text-muted tabular whitespace-nowrap hidden md:inline">{new Date(host.last_seen_at).toLocaleString()}</span>
        <button
          onClick={(e) => { e.stopPropagation(); toggleOOS.mutate(); }}
          disabled={toggleOOS.isPending}
          title={host.out_of_scope ? "Mark as in scope" : "Mark as out of scope"}
          className={`text-xs px-2 py-1 rounded-md border transition-colors disabled:opacity-50 ${
            host.out_of_scope
              ? "border-border text-subtle hover:bg-panel2"
              : "border-border-soft text-muted hover:text-danger hover:border-danger/40"
          }`}
        >
          {host.out_of_scope ? "Restore" : "OOS"}
        </button>
      </div>
      {open && (
        <div className="px-4 pb-4 space-y-3 bg-panel/40">
          {ips.length > 0 && (
            <div className="flex flex-wrap gap-1 pt-3">
              {ips.map((ip) => (
                <span key={ip} className="badge bg-bg text-ink font-mono normal-case">{ip}</span>
              ))}
            </div>
          )}
          {detail.isLoading ? (
            <div className="space-y-2"><Skeleton /><Skeleton /></div>
          ) : detail.data ? (() => {
            const services = detail.data.services || [];
            const allEndpoints = detail.data.endpoints || [];
            const isJS = (e: any) =>
              e?.meta?.asset_type === "js" ||
              /\.(m|c)?js(\?|$)/i.test(e?.url || "");
            const jsFiles = allEndpoints.filter(isJS);
            const endpoints = allEndpoints.filter((e: any) => !isJS(e));
            return (
              <>
                {services.length > 0 && (
                  <div>
                    <div className="text-xs font-medium text-muted mb-1.5">Services</div>
                    <table className="table-base">
                      <thead><tr><th>Port</th><th>Scheme</th><th>Status</th><th>Title</th><th>Tech</th><th></th></tr></thead>
                      <tbody>
                        {services.map((s: any) => {
                          const url = serviceURL(s, host.value);
                          return (
                            <tr key={s.id}>
                              <td className="font-mono text-[13px]">{s.port}/{s.proto}</td>
                              <td><span className="badge bg-bg text-ink">{s.scheme || "-"}</span></td>
                              <td className="text-xs tabular">{s.status_code?.Valid ? s.status_code.Int64 : "-"}</td>
                              <td className="text-xs truncate max-w-xs">{s.title}</td>
                              <td className="text-xs text-muted truncate max-w-xs font-mono">{(s.tech || []).join(", ")}</td>
                              <td>{url && <ExternalLink href={url} title={url} />}</td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
                {endpoints.length > 0 && (
                  <div>
                    <div className="text-xs font-medium text-muted mb-1.5">Endpoints ({endpoints.length})</div>
                    <div className="max-h-72 overflow-auto scroll-thin border border-border-soft rounded-md">
                      {endpoints.map((e: any) => (
                        <div key={e.id} className="px-3 py-1.5 font-mono text-xs flex items-center gap-2 border-b border-border-soft last:border-b-0">
                          <span className="text-muted shrink-0">{e.source || "?"}</span>
                          <span className={`flex-1 truncate ${e.out_of_scope ? "line-through text-muted" : ""}`}>{e.url}</span>
                          <ExternalLink href={e.url} title={e.url} />
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                {jsFiles.length > 0 && (
                  <div>
                    <div className="text-xs font-medium text-muted mb-1.5">JS files ({jsFiles.length})</div>
                    <div className="max-h-72 overflow-auto scroll-thin border border-border-soft rounded-md">
                      {jsFiles.map((e: any) => (
                        <div key={e.id} className="px-3 py-1.5 font-mono text-xs flex items-center gap-2 border-b border-border-soft last:border-b-0">
                          <span className="text-muted shrink-0">{e.source || "?"}</span>
                          <span className={`flex-1 truncate ${e.out_of_scope ? "line-through text-muted" : ""}`}>{e.url}</span>
                          <ExternalLink href={e.url} title={e.url} />
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </>
            );
          })() : null}
        </div>
      )}
    </div>
  );
}

function Reports({ pid }: { pid: number }) {
  const qc = useQueryClient();
  const nav = useNavigate();
  const PAGE_SIZE = 25;
  const [page, setPage] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ["reports", pid, page],
    queryFn: () => api.reports(pid, { page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  });

  const create = useMutation({
    mutationFn: () => api.reportCreate(pid, { title: "Untitled report", severity: "medium", status: "draft" }),
    onSuccess: (f) => {
      qc.invalidateQueries({ queryKey: ["reports", pid] });
      nav(`/reports/${f.id}`);
    },
  });

  const items = data?.items || [];
  const total = data?.total || 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end">
        <Button variant="primary" onClick={() => create.mutate()} disabled={create.isPending}>
          <Icon name="plus" className="w-4 h-4" />
          {create.isPending ? "Creating…" : "New report"}
        </Button>
      </div>
      {isLoading && !data ? (
        <div className="space-y-1"><Skeleton /><Skeleton /><Skeleton /></div>
      ) : items.length === 0 ? (
        <Card><EmptyState icon={<Icon name="bug" className="w-7 h-7" />} title="No reports" hint="Vulnerabilities and observations will appear here." /></Card>
      ) : (
        <Card>
          <CardHeader title="Reports" right={`${total} total`} />
          <div className="divide-y divide-border-soft">
            {items.map((f: any) => (
              <Link to={`/reports/${f.id}`} key={f.id} className="flex items-center gap-3 px-4 py-2.5 hover:bg-panel2/60 transition-colors">
                <span className="text-xs text-muted tabular font-mono">R-{String(f.id).padStart(3, "0")}</span>
                <PriorityBadge p={priorityFromSeverity(f.severity)} />
                <span className="text-sm flex-1 truncate">{f.title}</span>
                <StatusBadge s={f.status} />
              </Link>
            ))}
          </div>
          <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} label="reports" />
        </Card>
      )}
    </div>
  );
}

// PIPELINE_STAGES describes the worker pipeline as it's defined in
// worker/internal/pipeline/pipeline.go. Keep this in sync when the worker
// flow changes — it's the user-facing description of "what every run does".
const PIPELINE_STAGES: { tool: string; label: string; hint: string; cond?: string }[] = [
  { tool: "subfinder + assetfinder + gau", label: "Discovery", hint: "Subdomain enum + historical URLs" },
  { tool: "go stdlib resolver",            label: "DNS",       hint: "Resolve A / AAAA / CNAME records" },
  { tool: "httpx",                         label: "Probe",     hint: "Detect live HTTP services & IPs" },
  { tool: "naabu",                         label: "Ports",     hint: "Scan top ports", cond: "if IPs found" },
  { tool: "katana",                        label: "Crawl",     hint: "Crawl URLs & params", cond: "if HTTP hosts" },
  { tool: "ffuf",                          label: "Content",   hint: "Path / file fuzzing", cond: "if HTTP hosts" },
  { tool: "playwright",                    label: "Browser",   hint: "JS / SPA deep recon", cond: "if HTTP hosts" },
];

function PipelineOverview() {
  return (
    <Card>
      <CardHeader title="Pipeline" meta="What every run does, in order" />
      <div className="p-4">
        <ol className="flex flex-wrap gap-2 items-stretch">
          {PIPELINE_STAGES.map((s, i) => (
            <li key={s.tool} className="flex items-stretch gap-2">
              <div className="flex flex-col border border-border rounded-md bg-bg px-3 py-2 min-w-[160px]">
                <div className="flex items-center gap-2">
                  <span className="text-xs text-muted tabular font-mono">{String(i + 1).padStart(2, "0")}</span>
                  <span className="text-sm font-semibold text-ink">{s.label}</span>
                </div>
                <span className="text-xs text-muted font-mono mt-0.5 truncate">{s.tool}</span>
                <span className="text-xs text-subtle mt-1.5">{s.hint}</span>
                {s.cond && (
                  <span className="text-[11px] text-warn mt-1">↳ {s.cond}</span>
                )}
              </div>
              {i < PIPELINE_STAGES.length - 1 && (
                <span className="self-center text-muted shrink-0" aria-hidden>→</span>
              )}
            </li>
          ))}
        </ol>
      </div>
    </Card>
  );
}

function Runs({ pid }: { pid: number }) {
  const qc = useQueryClient();
  const PAGE_SIZE = 25;
  const [page, setPage] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ["runs", pid, page],
    queryFn: () => api.runs(pid, { page, page_size: PAGE_SIZE }),
    refetchInterval: 5000,
    placeholderData: keepPreviousData,
  });

  const delOne = useMutation({
    mutationFn: (id: number) => api.runDelete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["runs", pid] }),
  });
  const delAll = useMutation({
    mutationFn: () => api.runsDeleteAll(pid),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["runs", pid] }),
  });

  const items = data?.items || [];
  const total = data?.total || 0;

  return (
    <div className="space-y-4">
      <PipelineOverview />
      <Card>
        <CardHeader
          title="Runs"
          right={
            <span className="flex items-center gap-3">
              <span>Auto-refresh 5s · {total} total</span>
              {total > 0 && (
                <button
                  onClick={() => {
                    if (confirm(`Delete all ${total} run${total === 1 ? "" : "s"} for this program? Hosts and reports are preserved.`)) {
                      delAll.mutate();
                    }
                  }}
                  disabled={delAll.isPending}
                  className="text-xs text-danger hover:underline disabled:opacity-50"
                >
                  {delAll.isPending ? "Deleting…" : "Delete all"}
                </button>
              )}
            </span>
          }
        />
      {isLoading && !data ? (
        <div className="p-4 space-y-2"><Skeleton /><Skeleton /><Skeleton /></div>
      ) : items.length === 0 ? (
        <EmptyState icon={<Icon name="play" className="w-7 h-7" />} title="No runs yet" hint="Trigger a target to see scan runs and steps." />
      ) : (
        <>
          <div className="table-wrap">
            <table className="table-base">
              <thead>
                <tr><th>Run</th><th>Scope</th><th>Trigger</th><th>Status</th><th className="whitespace-nowrap">Started</th><th className="whitespace-nowrap">Finished</th><th></th></tr>
              </thead>
              <tbody>
                {items.map((r: any) => {
                  const deleting = delOne.isPending && delOne.variables === r.id;
                  return (
                    <tr key={r.id} className={deleting ? "opacity-50" : ""}>
                      <td className="whitespace-nowrap"><Link to={`/runs/${r.id}`} className="text-accent hover:underline font-medium tabular">#{String(r.id).padStart(4, "0")}</Link></td>
                      <td className="font-mono text-[13px] truncate max-w-xs">{r.scope_value || `#${r.scope_id}`}</td>
                      <td><span className="badge bg-bg text-ink whitespace-nowrap">{r.trigger}</span></td>
                      <td className="whitespace-nowrap"><StatusBadge s={r.status} /></td>
                      <td className="text-xs text-muted tabular whitespace-nowrap">{r.started_at ? new Date(r.started_at).toLocaleString() : "—"}</td>
                      <td className="text-xs text-muted tabular whitespace-nowrap">{r.finished_at ? new Date(r.finished_at).toLocaleString() : "—"}</td>
                      <td className="text-right whitespace-nowrap">
                        <button
                          onClick={() => { if (confirm(`Delete run #${r.id}? Hosts and reports stay.`)) delOne.mutate(r.id); }}
                          disabled={deleting}
                          title="Delete run (keeps assets)"
                          className="text-muted hover:text-danger disabled:opacity-50"
                        >
                          <Icon name="trash" className="w-3.5 h-3.5" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} label="runs" />
        </>
      )}
      </Card>
    </div>
  );
}
