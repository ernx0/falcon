import { Link, NavLink, Outlet, useNavigate } from "react-router-dom";
import { setToken } from "./lib/api";
import { useTheme } from "./hooks/useTheme";

export function App() {
  const nav = useNavigate();
  const [theme, , toggleTheme] = useTheme();
  const logout = () => {
    setToken(null);
    nav("/login");
  };

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b border-border bg-bg/80 backdrop-blur sticky top-0 z-20">
        <div className="max-w-[1280px] mx-auto px-5 h-14 flex items-center gap-5">
          <Link to="/" className="flex items-center gap-2.5 group">
            <span className="w-8 h-8 bg-ink text-bg rounded-md grid place-items-center font-bold text-base group-hover:bg-accent transition-colors">F</span>
            <span className="text-sm font-semibold tracking-tight">Falcon</span>
          </Link>
          <nav className="flex items-center gap-1 ml-2">
            <NavItem to="/programs" label="Programs" />
            <NavItem to="/search" label="Search" />
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <span className="hidden md:inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="inline-block w-1.5 h-1.5 bg-success rounded-full" />
              Online
            </span>
            <button
              onClick={toggleTheme}
              className="text-subtle hover:text-ink hover:bg-panel2 w-8 h-8 grid place-items-center rounded-md transition-colors"
              title={theme === "dark" ? "Switch to light" : "Switch to dark"}
              aria-label="Toggle theme"
            >
              {theme === "dark" ? <SunIcon /> : <MoonIcon />}
            </button>
            <button
              onClick={logout}
              className="text-sm text-subtle hover:text-ink px-3 py-1.5 rounded-md hover:bg-panel2 transition-colors"
            >
              Log out
            </button>
          </div>
        </div>
      </header>

      <main className="flex-1">
        <div className="max-w-[1280px] mx-auto px-5 py-7">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

function NavItem({ to, label }: { to: string; label: string }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `text-sm font-medium px-3 py-1.5 rounded-md transition-colors ${
          isActive ? "text-ink bg-panel2" : "text-subtle hover:text-ink hover:bg-panel2/60"
        }`
      }
    >
      {label}
    </NavLink>
  );
}

function SunIcon() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2" />
    </svg>
  );
}
function MoonIcon() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
    </svg>
  );
}
