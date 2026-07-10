import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router";
import { ErrorBoundary, ToastProvider } from "@sockerless/ui-core/components";
import { isLoggedIn } from "./api.js";
import { BleephubShell } from "./components/Shell.js";

const LoginPage = lazy(() => import("./pages/LoginPage.js").then(({ LoginPage }) => ({ default: LoginPage })));
const OverviewPage = lazy(() => import("./pages/OverviewPage.js").then(({ OverviewPage }) => ({ default: OverviewPage })));
const WorkflowsPage = lazy(() => import("./pages/WorkflowsPage.js").then(({ WorkflowsPage }) => ({ default: WorkflowsPage })));
const WorkflowDetailPage = lazy(() => import("./pages/WorkflowDetailPage.js").then(({ WorkflowDetailPage }) => ({ default: WorkflowDetailPage })));
const RunnersPage = lazy(() => import("./pages/RunnersPage.js").then(({ RunnersPage }) => ({ default: RunnersPage })));
const ReposPage = lazy(() => import("./pages/ReposPage.js").then(({ ReposPage }) => ({ default: ReposPage })));
const OrgReposPage = lazy(() => import("./pages/OrgReposPage.js").then(({ OrgReposPage }) => ({ default: OrgReposPage })));
const RepoDetailPage = lazy(() => import("./pages/RepoDetailPage.js").then(({ RepoDetailPage }) => ({ default: RepoDetailPage })));
const IssuesPage = lazy(() => import("./pages/IssuesPage.js").then(({ IssuesPage }) => ({ default: IssuesPage })));
const PullsPage = lazy(() => import("./pages/PullsPage.js").then(({ PullsPage }) => ({ default: PullsPage })));
const DiscussionsPage = lazy(() => import("./pages/DiscussionsPage.js").then(({ DiscussionsPage }) => ({ default: DiscussionsPage })));
const ActionsPage = lazy(() => import("./pages/ActionsPage.js").then(({ ActionsPage }) => ({ default: ActionsPage })));
const RunDetailPage = lazy(() => import("./pages/RunDetailPage.js").then(({ RunDetailPage }) => ({ default: RunDetailPage })));
const RepoSettingsPage = lazy(() => import("./pages/RepoSettingsPage.js").then(({ RepoSettingsPage }) => ({ default: RepoSettingsPage })));
const BranchProtectionPage = lazy(() => import("./pages/BranchProtectionPage.js").then(({ BranchProtectionPage }) => ({ default: BranchProtectionPage })));
const SecretScanningPage = lazy(() => import("./pages/SecretScanningPage.js").then(({ SecretScanningPage }) => ({ default: SecretScanningPage })));
const CodeScanningPage = lazy(() => import("./pages/CodeScanningPage.js").then(({ CodeScanningPage }) => ({ default: CodeScanningPage })));
const DependabotPage = lazy(() => import("./pages/DependabotPage.js").then(({ DependabotPage }) => ({ default: DependabotPage })));
const SecurityAdvisoriesPage = lazy(() => import("./pages/SecurityAdvisoriesPage.js").then(({ SecurityAdvisoriesPage }) => ({ default: SecurityAdvisoriesPage })));
const ProjectsClassicPage = lazy(() => import("./pages/ProjectsClassicPage.js").then(({ ProjectsClassicPage }) => ({ default: ProjectsClassicPage })));
const RepoSecretsPage = lazy(() => import("./pages/RepoSecretsPage.js").then(({ RepoSecretsPage }) => ({ default: RepoSecretsPage })));
const MetricsPage = lazy(() => import("./pages/MetricsPage.js").then(({ MetricsPage }) => ({ default: MetricsPage })));
const AppsPage = lazy(() => import("./pages/AppsPage.js").then(({ AppsPage }) => ({ default: AppsPage })));
const OAuthPage = lazy(() => import("./pages/OAuthPage.js").then(({ OAuthPage }) => ({ default: OAuthPage })));
const UsersPage = lazy(() => import("./pages/UsersPage.js").then(({ UsersPage }) => ({ default: UsersPage })));
const OrgsPage = lazy(() => import("./pages/OrgsPage.js").then(({ OrgsPage }) => ({ default: OrgsPage })));
const TeamsPage = lazy(() => import("./pages/TeamsPage.js").then(({ TeamsPage }) => ({ default: TeamsPage })));
const RulesetsPage = lazy(() => import("./pages/RulesetsPage.js").then(({ RulesetsPage }) => ({ default: RulesetsPage })));
const AuditLogPage = lazy(() => import("./pages/AuditLogPage.js").then(({ AuditLogPage }) => ({ default: AuditLogPage })));
const GistsPage = lazy(() => import("./pages/GistsPage.js").then(({ GistsPage }) => ({ default: GistsPage })));
const NotificationsPage = lazy(() => import("./pages/NotificationsPage.js").then(({ NotificationsPage }) => ({ default: NotificationsPage })));
const MigrationsPage = lazy(() => import("./pages/MigrationsPage.js").then(({ MigrationsPage }) => ({ default: MigrationsPage })));
const CodespacesPage = lazy(() => import("./pages/CodespacesPage.js").then(({ CodespacesPage }) => ({ default: CodespacesPage })));
const PackagesPage = lazy(() => import("./pages/PackagesPage.js").then(({ PackagesPage }) => ({ default: PackagesPage })));
const InsightsPage = lazy(() => import("./pages/InsightsPage.js").then(({ InsightsPage }) => ({ default: InsightsPage })));
const OrgGovernancePage = lazy(() => import("./pages/OrgGovernancePage.js").then(({ OrgGovernancePage }) => ({ default: OrgGovernancePage })));
const CopilotPage = lazy(() => import("./pages/CopilotPage.js").then(({ CopilotPage }) => ({ default: CopilotPage })));
const EnterprisePage = lazy(() => import("./pages/EnterprisePage.js").then(({ EnterprisePage }) => ({ default: EnterprisePage })));
const DeploymentsPage = lazy(() => import("./pages/DeploymentsPage.js").then(({ DeploymentsPage }) => ({ default: DeploymentsPage })));
const WebhookDeliveriesPage = lazy(() => import("./pages/WebhookDeliveriesPage.js").then(({ WebhookDeliveriesPage }) => ({ default: WebhookDeliveriesPage })));
const OrgHooksPage = lazy(() => import("./pages/OrgHooksPage.js").then(({ OrgHooksPage }) => ({ default: OrgHooksPage })));
const SearchPage = lazy(() => import("./pages/SearchPage.js").then(({ SearchPage }) => ({ default: SearchPage })));
const AccountPage = lazy(() => import("./pages/AccountPage.js").then(({ AccountPage }) => ({ default: AccountPage })));
const RepoSocialPage = lazy(() => import("./pages/RepoSocialPage.js").then(({ RepoSocialPage }) => ({ default: RepoSocialPage })));
const DashboardPage = lazy(() => import("./pages/DashboardPage.js").then(({ DashboardPage }) => ({ default: DashboardPage })));
const ProfilePage = lazy(() => import("./pages/ProfilePage.js").then(({ ProfilePage }) => ({ default: ProfilePage })));
const OrgOverviewPage = lazy(() => import("./pages/OrgOverviewPage.js").then(({ OrgOverviewPage }) => ({ default: OrgOverviewPage })));
const OrgPeoplePage = lazy(() => import("./pages/OrgPeoplePage.js").then(({ OrgPeoplePage }) => ({ default: OrgPeoplePage })));
const OrgTeamsPage = lazy(() => import("./pages/OrgTeamsPage.js").then(({ OrgTeamsPage }) => ({ default: OrgTeamsPage })));

