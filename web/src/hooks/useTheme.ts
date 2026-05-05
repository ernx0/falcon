import { useEffect, useState } from "react";

export type Theme = "light" | "dark";

function readTheme(): Theme {
  if (typeof document === "undefined") return "light";
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function useTheme(): [Theme, (t: Theme) => void, () => void] {
  const [theme, setThemeState] = useState<Theme>(() => readTheme());

  const applyTheme = (t: Theme) => {
    const root = document.documentElement;
    if (t === "dark") root.classList.add("dark");
    else root.classList.remove("dark");
    try { localStorage.setItem("falcon_theme", t); } catch {}
    setThemeState(t);
  };

  const toggle = () => applyTheme(theme === "dark" ? "light" : "dark");

  useEffect(() => {
    setThemeState(readTheme());
  }, []);

  return [theme, applyTheme, toggle];
}
