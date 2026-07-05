import { useParams } from "react-router";
import { InlineError } from "@sockerless/ui-core/components";
import { fetchOrgReposPage } from "../api.js";
import { RepoListPage } from "./RepoListPage.js";
import { OrgHeader } from "../components/Shell.js";

export function OrgReposPage() {
  const { org } = useParams<{ org: string }>();
  if (!org) {
    return <InlineError title="Missing organization" detail="No organization login provided." />;
  }

  return (
    <div>
      <OrgHeader org={org} active="repos" />
      <RepoListPage
        title="Repositories"
        fetchPage={(filters, pageUrl) => fetchOrgReposPage(org, filters, pageUrl)}
        queryKey={["org-repos", org]}
        allowCreate
        createTarget={{ org }}
      />
    </div>
  );
}
