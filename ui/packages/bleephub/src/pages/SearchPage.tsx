import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoDetail,
  isNotFound,
  searchCode,
  searchCommits,
  searchIssues,
  searchLabels,
  searchRepositories,
  searchTopics,
  searchUsers,
  SEARCH_PER_PAGE,
  type SearchResultPage,
} from "../api.js";
import type {
  BleephubRepo,
  GithubSearchCodeItem,
  GithubSearchCommitItem,
  GithubSearchIssueItem,
  GithubSearchLabelItem,
  GithubSearchTopicItem,
  GithubSearchUserItem,
} from "../types.js";
import { PageTitle, Box, Blankslate, Button, Tabs, StateLabel } from "../components/ui.js";
import { SearchIcon } from "../components/octicons.js";

type SearchTab = "repositories" | "code" | "issues" | "users" | "commits" | "labels" | "topics";

const TABS: { key: SearchTab; label: string }[] = [
  { key: "repositories", label: "Repositories" },
  { key: "code", label: "Code" },
  { key: "issues", label: "Issues & PRs" },
  { key: "users", label: "Users" },
  { key: "commits", label: "Commits" },
  { key: "labels", label: "Labels" },
  { key: "topics", label: "Topics" },
];

export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const tab = (TABS.some((t) => t.key === params.get("type"))
    ? params.get("type")
    : "repositories") as SearchTab;
  const page = Math.max(1, parseInt(params.get("page") ?? "1", 10) || 1);
  const labelsRepo = params.get("repo") ?? "";
  const [draft, setDraft] = useState(q);
  const [labelsRepoDraft, setLabelsRepoDraft] = useState(labelsRepo);

  const update = (next: Record<string, string>) => {
    const merged = new URLSearchParams(params);
    for (const [k, v] of Object.entries(next)) {
      if (v) merged.set(k, v);
      else merged.delete(k);
    }
    setParams(merged);
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    update({ q: draft.trim(), repo: labelsRepoDraft.trim(), page: "" });
  };

  return (
    <div>
      <PageTitle title="Search" />
      <form onSubmit={submit} className="mb-1 flex flex-wrap items-center gap-2">
        <input
          type="search"
          aria-label="Search query"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Search bleephub…"
          style={{ fontSize: "0.9rem", padding: "0.45rem 0.6rem", minWidth: "20rem", flex: 1 }}
        />
        {tab === "labels" && (
          <input
            type="text"
            aria-label="Repository for label search"
            value={labelsRepoDraft}
            onChange={(e) => setLabelsRepoDraft(e.target.value)}
            placeholder="owner/repo (required for labels)"
            style={{ fontSize: "0.9rem", padding: "0.45rem 0.6rem", minWidth: "14rem" }}
          />
        )}
        <Button type="submit" variant="primary">
          <span className="inline-flex items-center gap-1.5">
            <SearchIcon size={14} /> Search
          </span>
        </Button>
      </form>
      <p className="mb-4" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
        Qualifiers: <code>repo:owner/name</code> <code>user:login</code> <code>org:name</code>{" "}
        <code>language:go</code> <code>label:bug</code> <code>state:open</code> <code>is:pr</code>{" "}
        <code>is:issue</code> <code>in:title</code> <code>path:dir</code> <code>extension:go</code>{" "}
        <code>filename:main.go</code> <code>author:login</code> <code>hash:sha</code> — quote
        multi-word terms.
      </p>
      <Tabs
        items={TABS}
        active={tab}
        onChange={(next) => update({ type: next, page: "" })}
      />
      {!q.trim() ? (
        <Blankslate
          icon={<SearchIcon size={26} />}
          title="Search bleephub"
        >
          <p>Type a query above to search {TABS.find((t) => t.key === tab)?.label.toLowerCase()}.</p>
        </Blankslate>
      ) : (
        <SearchResults
          tab={tab}
          q={q}
          page={page}
          labelsRepo={labelsRepo}
          onPage={(p) => update({ page: String(p) })}
        />
      )}
    </div>
  );
}

