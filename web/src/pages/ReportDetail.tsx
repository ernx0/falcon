import { useEffect, useMemo, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { marked } from "marked";
import DOMPurify from "dompurify";
import { api, flattenVRT, severityFromPriority, priorityFromVRT } from "../lib/api";
import { Button, Card, CardHeader, Field, Input, Select, Textarea, PriorityBadge, StatusBadge, Skeleton, Icon } from "../components/ui";

// renderMarkdown converts the report's markdown into sanitized HTML.
// `marked.parse` runs synchronously when given a string; we wrap with
// DOMPurify so admin-pasted HTML can't smuggle script tags.
function renderMarkdown(md: string): string {
  if (!md) return "";
  const html = marked.parse(md, { async: false }) as string;
  return DOMPurify.sanitize(html);
}

export function ReportDetail() {
  const { id } = useParams();
  const fid = Number(id);
  const nav = useNavigate();
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ["report", fid], queryFn: () => api.reportGet(fid), enabled: !!fid });
  const vrt = useQuery({
    queryKey: ["vrt"],
    queryFn: () => api.vrt(),
    staleTime: 24 * 60 * 60 * 1000,
  });

  const vrtOptions = useMemo(
    () => (vrt.data ? flattenVRT(vrt.data.content) : []),
    [vrt.data],
  );

  const refresh = useMutation({
    mutationFn: () => api.vrtRefresh(),
    onSuccess: (data) => qc.setQueryData(["vrt"], data),
  });

  const [form, setForm] = useState<any>(null);
  useEffect(() => { if (data) setForm({ ...data }); }, [data]);

  const save = useMutation({
    mutationFn: () => api.reportUpdate(fid, {
      title: form.title,
      target: form.target,
      vrt: form.vrt,
      url: form.url,
      // severity is intentionally omitted: backend re-derives it from VRT.
      status: form.status,
      description_md: form.description_md,
    }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["report", fid] }),
  });
  const del = useMutation({
    mutationFn: () => api.reportDelete(fid),
    onSuccess: () => nav(`/programs/${data.program_id}`),
  });

  if (!form) {
    return <div className="w-full space-y-3"><Skeleton className="h-8 w-1/2" /><Skeleton /><Skeleton className="h-32" /></div>;
  }

  const onPickVRT = (id: string) => {
    const opt = vrtOptions.find((o) => o.id === id);
    setForm({
      ...form,
      vrt: id,
      severity: opt ? severityFromPriority(opt.priority) : form.severity,
    });
  };

  const priority = priorityFromVRT(form.vrt, vrtOptions);

  return (
    <div className="w-full space-y-5">
      <div>
        <button onClick={() => nav(-1)} className="inline-flex items-center gap-1 text-sm text-muted hover:text-ink">
          <Icon name="chevronLeft" className="w-4 h-4" /> Back
        </button>
        <div className="flex items-center gap-3 mt-3 flex-wrap">
          <span className="text-xs text-muted font-mono">R-{String(form.id).padStart(3, "0")}</span>
          <PriorityBadge p={priority} />
          <StatusBadge s={form.status} />
        </div>
        <input
          className="w-full text-3xl font-bold tracking-tight bg-transparent border-0 outline-none focus:ring-0 px-0 py-3 placeholder-muted"
          value={form.title}
          onChange={(e) => setForm({ ...form, title: e.target.value })}
          placeholder="Untitled report…"
        />
      </div>

      <Card>
        <CardHeader title="Details" />
        <div className="p-5 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <Field label="Target" hint="Asset, host, IP or app this report covers.">
              <Input
                value={form.target || ""}
                onChange={(e) => setForm({ ...form, target: e.target.value })}
                placeholder="example.com / Acme Web App"
              />
            </Field>
            <Field label="URL" hint="Affected URL or endpoint.">
              <Input
                value={form.url || ""}
                onChange={(e) => setForm({ ...form, url: e.target.value })}
                placeholder="https://example.com/api/users/{id}"
              />
            </Field>
          </div>

          <Field
            label="VRT"
            hint={
              vrt.isLoading
                ? "Loading taxonomy…"
                : `Bugcrowd Vulnerability Rating Taxonomy — priority sets the severity.${
                    vrt.data?.metadata?.release_date
                      ? ` Released ${String(vrt.data.metadata.release_date).slice(0, 10)}.`
                      : ""
                  }`
            }
          >
            <div className="flex gap-2">
              <div className="flex-1 min-w-0">
                <VRTPicker
                  value={form.vrt || ""}
                  onChange={onPickVRT}
                  options={vrtOptions}
                  loading={vrt.isLoading}
                />
              </div>
              <Button
                type="button"
                variant="secondary"
                onClick={() => refresh.mutate()}
                disabled={refresh.isPending}
                title="Pull the latest VRT from Bugcrowd"
              >
                <Icon name="activity" className={refresh.isPending ? "w-4 h-4 animate-pulse" : "w-4 h-4"} />
                {refresh.isPending ? "Updating…" : "Update"}
              </Button>
            </div>
            {refresh.isError && (
              <div className="mt-2 text-xs text-danger">Update failed: {(refresh.error as any)?.message}</div>
            )}
          </Field>

          <Field label="Status">
            <Select
              value={form.status}
              onChange={(e) => setForm({ ...form, status: e.target.value })}
              className="md:w-60"
            >
              {["draft", "submitted", "triaged", "resolved", "duplicate", "n_a"].map((s) => (
                <option key={s} value={s}>{s.replace("_", " ")}</option>
              ))}
            </Select>
          </Field>
        </div>
      </Card>

      <DescriptionCard
        value={form.description_md || ""}
        onChange={(v) => setForm({ ...form, description_md: v })}
      />

      <div className="sticky bottom-4 z-10 flex gap-2 bg-bg/90 backdrop-blur p-3 border border-border rounded-lg shadow-sm">
        <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>
          <Icon name="save" className="w-4 h-4" />
          {save.isPending ? "Saving…" : "Save"}
        </Button>
        {form.url && (
          <a href={form.url} target="_blank" rel="noreferrer" className="btn-secondary">
            <Icon name="external" className="w-4 h-4" /> Open URL
          </a>
        )}
        <Button variant="danger" onClick={() => { if (confirm("Delete this report?")) del.mutate(); }} className="ml-auto">
          <Icon name="trash" className="w-4 h-4" /> Delete
        </Button>
      </div>
    </div>
  );
}

