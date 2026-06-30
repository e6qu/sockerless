import { useParams } from "react-router";
import { InlineError } from "@sockerless/ui-core/components";
import { fetchOrgReposPage } from "../api.js";
import { RepoListPage } from "./RepoListPage.js";

export function OrgReposPage() {
  const { org } = useParams<{ org: string }>();
  if (!org) {
    return <InlineError title="Missing organization" detail="No organization login provided." />;
  }

  return (
    <RepoListPage
      title={`${org} repositories`}
      fetchPage={(filters, pageUrl) => fetchOrgReposPage(org, filters, pageUrl)}
      queryKey={["org-repos", org]}
      allowCreate
      createTarget={{ org }}
    />
  );
}