function SearchResults({
  tab,
  q,
  page,
  labelsRepo,
  onPage,
}: {
  tab: SearchTab;
  q: string;
  page: number;
  labelsRepo: string;
  onPage: (page: number) => void;
}) {
  switch (tab) {
    case "repositories":
      return (
        <ResultList
          queryKey={["search", "repositories", q, page]}
          queryFn={() => searchRepositories(q, page)}
          page={page}
          onPage={onPage}
          noun={{ singular: "repository", plural: "repositories" }}
          render={(r: BleephubRepo) => (
            <div>
              <Link
                to={`/ui/repos/${r.full_name}`}
                style={{ color: "var(--color-accent)", fontWeight: 600, textDecoration: "none" }}
              >
                {r.full_name}
              </Link>
              <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
                {r.description || "No description."} · {r.visibility}
              </div>
            </div>
          )}
        />
      );
    case "code":
      return (
        <ResultList
          queryKey={["search", "code", q, page]}
          queryFn={() => searchCode(q, page)}
          page={page}
          onPage={onPage}
          noun={{ singular: "code result", plural: "code results" }}
          render={(item: GithubSearchCodeItem) => (
            <div>
              <span style={{ fontWeight: 600 }}>{item.repository.full_name}</span>
              <span className="font-mono" style={{ marginLeft: "0.5rem", fontSize: "0.82rem" }}>
                {item.path}
              </span>
              <span style={{ marginLeft: "0.5rem", fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                {item.language ?? "unknown language"}
              </span>
            </div>
          )}
        />
      );
    case "issues":
      return (
        <ResultList
          queryKey={["search", "issues", q, page]}
          queryFn={() => searchIssues(q, page)}
          page={page}
          onPage={onPage}
          noun={{ singular: "issue or pull request", plural: "issues and pull requests" }}
          render={(item: GithubSearchIssueItem) => (
            <div className="flex items-center gap-2">
              <StateLabel state={item.state === "open" ? "open" : "closed"}>{item.state}</StateLabel>
              <div className="min-w-0 flex-1">
                <Link
                  to={`/ui/repos/${item.repository.full_name}/${item.pull_request ? "pulls" : "issues"}/${item.number}`}
                  style={{ color: "var(--color-fg)", fontWeight: 600, textDecoration: "none" }}
                >
                  {item.title}
                </Link>
                <div style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                  {item.pull_request ? "pull request" : "issue"} · {item.repository.full_name}#
                  {item.number} · {item.user?.login ?? "ghost"} · {item.comments} comments
                </div>
              </div>
            </div>
          )}
        />
      );
    case "users":
      return (
        <ResultList
          queryKey={["search", "users", q, page]}
          queryFn={() => searchUsers(q, page)}
          page={page}
          onPage={onPage}
          noun={{ singular: "user", plural: "users" }}
          render={(u: GithubSearchUserItem) => (
            <div>
              <span style={{ fontWeight: 600 }}>{u.login}</span>
              <span style={{ marginLeft: "0.5rem", fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                {u.type}
                {u.name ? ` · ${u.name}` : ""}
                {u.bio ? ` · ${u.bio}` : ""}
              </span>
            </div>
          )}
        />
      );
    case "commits":
      return (
        <ResultList
          queryKey={["search", "commits", q, page]}
          queryFn={() => searchCommits(q, page)}
          page={page}
          onPage={onPage}
          noun={{ singular: "commit", plural: "commits" }}
          render={(c: GithubSearchCommitItem) => (
            <div>
              <div style={{ fontWeight: 600, fontSize: "0.88rem" }}>
                {c.commit.message.split("\n")[0]}
              </div>
              <div style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                {c.repository.full_name} ·{" "}
                <span className="font-mono">{c.sha.slice(0, 7)}</span> ·{" "}
                {c.author?.login ?? c.commit.author.name} ·{" "}
                {new Date(c.commit.author.date).toLocaleDateString()}
              </div>
            </div>
          )}
        />
      );
    case "labels":
      return <LabelResults q={q} page={page} labelsRepo={labelsRepo} onPage={onPage} />;
    case "topics":
      return (
        <ResultList
          queryKey={["search", "topics", q, page]}
          queryFn={() => searchTopics(q, page)}
          page={page}
          onPage={onPage}
          noun={{ singular: "topic", plural: "topics" }}
          render={(t: GithubSearchTopicItem) => (
            <div>
              <span style={{ fontWeight: 600 }}>{t.name}</span>
              <span style={{ marginLeft: "0.5rem", fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                {t.repository_count} {t.repository_count === 1 ? "repository" : "repositories"}
              </span>
            </div>
          )}
        />
      );
  }
}

/** Label search requires a repository_id; resolve the typed owner/repo first. */
function LabelResults({
  q,
  page,
  labelsRepo,
  onPage,
}: {
  q: string;
  page: number;
  labelsRepo: string;
  onPage: (page: number) => void;
}) {
  const [owner = "", repo = ""] = labelsRepo.split("/");
  const valid = !!owner && !!repo && labelsRepo.split("/").length === 2;
  const repoQuery = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => fetchRepoDetail(owner, repo),
    enabled: valid,
  });

  if (!valid) {
    return (
      <Blankslate title="Pick a repository">
        <p>Label search is scoped to one repository — enter it as owner/repo above.</p>
      </Blankslate>
    );
  }
  if (repoQuery.isLoading) return <Spinner label={`resolving ${labelsRepo}`} />;
  if (repoQuery.isError || !repoQuery.data) {
    return isNotFound(repoQuery.error) ? (
      <Blankslate title={`Repository ${labelsRepo} not found`} />
    ) : (
      <InlineError title={`Failed to resolve ${labelsRepo}`} detail={String(repoQuery.error)} />
    );
  }

  const repositoryId = repoQuery.data.id;
  return (
    <ResultList
      queryKey={["search", "labels", q, repositoryId, page]}
      queryFn={() => searchLabels(q, repositoryId, page)}
      page={page}
      onPage={onPage}
      noun={{ singular: "label", plural: "labels" }}
      render={(l: GithubSearchLabelItem) => (
        <div className="flex items-center gap-2">
          <span
            aria-hidden
            style={{
              width: 12,
              height: 12,
              borderRadius: "999px",
              background: `#${l.color}`,
              border: "1px solid var(--color-border)",
              flexShrink: 0,
            }}
          />
          <span style={{ fontWeight: 600 }}>{l.name}</span>
          <span style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
            {l.description || (l.default ? "default label" : "")}
          </span>
        </div>
      )}
    />
  );
}

function ResultList<T>({
  queryKey,
  queryFn,
  page,
  onPage,
  noun,
  render,
}: {
  queryKey: (string | number)[];
  queryFn: () => Promise<SearchResultPage<T>>;
  page: number;
  onPage: (page: number) => void;
  noun: { singular: string; plural: string };
  render: (item: T) => React.ReactNode;
}) {
  const { data, isLoading, isError, error } = useQuery({ queryKey, queryFn });

  if (isLoading) return <Spinner label={`searching ${noun.plural}`} />;
  if (isError || !data)
    return <InlineError title={`Search failed`} detail={String(error)} />;
  if (data.totalCount === 0)
    return (
      <Blankslate icon={<SearchIcon size={26} />} title={`No matching ${noun.plural}`}>
        <p>Try different terms or qualifiers.</p>
      </Blankslate>
    );

  const lastPage = Math.max(1, Math.ceil(data.totalCount / SEARCH_PER_PAGE));
  return (
    <div>
      <div className="mb-2" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
        {data.totalCount} {data.totalCount === 1 ? noun.singular : noun.plural}
        {data.incompleteResults ? " (incomplete)" : ""}
      </div>
      <Box>
        {data.items.map((item, i) => (
          <div
            key={i}
            style={{
              padding: "0.6rem 1rem",
              fontSize: "0.88rem",
              borderBottom: i < data.items.length - 1 ? "1px solid var(--color-border)" : "none",
            }}
          >
            {render(item)}
          </div>
        ))}
      </Box>
      {lastPage > 1 && (
        <div className="mt-3 flex items-center justify-center gap-2">
          <Button size="sm" variant="secondary" disabled={page <= 1} onClick={() => onPage(page - 1)}>
            Previous
          </Button>
          <span style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
            Page {page} of {lastPage}
          </span>
          <Button
            size="sm"
            variant="secondary"
            disabled={page >= lastPage}
            onClick={() => onPage(page + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
