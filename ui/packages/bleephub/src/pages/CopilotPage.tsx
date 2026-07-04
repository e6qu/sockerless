import { useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchCopilotBilling,
  fetchCopilotSeats,
  addCopilotSeats,
  cancelCopilotSeats,
  fetchCopilotSpaces,
} from "../api.js";
import { OrgHeader } from "../components/Shell.js";
import { Box, Button, ErrorBanner, SectionLabel, StatCard } from "../components/ui.js";
import { CommentIcon } from "../components/octicons.js";
import { Blankslate } from "../components/ui.js";

export function CopilotPage() {
  const { org = "" } = useParams<{ org: string }>();

  return (
    <div>
      <OrgHeader org={org} active="copilot" />
      <div className="flex flex-col gap-6">
        <BillingSection org={org} />
        <SeatsSection org={org} />
        <SpacesSection org={org} />
      </div>
    </div>
  );
}

function BillingSection({ org }: { org: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["copilot-billing", org],
    queryFn: () => fetchCopilotBilling(org),
  });

  return (
    <section>
      <SectionLabel>Billing</SectionLabel>
      {isLoading && <Spinner label="loading Copilot billing" />}
      {isError && <InlineError title="Failed to load Copilot billing" detail={String(error)} />}
      {data && (
        <>
          <div className="mb-3 grid gap-3 sm:grid-cols-4">
            <StatCard title="Total seats" value={data.seat_breakdown.total} emphasized />
            <StatCard title="Added this cycle" value={data.seat_breakdown.added_this_cycle} />
            <StatCard title="Pending invitation" value={data.seat_breakdown.pending_invitation} />
            <StatCard title="Pending cancellation" value={data.seat_breakdown.pending_cancellation} />
          </div>
          <Box>
            <div className="flex flex-wrap gap-x-6 gap-y-1" style={{ padding: "0.75rem 1rem", fontSize: "0.85rem" }}>
              <span>
                Plan: <strong>{data.plan_type}</strong>
              </span>
              <span>
                Seat management: <strong>{data.seat_management_setting}</strong>
              </span>
              <span>
                Public code suggestions: <strong>{data.public_code_suggestions}</strong>
              </span>
              <span>
                IDE chat: <strong>{data.ide_chat}</strong>
              </span>
              <span>
                Platform chat: <strong>{data.platform_chat}</strong>
              </span>
              <span>
                CLI: <strong>{data.cli}</strong>
              </span>
            </div>
          </Box>
        </>
      )}
    </section>
  );
}

function SeatsSection({ org }: { org: string }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [usernames, setUsernames] = useState("");

  const { data, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["copilot-seats", org],
    queryFn: () => fetchCopilotSeats(org),
  });

  const invalidate = () => {
    setError(null);
    qc.invalidateQueries({ queryKey: ["copilot-seats", org] });
    qc.invalidateQueries({ queryKey: ["copilot-billing", org] });
  };
  const addMut = useMutation({
    mutationFn: () =>
      addCopilotSeats(
        org,
        usernames
          .split(",")
          .map((u) => u.trim())
          .filter(Boolean),
      ),
    onSuccess: () => {
      invalidate();
      setUsernames("");
    },
    onError: (err: Error) => setError(err.message),
  });
  const removeMut = useMutation({
    mutationFn: (login: string) => cancelCopilotSeats(org, [login]),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  return (
    <section>
      <SectionLabel>Seats</SectionLabel>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box>
        <div className="flex gap-2" style={{ padding: "0.75rem 1rem", borderBottom: "1px solid var(--color-border)" }}>
          <input
            aria-label="Usernames to assign Copilot seats"
            placeholder="usernames, comma-separated"
            value={usernames}
            onChange={(e) => setUsernames(e.target.value)}
            className="w-full"
          />
          <Button
            variant="primary"
            size="sm"
            disabled={addMut.isPending || !usernames.trim()}
            onClick={() => {
              setError(null);
              addMut.mutate();
            }}
          >
            Assign seats
          </Button>
        </div>
        {isLoading && <Spinner label="loading Copilot seats" />}
        {isError && <InlineError title="Failed to load Copilot seats" detail={String(loadErr)} />}
        {data &&
          (data.seats.length === 0 ? (
            <div style={{ padding: "0.9rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No Copilot seats assigned.
            </div>
          ) : (
            data.seats.map((seat, i) => {
              const login = seat.assignee?.login;
              return (
              <div
                key={login ?? i}
                className="flex flex-wrap items-center justify-between gap-3"
                style={{
                  padding: "0.6rem 1rem",
                  borderBottom: i < data.seats.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span style={{ fontWeight: 500 }}>
                  {login ? `@${login}` : "(unresolved assignee)"}
                </span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                  {seat.plan_type} · since {new Date(seat.created_at).toLocaleDateString()}
                  {seat.assigning_team && ` · via team @${seat.assigning_team.slug}`}
                  {seat.pending_cancellation_date && ` · cancels ${seat.pending_cancellation_date}`}
                </span>
                {login && (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={removeMut.isPending}
                    onClick={() => {
                      if (confirm(`Cancel ${login}'s Copilot seat?`)) {
                        removeMut.mutate(login);
                      }
                    }}
                  >
                    cancel seat
                  </Button>
                )}
              </div>
              );
            })
          ))}
      </Box>
    </section>
  );
}

function SpacesSection({ org }: { org: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["copilot-spaces", org],
    queryFn: () => fetchCopilotSpaces(org),
  });

  return (
    <section>
      <SectionLabel>Copilot Spaces</SectionLabel>
      {isLoading && <Spinner label="loading Copilot Spaces" />}
      {isError && <InlineError title="Failed to load Copilot Spaces" detail={String(error)} />}
      {data &&
        (data.length === 0 ? (
          <Blankslate icon={<CommentIcon size={26} />} title="No Copilot Spaces">
            Spaces bundle repositories, files, and instructions into a shared Copilot context.
          </Blankslate>
        ) : (
          <Box>
            {data.map((space, i) => (
              <div
                key={space.id}
                className="flex flex-wrap items-center justify-between gap-3"
                style={{
                  padding: "0.6rem 1rem",
                  borderBottom: i < data.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>
                  {space.name}
                  <span style={{ color: "var(--color-fg-muted)", fontWeight: 400 }}> #{space.number}</span>
                </span>
                <span className="min-w-0 flex-1" style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                  {space.description ?? "No description"}
                </span>
                <span style={{ color: "var(--color-fg-muted)", fontSize: "0.82rem" }}>
                  base role: {space.base_role}
                  {space.creator && ` · created by @${space.creator.login}`} ·{" "}
                  {new Date(space.updated_at).toLocaleDateString()}
                </span>
              </div>
            ))}
          </Box>
        ))}
    </section>
  );
}
