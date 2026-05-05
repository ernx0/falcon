import { ReactNode, ButtonHTMLAttributes, InputHTMLAttributes, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";
import clsx from "clsx";

type Variant = "primary" | "secondary" | "accent" | "ghost" | "danger";
type Size = "sm" | "md";

export function Button({
  variant = "secondary",
  size = "md",
  className,
  children,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: Size }) {
  const v =
    variant === "primary" ? "btn-primary"
    : variant === "accent" ? "btn-accent"
    : variant === "ghost" ? "btn-ghost"
    : variant === "danger" ? "btn-danger"
    : "btn-secondary";
  return (
    <button {...rest} className={clsx(v, size === "sm" && "btn-sm", className)}>
      {children}
    </button>
  );
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={clsx("input", props.className)} />;
}

export function Select({ children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={clsx("input", props.className)}>{children}</select>;
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea {...props} className={clsx("input", props.className)} />;
}

export function Card({
  children, className, hover, shadow,
}: { children: ReactNode; className?: string; hover?: boolean; shadow?: boolean }) {
  return (
    <div className={clsx(shadow ? "card-shadow" : hover ? "card-hover" : "card", className)}>
      {children}
    </div>
  );
}

export function CardHeader({
  title, right, meta,
}: { title: string; right?: ReactNode; meta?: string }) {
  return (
    <div className="card-header">
      <div className="flex items-center gap-2 min-w-0">
        <span className="card-header-title truncate">{title}</span>
        {meta && <span className="card-header-meta">· {meta}</span>}
      </div>
      {right && <span className="card-header-meta">{right}</span>}
    </div>
  );
}

export function Section({
  number, title, right, action, children, description,
}: {
  number?: string;
  title: string;
  right?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  description?: string;
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-end gap-3 border-b border-border pb-2">
        <div className="min-w-0">
          <h2 className="text-base font-semibold tracking-tight">{title}</h2>
          {description && <p className="text-sm text-muted mt-0.5">{description}</p>}
        </div>
        {right && <span className="ml-auto section-tag">{right}</span>}
        {!right && number && <span className="ml-auto section-tag tabular">{number}</span>}
        {action && <span className={clsx(!right && !number && "ml-auto")}>{action}</span>}
      </div>
      {children}
    </section>
  );
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="block">
      <span className="label block mb-1.5">{label}</span>
      {children}
      {hint && <span className="text-xs text-muted block mt-1">{hint}</span>}
    </label>
  );
}

const sevStyle: Record<string, string> = {
  critical: "bg-danger text-bg border-danger",
  high:     "bg-accent text-bg border-accent",
  medium:   "bg-warn text-bg border-warn",
  low:      "bg-bg text-ink border-border",
  info:     "bg-panel2 text-muted border-border-soft",
};
export function SeverityBadge({ s }: { s: string }) {
  return <span className={clsx("badge tabular", sevStyle[s] || sevStyle.info)}>{s}</span>;
}

// PriorityBadge renders Bugcrowd-style P1-P5 colored badges. P1 is the most
// severe (critical), P5 the least (info). Unknown priority falls back to a
// neutral muted style.
const prioStyle: Record<number, string> = {
  1: "bg-crit text-bg border-crit",
  2: "bg-danger text-bg border-danger",
  3: "bg-accent text-bg border-accent",
  4: "bg-warn text-bg border-warn",
  5: "bg-panel2 text-subtle border-border",
};
export function PriorityBadge({ p, className }: { p: number; className?: string }) {
  if (!p) {
    return <span className={clsx("badge bg-panel2 text-muted border-border-soft", className)}>—</span>;
  }
  return <span className={clsx("badge tabular", prioStyle[p] || prioStyle[5], className)}>P{p}</span>;
}

const statusStyle: Record<string, { dot: string; text: string }> = {
  success:   { dot: "bg-success",  text: "text-success" },
  running:   { dot: "bg-accent animate-pulse", text: "text-accent" },
  queued:    { dot: "bg-muted",    text: "text-muted" },
  pending:   { dot: "bg-muted",    text: "text-muted" },
  failed:    { dot: "bg-danger",   text: "text-danger" },
  partial:   { dot: "bg-warn",     text: "text-warn" },
  draft:     { dot: "bg-muted",    text: "text-muted" },
  submitted: { dot: "bg-accent",   text: "text-accent" },
  triaged:   { dot: "bg-warn",     text: "text-warn" },
  resolved:  { dot: "bg-success",  text: "text-success" },
  duplicate: { dot: "bg-muted",    text: "text-muted" },
  n_a:       { dot: "bg-muted",    text: "text-muted" },
};
export function StatusBadge({ s }: { s: string }) {
  const st = statusStyle[s] || statusStyle.queued;
  return (
    <span className={clsx("inline-flex items-center gap-1.5 text-xs font-medium tabular", st.text)}>
      <span className={clsx("w-1.5 h-1.5 rounded-full", st.dot)} />
      {s.replace("_", " ")}
    </span>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={clsx("skeleton h-4 w-full", className)} />;
}

export function EmptyState({
  icon, title, hint, action,
}: { icon?: ReactNode; title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center text-center py-10 px-4">
      {icon && (
        <div className="text-muted mb-4 border border-border rounded-lg p-3 inline-block bg-panel/40">
          {icon}
        </div>
      )}
      <div className="text-base font-semibold text-ink">{title}</div>
      {hint && <div className="text-sm text-muted mt-1.5 max-w-sm leading-relaxed">{hint}</div>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function Pagination({
  page, pageSize, total, onChange, label = "rows",
}: {
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number) => void;
  label?: string;
}) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(1, page), totalPages);
  const from = total === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const to = Math.min(safePage * pageSize, total);

  return (
    <div className="px-4 py-2 flex items-center justify-between gap-3 text-xs text-muted">
      <span className="tabular">
        {from.toLocaleString()}–{to.toLocaleString()} of {total.toLocaleString()} {label}
      </span>
      <div className="flex items-center gap-1">
        <button
          onClick={() => onChange(1)}
          disabled={safePage <= 1}
          className="btn-ghost btn-sm disabled:opacity-30"
          title="First"
        >«</button>
        <button
          onClick={() => onChange(safePage - 1)}
          disabled={safePage <= 1}
          className="btn-ghost btn-sm disabled:opacity-30"
          title="Previous"
        >‹ Prev</button>
        <span className="px-3 text-ink tabular font-medium">
          {safePage} / {totalPages}
        </span>
        <button
          onClick={() => onChange(safePage + 1)}
          disabled={safePage >= totalPages}
          className="btn-ghost btn-sm disabled:opacity-30"
          title="Next"
        >Next ›</button>
        <button
          onClick={() => onChange(totalPages)}
          disabled={safePage >= totalPages}
          className="btn-ghost btn-sm disabled:opacity-30"
          title="Last"
        >»</button>
      </div>
    </div>
  );
}

export function Stat({
  label, value, hint, accent,
}: { label: string; value: ReactNode; hint?: string; accent?: boolean; number?: string }) {
  return (
    <div className={clsx(
      "border border-border rounded-lg p-4",
      accent ? "bg-accent text-bg border-accent" : "bg-bg text-ink"
    )}>
      <div className={clsx("text-xs font-medium uppercase tracking-wide", accent ? "text-bg/80" : "text-muted")}>
        {label}
      </div>
      <div className="text-2xl font-bold mt-1.5 tabular leading-none tracking-tight">{value}</div>
      {hint && <div className={clsx("text-xs mt-1.5", accent ? "text-bg/70" : "text-muted")}>{hint}</div>}
    </div>
  );
}

export function Icon({ name, className = "w-4 h-4" }: { name: string; className?: string }) {
  const paths: Record<string, ReactNode> = {
    plus: <path d="M12 5v14M5 12h14" />,
    search: <><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></>,
    play: <polygon points="6 4 20 12 6 20 6 4" />,
    trash: <><path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" /><path d="M10 11v6M14 11v6" /></>,
    chevronLeft: <polyline points="15 18 9 12 15 6" />,
    chevronRight: <polyline points="9 18 15 12 9 6" />,
    chevronDown: <polyline points="6 9 12 15 18 9" />,
    target: <><circle cx="12" cy="12" r="10" /><circle cx="12" cy="12" r="6" /><circle cx="12" cy="12" r="2" /></>,
    shield: <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />,
    bug: <><path d="M9 9V5a3 3 0 0 1 6 0v4" /><rect x="6" y="9" width="12" height="11" rx="1" /><path d="M3 13h3M18 13h3M3 8l3 2M18 10l3-2M3 18l3-2M18 16l3 2" /></>,
    activity: <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />,
    folder: <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" />,
    logout: <><path d="M15 17l5-5-5-5M20 12H9M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h7" /></>,
    save: <><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" /><polyline points="17 21 17 13 7 13 7 21" /><polyline points="7 3 7 8 15 8" /></>,
    external: <><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" /><polyline points="15 3 21 3 21 9" /><line x1="10" y1="14" x2="21" y2="3" /></>,
    grid: <><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><rect x="14" y="14" width="7" height="7" rx="1" /></>,
    terminal: <><polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" /></>,
  };
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
      {paths[name]}
    </svg>
  );
}
