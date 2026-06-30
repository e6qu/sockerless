import { fetchUserReposPage } from "../api.js";
import { RepoListPage } from "./RepoListPage.js";

export function ReposPage() {
  return (
    <RepoListPage
      title="Repositories"
      fetchPage={fetchUserReposPage}
      queryKey={["user-repos"]}
      allowCreate
      createTarget="user"
    />
  );
}
