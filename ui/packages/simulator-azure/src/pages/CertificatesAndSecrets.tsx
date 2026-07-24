import { useState } from "react";
import { AzureIcon } from "../portal/icons.js";
import type { ClientSecretMetadata, MintedClientSecret } from "../api.js";

// The expiry choices the real Certificates & secrets blade offers, as days
// from now; the blade posts the resulting endDateTime to Microsoft Graph.
export const EXPIRY_OPTIONS = [
  { label: "90 days (3 months)", days: 90 },
  { label: "Recommended: 180 days (6 months)", days: 180 },
  { label: "365 days (12 months)", days: 365 },
  { label: "730 days (24 months)", days: 730 },
] as const;

export interface CertificatesAndSecretsProps {
  credentials: ClientSecretMetadata[];
  /** The secret minted in this session, shown in full exactly once. */
  minted: MintedClientSecret | null;
  busy: boolean;
  onCreate: (description: string, endDateTime: string) => void;
  onRemove: (keyId: string) => void;
}

function expiryDate(days: number): string {
  const end = new Date(Date.now() + days * 24 * 60 * 60 * 1000);
  return end.toISOString();
}

function formatDate(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleDateString();
}

// CertificatesAndSecrets is the Certificates & secrets blade of an app
// registration: it lists the client secrets' metadata (Microsoft Graph never
// returns a stored secret's value — only its hint) and mints new ones. The
// full secretText appears exactly once, from the addPassword response held in
// this session; leaving the blade discards it, matching the real portal.
export function CertificatesAndSecrets({ credentials, minted, busy, onCreate, onRemove }: CertificatesAndSecretsProps) {
  const [adding, setAdding] = useState(false);
  const [description, setDescription] = useState("");
  const [days, setDays] = useState<number>(180);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState<string | null>(null);

  return (
    <section className="az-blade-section" aria-label="Certificates & secrets">
      <h2>Certificates &amp; secrets</h2>
      <p className="az-blade-hint">
        Credentials enable confidential applications to identify themselves to the authentication service when
        receiving tokens. A client secret is a string the application uses as a password.
      </p>

      <button
        type="button"
        className="az-command"
        data-testid="entra-new-client-secret"
        disabled={busy}
        onClick={() => setAdding(true)}
      >
        <span className="az-command-glyph"><AzureIcon name="add" size={16} /></span>
        New client secret
      </button>

      {adding ? (
        <form
          className="az-form"
          data-testid="entra-secret-form"
          onSubmit={(event) => {
            event.preventDefault();
            setAdding(false);
            onCreate(description.trim(), expiryDate(days));
            setDescription("");
          }}
        >
          <h3>Add a client secret</h3>
          <label>
            Description
            <input
              className="az-input"
              data-testid="entra-secret-description"
              value={description}
              placeholder="Enter a description for this client secret"
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          <label>
            Expires
            <select
              className="az-input"
              data-testid="entra-secret-expiry"
              value={days}
              onChange={(event) => setDays(Number(event.target.value))}
            >
              {EXPIRY_OPTIONS.map((option) => (
                <option key={option.days} value={option.days}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
          <div className="az-form-actions">
            <button type="submit" className="az-button-primary" data-testid="entra-secret-add" disabled={busy}>
              Add
            </button>
            <button type="button" className="az-button" onClick={() => setAdding(false)}>
              Cancel
            </button>
          </div>
        </form>
      ) : null}

      {minted ? (
        <div className="az-message az-message-warning" role="alert" data-testid="entra-secret-notice">
          Remember to copy the new client secret value. You won&rsquo;t be able to retrieve it after you leave this
          blade.
        </div>
      ) : null}
      {copyError ? (
        <div className="az-message az-message-error" role="alert">
          <strong>Could not copy the secret to the clipboard.</strong> {copyError}
        </div>
      ) : null}

      <table className="az-table" data-testid="entra-secrets-table">
        <thead>
          <tr>
            <th>Description</th>
            <th>Expires</th>
            <th>Value</th>
            <th>Secret ID</th>
            <th aria-label="Actions" />
          </tr>
        </thead>
        <tbody>
          {credentials.length === 0 ? (
            <tr>
              <td className="az-table-state" colSpan={5}>
                <div className="az-empty">
                  <strong>No client secrets have been created for this application.</strong>
                </div>
              </td>
            </tr>
          ) : (
            credentials.map((credential) => {
              const isMinted = minted !== null && credential.keyId === minted.keyId;
              return (
                <tr key={credential.keyId} data-testid="entra-secret-row">
                  <td>{credential.displayName || "—"}</td>
                  <td>{formatDate(credential.endDateTime)}</td>
                  <td>
                    {isMinted ? (
                      <span className="az-secret-value">
                        <code data-testid="entra-secret-value">{minted.secretText}</code>
                        <button
                          type="button"
                          className="az-icon-button az-copy"
                          aria-label="Copy the client secret value"
                          data-testid="entra-secret-copy"
                          onClick={() => {
                            navigator.clipboard.writeText(minted.secretText).then(
                              () => setCopied(true),
                              (err: unknown) => setCopyError(err instanceof Error ? err.message : String(err)),
                            );
                          }}
                        >
                          <AzureIcon name="copy" size={16} />
                        </button>
                        {copied ? <span role="status">Copied</span> : null}
                      </span>
                    ) : (
                      <code data-testid="entra-secret-hint">{credential.hint ? `${credential.hint}${"*".repeat(31)}` : "Hidden"}</code>
                    )}
                  </td>
                  <td>
                    <code>{credential.keyId}</code>
                  </td>
                  <td>
                    <button
                      type="button"
                      className="az-icon-button"
                      aria-label={`Delete the client secret ${credential.displayName || credential.keyId}`}
                      data-testid="entra-secret-delete"
                      disabled={busy}
                      onClick={() => onRemove(credential.keyId)}
                    >
                      <AzureIcon name="delete" size={16} />
                    </button>
                  </td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </section>
  );
}
