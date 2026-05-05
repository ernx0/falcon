import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, setToken } from "../lib/api";
import { Button, Input, Field } from "../components/ui";

export function Login() {
  const [email, setEmail] = useState("admin@falcon.local");
  const [password, setPassword] = useState("admin");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const nav = useNavigate();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setLoading(true);
    try {
      const r = await api.login(email, password);
      setToken(r.token);
      nav("/programs");
    } catch (e: any) {
      setErr(e.message || "Login failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen grid place-items-center px-4">
      <div className="w-full max-w-sm">
        <div className="flex items-center gap-3 mb-8 justify-center">
          <span className="w-11 h-11 bg-ink text-bg rounded-lg grid place-items-center font-bold text-xl">F</span>
          <div className="flex flex-col leading-tight">
            <span className="text-lg font-semibold tracking-tight">Falcon</span>
            <span className="text-xs text-muted">Recon engine · v0.1.0</span>
          </div>
        </div>

        <div className="card-shadow">
          <div className="card-header">
            <span className="card-header-title">Sign in</span>
            <span className="card-header-meta">Auth</span>
          </div>
          <form onSubmit={submit} className="p-5 flex flex-col gap-4">
            <Field label="Email">
              <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoFocus autoComplete="username" />
            </Field>
            <Field label="Password">
              <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
            </Field>
            {err && (
              <div className="text-sm text-danger bg-danger/10 border border-danger/30 rounded-md px-3 py-2">
                {err}
              </div>
            )}
            <Button type="submit" variant="primary" disabled={loading} className="w-full py-2.5">
              {loading ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </div>

        <div className="text-xs text-muted text-center mt-5">
          Credentials configured via environment.
        </div>
      </div>
    </div>
  );
}
