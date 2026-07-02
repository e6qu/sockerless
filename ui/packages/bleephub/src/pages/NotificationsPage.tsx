import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import {
  deleteThreadSubscription,
  fetchNotifications,
  getThreadSubscription,
  markThreadRead,
  setThreadSubscription,
} from "../api.js";
import type { GithubNotificationThread, GithubThreadSubscription } from "../types.js";
import {
  Box,
  Button,
  DialogActions,
  ErrorBanner,
  Modal,
  PageTitle,
  StateLabel,
  Tabs,
} from "../components/ui.js";
import { NotificationBellIcon } from "../components/octicons.js";

const col = createColumnHelper<GithubNotificationThread>();

export function NotificationsPage() {
  const [tab, setTab] = useState<"unread" | "all">("unread");

  return (
    <div>
      <PageTitle
        icon={<NotificationBellIcon size={20} />}
        title="Notifications"
        meta="Issue and pull request activity across repositories you can access."
      />

      <Tabs<"unread" | "all">
        items={[
          { key: "unread", label: "Unread" },
          { key: "all", label: "All" },
        ]}
        active={tab}
        onChange={setTab}
      />

      <ThreadsTable all={tab === "all"} />
    </div>
  );
}

function ThreadsTable({ all }: { all: boolean }) {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);
  const [activeThread, setActiveThread] = useState<GithubNotificationThread | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["notifications", all],
    queryFn: () => fetchNotifications(),
    refetchInterval: 10000,
  });

  const readMut = useMutation({
    mutationFn: (id: string) => markThreadRead(id),
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  const filtered = all ? data : data?.filter((t) => t.unread);

  if (isError) return <InlineError title="Failed to load notifications" />;
  if (isLoading || !data) return <Spinner label="loading notifications" />;

  const columns = [
    col.accessor("unread", {
      header: "Status",
      cell: (info) =>
        info.getValue() ? (
          <StateLabel state="open">unread</StateLabel>
        ) : (
          <StateLabel state="closed">read</StateLabel>
        ),
    }),
    col.accessor("subject", {
      header: "Subject",
      cell: (info) => {
        const subject = info.getValue();
        const href = subjectUrlToUI(subject.url);
        return href ? (
          <Link to={href} style={{ color: "var(--color-accent)", textDecoration: "none" }}>
            {subject.title}
          </Link>
        ) : (
          <span>{subject.title}</span>
        );
      },
    }),
    col.accessor("subject", {
      header: "Type",
      id: "type",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
          {info.getValue().type}
        </span>
      ),
    }),
    col.accessor("reason", {
      header: "Reason",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("repository", {
      header: "Repository",
      cell: (info) => {
        const repo = info.getValue();
        const fullName = typeof repo.full_name === "string" ? repo.full_name : "";
        return (
          <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>{fullName}</span>
        );
      },
    }),
    col.accessor("updated_at", {
      header: "Updated",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    col.display({
      id: "actions",
      header: "Actions",
      cell: (info) => {
        const thread = info.row.original;
        return (
          <div className="flex flex-wrap items-center gap-1">
            {thread.unread && (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => readMut.mutate(thread.id)}
                disabled={readMut.isPending}
              >
                Mark read
              </Button>
            )}
            <Button size="sm" variant="ghost" onClick={() => setActiveThread(thread)}>
              Subscription
            </Button>
          </div>
        );
      },
    }),
  ];

  return (
    <>
      {mutationError && <ErrorBanner>{mutationError}</ErrorBanner>}
      <DataTable
        data={filtered ?? []}
        columns={columns}
        filterPlaceholder="Filter notifications…"
        emptyMessage={all ? "No notifications." : "No unread notifications."}
      />
      {activeThread && (
        <SubscriptionDialog thread={activeThread} onClose={() => setActiveThread(null)} />
      )}
    </>
  );
}

function subjectUrlToUI(url: string): string | null {
  const m = url.match(/\/api\/v3\/repos\/([^/]+)\/([^/]+)\/(issues|pulls)\/(\d+)$/);
  if (!m) return null;
  const [, owner, repo, kind, number] = m;
  const path = kind === "pulls" ? "pulls" : "issues";
  return `/ui/repos/${owner}/${repo}/${path}/${number}`;
}

function SubscriptionDialog({
  thread,
  onClose,
}: {
  thread: GithubNotificationThread;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [mutationError, setMutationError] = useState<string | null>(null);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["notifications", thread.id, "subscription"],
    queryFn: () => getThreadSubscription(thread.id),
  });

  const setMut = useMutation({
    mutationFn: async (subscribed: boolean) => {
      if (subscribed) {
        await setThreadSubscription(thread.id, true);
      } else {
        await deleteThreadSubscription(thread.id);
      }
    },
    onSuccess: () => {
      setMutationError(null);
      queryClient.invalidateQueries({ queryKey: ["notifications", thread.id, "subscription"] });
    },
    onError: (err: Error) => setMutationError(err.message),
  });

  const subscribe = () => setMut.mutate(true);
  const unsubscribe = () => setMut.mutate(false);

  return (
    <Modal title="Thread subscription" onClose={onClose}>
      <Box header={thread.subject.title} className="mb-4">
        <div style={{ padding: "1rem" }}>
          {isLoading ? (
            <Spinner label="loading subscription" />
          ) : isError ? (
            <InlineError title="Failed to load subscription" />
          ) : (
            <SubscriptionState subscription={data ?? null} />
          )}
        </div>
      </Box>

      {mutationError && <ErrorBanner>{mutationError}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} variant="ghost">
          Close
        </Button>
        <Button onClick={subscribe} disabled={setMut.isPending} variant="secondary">
          Subscribe
        </Button>
        <Button onClick={unsubscribe} disabled={setMut.isPending} variant="danger">
          Unsubscribe
        </Button>
      </DialogActions>
    </Modal>
  );
}

function SubscriptionState({ subscription }: { subscription: GithubThreadSubscription | null }) {
  if (!subscription) {
    return (
      <div style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
        No explicit subscription set for this thread.
      </div>
    );
  }
  return (
    <div style={{ fontSize: "0.85rem" }}>
      <div>
        <strong>Subscribed:</strong> {subscription.subscribed ? "yes" : "no"}
      </div>
      <div>
        <strong>Ignored:</strong> {subscription.ignored ? "yes" : "no"}
      </div>
      <div style={{ color: "var(--color-fg-muted)" }}>Reason: {subscription.reason}</div>
    </div>
  );
}
