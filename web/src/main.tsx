import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "./index.css";
import { App } from "./App";
import { Login } from "./pages/Login";
import { Programs } from "./pages/Programs";
import { ProgramDetail } from "./pages/ProgramDetail";
import { ReportDetail } from "./pages/ReportDetail";
import { RunDetail } from "./pages/RunDetail";
import { Search } from "./pages/Search";

const qc = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5_000, refetchOnWindowFocus: false, retry: 1 },
  },
});

function RequireAuth({ children }: { children: React.ReactNode }) {
  const tok = localStorage.getItem("falcon_token");
  if (!tok) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={<RequireAuth><App /></RequireAuth>}>
            <Route path="/" element={<Navigate to="/programs" replace />} />
            <Route path="/programs" element={<Programs />} />
            <Route path="/programs/:id" element={<ProgramDetail />} />
            <Route path="/reports/:id" element={<ReportDetail />} />
            <Route path="/runs/:id" element={<RunDetail />} />
            <Route path="/search" element={<Search />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>
);
