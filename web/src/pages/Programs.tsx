import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient, keepPreviousData } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Button, Card, CardHeader, Field, Input, Textarea, EmptyState, Skeleton, Icon, Pagination } from "../components/ui";

const PAGE_SIZE = 24;

export function Programs() {
  const qc = useQueryClient();
  const nav = useNavigate();
  const [page, setPage] = useState(1);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [platform, setPlatform] = useState("");
  const [iconURL, setIconURL] = useState("");
  const [importing, setImporting] = useState(false);
  const [importHTML, setImportHTML] = useState("");
  const [importError, setImportError] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["programs", page],
    queryFn: () => api.programs({ page, page_size: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  });

  const create = useMutation({
    mutationFn: () => api.programCreate({ name, description: desc, platform, icon_url: iconURL }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["programs"] });
      setName(""); setDesc(""); setPlatform(""); setIconURL(""); setCreating(false);
      setPage(1);
    },
  });

  const importBugcrowd = useMutation({
    mutationFn: (htmlBody: string) => api.programImportBugcrowd(htmlBody),
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["programs"] });
      setImportError(null);
      setImporting(false);
      setImportHTML("");
      nav(`/programs/${p.id}`);
    },
    onError: (err: any) => setImportError(err?.message || "Import failed"),
  });

  const submitImport = () => {
    const html = importHTML.trim();
    if (!html || importBugcrowd.isPending) return;
    setImportError(null);
    importBugcrowd.mutate(html);
  };

  const programs = data?.items || [];
  const total = data?.total || 0;

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-end gap-2 flex-wrap">
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => { setImporting(true); setImportError(null); }}
            title="Paste a Bugcrowd program page HTML to import"
          >
            <Icon name="external" className="w-4 h-4" /> Import (Bugcrowd)
          </Button>
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Icon name="plus" className="w-4 h-4" /> New program
          </Button>
        </div>
      </div>

      {importing && (
        <Card shadow>
          <CardHeader
            title="Import from Bugcrowd"
            meta="Paste the program page HTML below"
            right={
              <button
                onClick={() => { setImporting(false); setImportHTML(""); setImportError(null); }}
                className="text-xs text-muted hover:text-danger"
              >
                Close
              </button>
            }
          />
          <div className="p-5 space-y-3">
            <Textarea
              autoFocus
              value={importHTML}
              onChange={(e) => setImportHTML(e.target.value)}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === "Enter") submitImport();
              }}
              placeholder="Open the Bugcrowd program page → View source (or DevTools → copy outer HTML) → paste here…"
              className="min-h-[260px] font-mono text-xs"
            />
            <div className="flex items-center gap-2">
              <Button
                variant="primary"
                onClick={submitImport}
                disabled={!importHTML.trim() || importBugcrowd.isPending}
              >
                {importBugcrowd.isPending ? (
                  <>
                    <span className="w-3.5 h-3.5 border-2 border-bg border-t-transparent rounded-full animate-spin inline-block" />
                    Importing…
                  </>
                ) : (
                  <>
                    <Icon name="save" className="w-4 h-4" /> Import
                  </>
                )}
              </Button>
              <Button
                variant="ghost"
                onClick={() => { setImporting(false); setImportHTML(""); setImportError(null); }}
                disabled={importBugcrowd.isPending}
              >
                Cancel
              </Button>
              <span className="ml-auto text-xs text-muted">
                {importHTML ? `${importHTML.length.toLocaleString()} chars` : "Cmd/Ctrl + Enter to submit"}
              </span>
            </div>
            {importError && (
              <div className="text-sm text-danger bg-danger/10 border border-danger/30 rounded-md px-3 py-2">
                Import failed: {importError}
              </div>
            )}
          </div>
        </Card>
      )}

      {creating && (
        <Card shadow>
          <CardHeader title="New program" right={
            <button onClick={() => setCreating(false)} className="text-xs text-muted hover:text-danger">Close</button>
          } />
          <div className="p-5 space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field label="Name">
                <Input placeholder="Acme Inc" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
              </Field>
              <Field label="Platform" hint="e.g. h1, bugcrowd, intigriti, private">
                <Input placeholder="private" value={platform} onChange={(e) => setPlatform(e.target.value)} />
              </Field>
            </div>
            <Field label="Icon URL" hint="Optional logo / image URL.">
              <div className="flex gap-3 items-start">
                <Input
                  placeholder="https://example.com/logo.png"
                  value={iconURL}
                  onChange={(e) => setIconURL(e.target.value)}
                  className="flex-1"
                />
                {iconURL && (
                  <img
                    src={iconURL}
                    alt=""
                    className="w-10 h-10 rounded-md border border-border-soft object-cover bg-panel/40 shrink-0"
                    onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
                  />
                )}
              </div>
            </Field>
            <Field label="Description">
              <Textarea placeholder="Short description, scope notes…" value={desc} onChange={(e) => setDesc(e.target.value)} />
            </Field>
            <div className="flex gap-2 pt-3 border-t border-border">
              <Button variant="primary" onClick={() => create.mutate()} disabled={!name || create.isPending}>
                {create.isPending ? "Creating…" : "Create program"}
              </Button>
              <Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>
            </div>
          </div>
        </Card>
      )}

      {/* Programs grid + sticky pagination at the bottom of the viewport. */}
      <div className="pb-16">
        {isLoading && !data ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2.5">
            {[1,2,3,4,5,6,7,8].map((i) => (
              <div key={i} className="card px-3 py-2.5 flex items-center gap-3">
                <div className="skeleton w-8 h-8 rounded shrink-0" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3 w-3/4" />
                  <Skeleton className="h-2.5 w-1/2" />
                </div>
              </div>
            ))}
          </div>
        ) : programs.length === 0 ? (
          <Card>
            <EmptyState
              icon={<Icon name="folder" className="w-7 h-7" />}
              title="No programs yet"
              hint="Initialize your first program to begin scope and target management."
              action={<Button variant="primary" onClick={() => setCreating(true)}><Icon name="plus" className="w-4 h-4" /> New program</Button>}
            />
          </Card>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2.5">
            {programs.map((p: any) => (
              <Link
                key={p.id}
                to={`/programs/${p.id}`}
                className="group card px-3 py-2.5 flex items-center gap-3 bg-bg hover:border-ink transition-colors"
              >
                {p.icon_url ? (
                  <img
                    src={p.icon_url}
                    alt=""
                    className="w-8 h-8 rounded border border-border-soft object-cover bg-panel/40 shrink-0"
                    loading="lazy"
                    onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
                  />
                ) : (
                  <span className="w-8 h-8 rounded border border-border-soft bg-panel/40 grid place-items-center text-xs font-bold text-muted shrink-0">
                    {(p.name || "?").charAt(0).toUpperCase()}
                  </span>
                )}
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold leading-tight group-hover:text-accent transition-colors truncate">
                    {p.name}
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-muted mt-0.5 tabular">
                    <span className="font-mono">P-{String(p.id).padStart(3, "0")}</span>
                    <span className="text-border-soft">·</span>
                    <span>{p.target_count} tgt</span>
                    {p.reports_open > 0 && (
                      <>
                        <span className="text-border-soft">·</span>
                        <span className="text-accent font-medium">{p.reports_open} open</span>
                      </>
                    )}
                  </div>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>

      {programs.length > 0 && (
        <div className="fixed bottom-0 left-0 right-0 border-t border-border bg-bg/95 backdrop-blur z-30">
          <div className="max-w-[1280px] mx-auto px-5">
            <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} label="programs" />
          </div>
        </div>
      )}
    </div>
  );
}
