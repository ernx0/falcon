const API_URL = (import.meta as any).env?.VITE_API_URL || "http://localhost:8080";

export class ApiError extends Error {
  constructor(public status: number, msg: string) {
    super(msg);
  }
}

export type Page<T> = {
  items: T[];
  total: number;
  page: number;
  page_size: number;
};

export type PageParams = { page?: number; page_size?: number };

function token(): string | null {
  return localStorage.getItem("falcon_token");
}

export function setToken(t: string | null) {
  if (t) localStorage.setItem("falcon_token", t);
  else localStorage.removeItem("falcon_token");
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const tk = token();
  if (tk) headers["Authorization"] = `Bearer ${tk}`;
  const r = await fetch(API_URL + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (r.status === 401) {
    setToken(null);
    if (location.pathname !== "/login") location.href = "/login";
  }
  if (!r.ok) {
    const text = await r.text().catch(() => r.statusText);
    throw new ApiError(r.status, text);
  }
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

function pgQS(p: PageParams = {}, extra: Record<string, string> = {}): string {
  const usp = new URLSearchParams();
  if (p.page) usp.set("page", String(p.page));
  if (p.page_size) usp.set("page_size", String(p.page_size));
  for (const [k, v] of Object.entries(extra)) {
    if (v) usp.set(k, v);
  }
  const s = usp.toString();
  return s ? "?" + s : "";
}

export const api = {
  login: (email: string, password: string) =>
    request<{ token: string; user: { id: number; email: string }; expires_at: string }>(
      "POST", "/api/auth/login", { email, password }),
  me: () => request<{ id: number; email: string }>("GET", "/api/auth/me"),

  programs: (p: PageParams = {}) =>
    request<Page<any>>("GET", `/api/programs${pgQS(p)}`),
  programGet: (id: number) => request<any>("GET", `/api/programs/${id}`),
  programCreate: (b: any) => request<any>("POST", "/api/programs", b),
  programUpdate: (id: number, b: any) => request<any>("PATCH", `/api/programs/${id}`, b),
  programDelete: (id: number) => request<void>("DELETE", `/api/programs/${id}`),

  scope: (pid: number, p: PageParams & { scope?: "all" | "scannable" } = {}) => {
    const extra: Record<string, string> = {};
    if (p.scope) extra.scope = p.scope;
    return request<Page<any>>("GET", `/api/programs/${pid}/scope${pgQS(p, extra)}`);
  },
  scopeCreate: (pid: number, b: any) => request<any>("POST", `/api/programs/${pid}/scope`, b),
  scopeUpdate: (id: number, b: any) => request<any>("PATCH", `/api/scope/${id}`, b),
  scopeDelete: (id: number) => request<void>("DELETE", `/api/scope/${id}`),
  scopeRun: (id: number) => request<{ run_id: number }>("POST", `/api/scope/${id}/run`),
  scopeRunAll: (pid: number) =>
    request<{ queued: number; failed: number; total: number; run_ids: { run_id: number; scope_id: number; value: string }[] }>(
      "POST",
      `/api/programs/${pid}/scope/run-all`,
    ),

  oos: (pid: number, p: PageParams = {}) =>
    request<Page<any>>("GET", `/api/programs/${pid}/oos${pgQS(p)}`),
  oosCreate: (pid: number, b: any) => request<any>("POST", `/api/programs/${pid}/oos`, b),
  oosDelete: (id: number) => request<void>("DELETE", `/api/oos/${id}`),

  reports: (pid: number, p: PageParams = {}) =>
    request<Page<any>>("GET", `/api/programs/${pid}/reports${pgQS(p)}`),
  reportGet: (id: number) => request<any>("GET", `/api/reports/${id}`),
  reportCreate: (pid: number, b: any) => request<any>("POST", `/api/programs/${pid}/reports`, b),
  reportUpdate: (id: number, b: any) => request<any>("PATCH", `/api/reports/${id}`, b),
  reportDelete: (id: number) => request<void>("DELETE", `/api/reports/${id}`),

  runs: (pid: number, p: PageParams = {}) =>
    request<Page<any>>("GET", `/api/programs/${pid}/runs${pgQS(p)}`),
  runGet: (id: number) => request<{ run: any; steps: any[] }>("GET", `/api/runs/${id}`),
  runDelete: (id: number) => request<void>("DELETE", `/api/runs/${id}`),
  runsDeleteAll: (pid: number) =>
    request<{ deleted: number }>("DELETE", `/api/programs/${pid}/runs`),

  hosts: (
    pid: number,
    params: HostsQuery & PageParams = {}
  ) => {
    const extra: Record<string, string> = {};
    if (params.q) extra.q = params.q;
    if (params.oos) extra.oos = "1";
    if (params.has_ips) extra.has_ips = "1";
    if (params.has_services) extra.has_services = "1";
    if (params.has_endpoints) extra.has_endpoints = "1";
    if (params.cdn === true)  extra.cdn = "1";
    if (params.cdn === false) extra.cdn = "0";
    if (params.min_ips)       extra.min_ips = String(params.min_ips);
    if (params.min_services)  extra.min_services = String(params.min_services);
    if (params.min_endpoints) extra.min_endpoints = String(params.min_endpoints);
    if (params.tech)   extra.tech = params.tech;
    if (params.status) extra.status = params.status;
    if (params.port)   extra.port = String(params.port);
    return request<Page<any>>(
      "GET",
      `/api/programs/${pid}/hosts${pgQS(params, extra)}`,
    );
  },
  hostDetail: (pid: number, hostId: number) =>
    request<{ host: any; ips: any[]; services: any[]; endpoints: any[] }>(
      "GET",
      `/api/programs/${pid}/hosts/${hostId}`,
    ),
  hostUpdate: (pid: number, hostId: number, b: { out_of_scope: boolean }) =>
    request<any>("PATCH", `/api/programs/${pid}/hosts/${hostId}`, b),
  services: (pid: number, params: { oos?: boolean } & PageParams = {}) => {
    const extra: Record<string, string> = {};
    if (params.oos) extra.oos = "1";
    return request<Page<any>>("GET", `/api/programs/${pid}/services${pgQS(params, extra)}`);
  },
  endpoints: (
    pid: number,
    params: { q?: string; oos?: boolean } & PageParams = {}
  ) => {
    const extra: Record<string, string> = {};
    if (params.q) extra.q = params.q;
    if (params.oos) extra.oos = "1";
    return request<Page<any>>("GET", `/api/programs/${pid}/endpoints${pgQS(params, extra)}`);
  },
  search: (q: string) =>
    request<{
      programs: any[];
      scope: any[];
      hosts: any[];
      services: any[];
      endpoints: any[];
      reports: any[];
    }>("GET", `/api/search?q=${encodeURIComponent(q)}`),
  vrt: () =>
    request<{ metadata: any; content: VRTNode[] }>("GET", "/api/vrt"),
  vrtRefresh: () =>
    request<{ metadata: any; content: VRTNode[] }>("POST", "/api/vrt/refresh"),

  // Fetch a run-step artifact file as raw text for inline preview. Path is
  // resolved against the worker artifacts dir on the server side.
  artifact: async (path: string): Promise<{ text: string; size: number; truncated: boolean }> => {
    const tk = token();
    const headers: Record<string, string> = {};
    if (tk) headers["Authorization"] = `Bearer ${tk}`;
    const r = await fetch(`${API_URL}/api/artifacts?path=${encodeURIComponent(path)}`, { headers });
    if (r.status === 401) {
      setToken(null);
      if (location.pathname !== "/login") location.href = "/login";
    }
    if (!r.ok) {
      const t = await r.text().catch(() => r.statusText);
      throw new ApiError(r.status, t);
    }
    // Cap at 1 MiB to keep the browser responsive on huge jsonl files.
    const max = 1024 * 1024;
    const reader = r.body?.getReader();
    if (!reader) {
      const text = await r.text();
      return { text, size: text.length, truncated: false };
    }
    let received = 0;
    const chunks: Uint8Array[] = [];
    let truncated = false;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      received += value.byteLength;
      if (received > max) {
        truncated = true;
        const slice = value.slice(0, Math.max(0, max - (received - value.byteLength)));
        chunks.push(slice);
        try { await reader.cancel(); } catch {}
        break;
      }
      chunks.push(value);
    }
    const total = chunks.reduce((s, c) => s + c.byteLength, 0);
    const buf = new Uint8Array(total);
    let off = 0;
    for (const c of chunks) { buf.set(c, off); off += c.byteLength; }
    return { text: new TextDecoder().decode(buf), size: received, truncated };
  },
};

export type VRTNode = {
  id: string;
  name: string;
  type?: string;
  priority?: number;
  children?: VRTNode[];
};

export type VRTOption = {
  id: string;        // dotted path, e.g. "ai.adversarial.misclass"
  label: string;     // joined names "AI › Adversarial › Misclass"
  priority: number;  // 1-5 (0 if missing)
};

export function flattenVRT(nodes: VRTNode[], prefix = "", labelPrefix = ""): VRTOption[] {
  const out: VRTOption[] = [];
  for (const n of nodes) {
    const id = prefix ? `${prefix}.${n.id}` : n.id;
    const label = labelPrefix ? `${labelPrefix} › ${n.name}` : n.name;
    if (n.children && n.children.length) {
      out.push(...flattenVRT(n.children, id, label));
    } else {
      out.push({ id, label, priority: n.priority ?? 0 });
    }
  }
  return out;
}

export function severityFromPriority(p: number): string {
  // Falls back to "none" — the bucket reserved for VRT nodes that don't
  // carry an explicit priority (parent categories etc.). Distinct from
  // "info" (P5), which is the lowest *explicit* bucket.
  return ({ 1: "critical", 2: "high", 3: "medium", 4: "low", 5: "info" } as Record<number, string>)[p] || "none";
}

export function priorityFromVRT(vrtId: string, options: VRTOption[]): number {
  if (!vrtId) return 0;
  return options.find((o) => o.id === vrtId)?.priority || 0;
}

export function priorityFromSeverity(s: string): number {
  return ({ critical: 1, high: 2, medium: 3, low: 4, info: 5 } as Record<string, number>)[s] || 0;
}

export type HostsQuery = {
  q?: string;
  oos?: boolean;
  has_ips?: boolean;
  has_services?: boolean;
  has_endpoints?: boolean;
  cdn?: boolean;
  min_ips?: number;
  min_services?: number;
  min_endpoints?: number;
  tech?: string;
  status?: string;
  port?: number;
};