// DescriptionCard wraps the markdown textarea with an Edit/Preview tab
// switcher. Preview renders the same string the server will store, so
// formatting matches what reviewers will see.
function DescriptionCard({
  value, onChange,
}: { value: string; onChange: (v: string) => void }) {
  const [tab, setTab] = useState<"edit" | "preview">("edit");
  const html = useMemo(() => (tab === "preview" ? renderMarkdown(value) : ""), [tab, value]);

  return (
    <Card>
      <div className="card-header">
        <div className="flex items-center gap-3">
          <span className="card-header-title">Description</span>
          <span className="card-header-meta">Markdown</span>
        </div>
        <div className="inline-flex items-center text-xs rounded-md border border-border overflow-hidden">
          <button
            onClick={() => setTab("edit")}
            className={`px-3 py-1 transition-colors ${
              tab === "edit" ? "bg-ink text-bg" : "text-subtle hover:bg-panel2"
            }`}
          >
            Edit
          </button>
          <button
            onClick={() => setTab("preview")}
            className={`px-3 py-1 border-l border-border transition-colors ${
              tab === "preview" ? "bg-ink text-bg" : "text-subtle hover:bg-panel2"
            }`}
          >
            Preview
          </button>
        </div>
      </div>
      <div className="p-5">
        {tab === "edit" ? (
          <Textarea
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder="Steps to reproduce, impact, evidence, references…"
            className="min-h-[480px]"
          />
        ) : value.trim() === "" ? (
          <div className="min-h-[480px] text-sm text-muted italic">
            Nothing to preview yet.
          </div>
        ) : (
          <div
            className="prose-md min-h-[480px] text-sm leading-relaxed"
            dangerouslySetInnerHTML={{ __html: html }}
          />
        )}
      </div>
    </Card>
  );
}

function VRTPicker({
  value, onChange, options, loading,
}: {
  value: string;
  onChange: (id: string) => void;
  options: { id: string; label: string; priority: number }[];
  loading?: boolean;
}) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);

  const selected = options.find((o) => o.id === value);

  const filtered = useMemo(() => {
    if (!query) return options.slice(0, 200);
    const q = query.toLowerCase();
    return options
      .filter((o) => o.label.toLowerCase().includes(q) || o.id.toLowerCase().includes(q))
      .slice(0, 200);
  }, [options, query]);

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="input flex items-center justify-between gap-2 text-left"
        disabled={loading}
      >
        <span className={selected ? "" : "text-muted"}>
          {loading ? "Loading…" : selected ? selected.label : "Pick a VRT category…"}
        </span>
        <span className="flex items-center gap-2 shrink-0">
          {selected && selected.priority > 0 && <PriorityBadge p={selected.priority} />}
          <Icon name="chevronDown" className="w-3.5 h-3.5 text-muted" />
        </span>
      </button>

      {open && (
        <div className="absolute z-20 left-0 right-0 mt-1 card-shadow max-h-80 overflow-hidden flex flex-col">
          <div className="p-2 border-b border-border-soft bg-panel/40">
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search taxonomy…"
              className="input"
            />
          </div>
          <div className="overflow-auto scroll-thin flex-1">
            {value && (
              <button
                type="button"
                onClick={() => { onChange(""); setOpen(false); setQuery(""); }}
                className="w-full text-left px-3 py-2 text-sm text-muted hover:bg-panel2/60 border-b border-border-soft"
              >
                Clear selection
              </button>
            )}
            {filtered.length === 0 ? (
              <div className="px-3 py-6 text-sm text-muted text-center">No matches</div>
            ) : (
              filtered.map((o) => (
                <button
                  key={o.id}
                  type="button"
                  onClick={() => { onChange(o.id); setOpen(false); setQuery(""); }}
                  className={`w-full text-left px-3 py-2 text-sm flex items-center gap-2 hover:bg-panel2/60 ${
                    o.id === value ? "bg-panel2" : ""
                  }`}
                >
                  <span className="flex-1 truncate">{o.label}</span>
                  {o.priority > 0 && <PriorityBadge p={o.priority} />}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
