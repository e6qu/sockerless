import { BrowserRouter, Routes, Route, Navigate } from "react-router";
import { ErrorBoundary, ToastProvider } from "@sockerless/ui-core/components";
import { isLoggedIn } from "./api.js";
import { BleephubShell } from "./components/Shell.js";
import { LoginPage } from "./pages/LoginPage.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { WorkflowsPage } from "./pages/WorkflowsPage.js";
import { WorkflowDetailPage } from "./pages/WorkflowDetailPage.js";
import { RunnersPage } from "./pages/RunnersPage.js";
import { ReposPage } from "./pages/ReposPage.js";
import { RepoDetailPage } from "./pages/RepoDetailPage.js";
import { IssuesPage } from "./pages/IssuesPage.js";
import { PullsPage } from "./pages/PullsPage.js";
import { ActionsPage } from "./pages/ActionsPage.js";
import { RunDetailPage } from "./pages/RunDetailPage.js";
import { RepoSecretsPage } from "./pages/RepoSecretsPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";
import { AppsPage } from "./pages/AppsPage.js";
import { OAuthPage } from "./pages/OAuthPage.js";

export function App() {
  if (!isLoggedIn()) {
    return (
      <ErrorBoundary>
        <BrowserRouter>
          <Routes>
            <Route path="/ui/login" element={<LoginPage />} />
            <Route path="/ui/*" element={<Navigate to="/ui/login" replace />} />
          </Routes>
        </BrowserRouter>
      </ErrorBoundary>
    );
  }

  return (
    <ErrorBoundary>
      <ToastProvider>
        <BrowserRouter>
          <BleephubShell>
            <Routes>
              <Route path="/ui/" element={<OverviewPage />} />
              <Route path="/ui/workflows" element={<WorkflowsPage />} />
              <Route path="/ui/workflows/:id" element={<WorkflowDetailPage />} />
              <Route path="/ui/runners" element={<RunnersPage />} />
              <Route path="/ui/repos" element={<ReposPage />} />
              <Route path="/ui/repos/:owner/:repo" element={<RepoDetailPage />} />
              <Route path="/ui/repos/:owner/:repo/issues" element={<IssuesPage />} />
              <Route path="/ui/repos/:owner/:repo/issues/:number" element={<IssuesPage />} />
              <Route path="/ui/repos/:owner/:repo/pulls" element={<PullsPage />} />
              <Route path="/ui/repos/:owner/:repo/pulls/:number" element={<PullsPage />} />
              <Route path="/ui/repos/:owner/:repo/actions" element={<ActionsPage />} />
              <Route path="/ui/repos/:owner/:repo/actions/runs/:runId" element={<RunDetailPage />} />
              <Route path="/ui/repos/:owner/:repo/settings/secrets" element={<RepoSecretsPage />} />
              <Route path="/ui/apps" element={<AppsPage />} />
              <Route path="/ui/oauth" element={<OAuthPage />} />
              <Route path="/ui/metrics" element={<MetricsPage />} />
              {/* A logged-in user hitting /ui/login (bookmark) or any
                  unknown /ui/* path lands back on the dashboard. */}
              <Route path="/ui/login" element={<Navigate to="/ui/" replace />} />
              <Route path="/ui/*" element={<Navigate to="/ui/" replace />} />
            </Routes>
          </BleephubShell>
        </BrowserRouter>
      </ToastProvider>
    </ErrorBoundary>
  );
}
