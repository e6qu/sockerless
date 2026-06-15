import { BrowserRouter, Routes, Route, Navigate } from "react-router";
import { ErrorBoundary, ToastProvider } from "@sockerless/ui-core/components";
import { BleeplabShell } from "./components/Shell.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { ProjectsPage } from "./pages/ProjectsPage.js";
import { ProjectDetailPage } from "./pages/ProjectDetailPage.js";
import { PipelinesPage } from "./pages/PipelinesPage.js";
import { PipelineDetailPage } from "./pages/PipelineDetailPage.js";
import { JobDetailPage } from "./pages/JobDetailPage.js";
import { RunnersPage } from "./pages/RunnersPage.js";

export function App() {
  return (
    <ErrorBoundary>
      <ToastProvider>
        <BrowserRouter>
          <BleeplabShell>
            <Routes>
              <Route path="/ui/" element={<OverviewPage />} />
              <Route path="/ui/projects" element={<ProjectsPage />} />
              <Route path="/ui/projects/:id" element={<ProjectDetailPage />} />
              <Route path="/ui/pipelines" element={<PipelinesPage />} />
              <Route path="/ui/pipelines/:id" element={<PipelineDetailPage />} />
              <Route path="/ui/jobs/:id" element={<JobDetailPage />} />
              <Route path="/ui/runners" element={<RunnersPage />} />
              {/* Unknown /ui/* → dashboard. Mirrors the server-side SPA
                  fallback to index.html. */}
              <Route path="/ui/*" element={<Navigate to="/ui/" replace />} />
            </Routes>
          </BleeplabShell>
        </BrowserRouter>
      </ToastProvider>
    </ErrorBoundary>
  );
}
