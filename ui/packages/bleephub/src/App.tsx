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
import { OrgReposPage } from "./pages/OrgReposPage.js";
import { RepoDetailPage } from "./pages/RepoDetailPage.js";
import { IssuesPage } from "./pages/IssuesPage.js";
import { PullsPage } from "./pages/PullsPage.js";
import { DiscussionsPage } from "./pages/DiscussionsPage.js";
import { ActionsPage } from "./pages/ActionsPage.js";
import { RunDetailPage } from "./pages/RunDetailPage.js";
import { RepoSettingsPage } from "./pages/RepoSettingsPage.js";
import { BranchProtectionPage } from "./pages/BranchProtectionPage.js";
import { SecretScanningPage } from "./pages/SecretScanningPage.js";
import { CodeScanningPage } from "./pages/CodeScanningPage.js";
import { DependabotPage } from "./pages/DependabotPage.js";
import { SecurityAdvisoriesPage } from "./pages/SecurityAdvisoriesPage.js";
import { ProjectsClassicPage } from "./pages/ProjectsClassicPage.js";
import { RepoSecretsPage } from "./pages/RepoSecretsPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";
import { AppsPage } from "./pages/AppsPage.js";
import { OAuthPage } from "./pages/OAuthPage.js";
import { UsersPage } from "./pages/UsersPage.js";
import { OrgsPage } from "./pages/OrgsPage.js";
import { TeamsPage } from "./pages/TeamsPage.js";
import { RulesetsPage } from "./pages/RulesetsPage.js";
import { AuditLogPage } from "./pages/AuditLogPage.js";
import { StorageHealthPage } from "./pages/StorageHealthPage.js";
import { GistsPage } from "./pages/GistsPage.js";
import { NotificationsPage } from "./pages/NotificationsPage.js";
import { MigrationsPage } from "./pages/MigrationsPage.js";
import { CodespacesPage } from "./pages/CodespacesPage.js";
import { PackagesPage } from "./pages/PackagesPage.js";
import { InsightsPage } from "./pages/InsightsPage.js";
import { OrgGovernancePage } from "./pages/OrgGovernancePage.js";
import { CopilotPage } from "./pages/CopilotPage.js";
import { EnterprisePage } from "./pages/EnterprisePage.js";
import { DeploymentsPage } from "./pages/DeploymentsPage.js";
import { WebhookDeliveriesPage } from "./pages/WebhookDeliveriesPage.js";
import { OrgHooksPage } from "./pages/OrgHooksPage.js";
import { SearchPage } from "./pages/SearchPage.js";
import { AccountPage } from "./pages/AccountPage.js";
import { RepoSocialPage } from "./pages/RepoSocialPage.js";

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
              <Route path="/ui/orgs/:org/repos" element={<OrgReposPage />} />
              <Route path="/ui/orgs/:org/rulesets" element={<RulesetsPage />} />
              <Route path="/ui/orgs/:org/governance" element={<OrgGovernancePage />} />
              <Route path="/ui/orgs/:org/copilot" element={<CopilotPage />} />
              <Route path="/ui/repos/:owner/:repo" element={<RepoDetailPage />} />
              <Route path="/ui/repos/:owner/:repo/issues" element={<IssuesPage />} />
              <Route path="/ui/repos/:owner/:repo/issues/:number" element={<IssuesPage />} />
              <Route path="/ui/repos/:owner/:repo/labels" element={<IssuesPage view="labels" />} />
              <Route path="/ui/repos/:owner/:repo/milestones" element={<IssuesPage view="milestones" />} />
              <Route path="/ui/repos/:owner/:repo/insights" element={<InsightsPage />} />
              <Route path="/ui/repos/:owner/:repo/pulls" element={<PullsPage />} />
              <Route path="/ui/repos/:owner/:repo/pulls/:number" element={<PullsPage />} />
              <Route path="/ui/repos/:owner/:repo/discussions" element={<DiscussionsPage />} />
              <Route path="/ui/repos/:owner/:repo/discussions/:number" element={<DiscussionsPage />} />
              <Route path="/ui/repos/:owner/:repo/actions" element={<ActionsPage />} />
              <Route path="/ui/repos/:owner/:repo/actions/runs/:runId" element={<RunDetailPage />} />
              <Route path="/ui/repos/:owner/:repo/settings" element={<RepoSettingsPage />} />
              <Route path="/ui/repos/:owner/:repo/settings/branch-protection" element={<BranchProtectionPage />} />
              <Route path="/ui/repos/:owner/:repo/security/secret-scanning" element={<SecretScanningPage />} />
              <Route path="/ui/repos/:owner/:repo/security/code-scanning" element={<CodeScanningPage />} />
              <Route path="/ui/repos/:owner/:repo/security/dependabot" element={<DependabotPage />} />
              <Route path="/ui/repos/:owner/:repo/security/advisories" element={<SecurityAdvisoriesPage />} />
              <Route path="/ui/repos/:owner/:repo/projects-classic" element={<ProjectsClassicPage />} />
              <Route path="/ui/repos/:owner/:repo/settings/secrets" element={<RepoSecretsPage />} />
              <Route path="/ui/apps" element={<AppsPage />} />
              <Route path="/ui/oauth" element={<OAuthPage />} />
              <Route path="/ui/metrics" element={<MetricsPage />} />
              <Route path="/ui/gists" element={<GistsPage />} />
              <Route path="/ui/notifications" element={<NotificationsPage />} />
              <Route path="/ui/packages" element={<PackagesPage />} />
              <Route path="/ui/orgs/:org/packages" element={<PackagesPage />} />
              <Route path="/ui/repos/:owner/:repo/packages" element={<PackagesPage />} />
              <Route path="/ui/migrations" element={<MigrationsPage />} />
              <Route path="/ui/codespaces" element={<CodespacesPage />} />
              <Route path="/ui/repos/:owner/:repo/codespaces" element={<CodespacesPage />} />
              <Route path="/ui/admin/users" element={<UsersPage />} />
              <Route path="/ui/admin/orgs" element={<OrgsPage />} />
              <Route path="/ui/admin/teams" element={<TeamsPage />} />
              <Route path="/ui/admin/enterprise" element={<EnterprisePage />} />
              <Route path="/ui/admin/audit-log" element={<AuditLogPage />} />
              <Route path="/ui/admin/storage" element={<StorageHealthPage />} />
              {/* Deployments + webhook deliveries + Pages */}
              <Route path="/ui/repos/:owner/:repo/deployments" element={<DeploymentsPage />} />
              <Route path="/ui/repos/:owner/:repo/hooks/:hookId/deliveries" element={<WebhookDeliveriesPage />} />
              <Route path="/ui/orgs/:org/hooks" element={<OrgHooksPage />} />
              <Route path="/ui/orgs/:org/hooks/:hookId/deliveries" element={<WebhookDeliveriesPage />} />
              {/* Search + repo social + account */}
              <Route path="/ui/search" element={<SearchPage />} />
              <Route path="/ui/account" element={<AccountPage />} />
              <Route path="/ui/repos/:owner/:repo/stargazers" element={<RepoSocialPage kind="stargazers" />} />
              <Route path="/ui/repos/:owner/:repo/watchers" element={<RepoSocialPage kind="watchers" />} />
              <Route path="/ui/repos/:owner/:repo/forks" element={<RepoSocialPage kind="forks" />} />
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