export function App() {
  if (!isLoggedIn()) {
    return (
      <ErrorBoundary>
        <BrowserRouter>
          <Suspense fallback={null}>
            <Routes>
              <Route path="/ui/login" element={<LoginPage />} />
              <Route path="/ui/*" element={<Navigate to="/ui/login" replace />} />
            </Routes>
          </Suspense>
        </BrowserRouter>
      </ErrorBoundary>
    );
  }

  return (
    <ErrorBoundary>
      <ToastProvider>
        <BrowserRouter>
          <BleephubShell>
            <Suspense fallback={null}>
              <Routes>
                <Route path="/ui/" element={<DashboardPage />} />
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
              <Route path="/ui/admin" element={<OverviewPage />} />
              <Route path="/ui/admin/users" element={<UsersPage />} />
              <Route path="/ui/admin/orgs" element={<OrgsPage />} />
              <Route path="/ui/admin/teams" element={<TeamsPage />} />
              <Route path="/ui/admin/enterprise" element={<EnterprisePage />} />
              <Route path="/ui/admin/audit-log" element={<AuditLogPage />} />
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
              {/* Bleephub organization overview, people, teams, and user profile pages.
                  Org sub-pages share the OrgHeader tab bar. The bare
                  top-level /ui/:login profile route is registered LAST so
                  every literal /ui/<page> route wins over it. */}
              <Route path="/ui/orgs/:org" element={<OrgOverviewPage />} />
              <Route path="/ui/orgs/:org/people" element={<OrgPeoplePage />} />
              <Route path="/ui/orgs/:org/teams" element={<OrgTeamsPage />} />
              <Route path="/ui/users/:login" element={<ProfilePage />} />
              {/* A logged-in user hitting /ui/login (bookmark) or any
                  unknown /ui/* path lands back on the dashboard. */}
              <Route path="/ui/login" element={<Navigate to="/ui/" replace />} />
              <Route path="/ui/:login" element={<ProfilePage />} />
                <Route path="/ui/*" element={<Navigate to="/ui/" replace />} />
              </Routes>
            </Suspense>
          </BleephubShell>
        </BrowserRouter>
      </ToastProvider>
    </ErrorBoundary>
  );
}
