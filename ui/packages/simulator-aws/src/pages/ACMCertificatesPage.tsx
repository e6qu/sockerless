import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { deleteACMCertificate, fetchACMCertificates, type ACMCertificate } from "../api.js";

// AWS Certificate Manager — Certificates. ListCertificates and
// DeleteCertificate on the real ACM API (X-Amz-Target CertificateManager.<Op>).

const columns: AwsColumn<ACMCertificate>[] = [
  { id: "domainName", header: "Domain name", cell: (row) => row.domainName, value: (row) => row.domainName },
  { id: "status", header: "Status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "type", header: "Type", cell: (row) => row.type || "–", value: (row) => row.type },
  {
    id: "keyAlgorithm",
    header: "Key algorithm",
    cell: (row) => row.keyAlgorithm || "–",
    value: (row) => row.keyAlgorithm,
  },
  {
    id: "inUseBy",
    header: "In use",
    cell: (row) => (row.inUseBy.length > 0 ? "Yes" : "No"),
    value: (row) => (row.inUseBy.length > 0 ? "Yes" : "No"),
  },
  {
    id: "notAfter",
    header: "Expires",
    cell: (row) => formatEpoch(row.notAfter),
    value: (row) => String(row.notAfter),
  },
];

function DeleteCertificatesModal({
  certificates,
  onClose,
  clearSelection,
}: {
  certificates: ACMCertificate[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const certificate of certificates) {
        await deleteACMCertificate(certificate.certificateArn);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["acm-certificates"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={
        certificates.length === 1 ? `Delete ${certificates[0].domainName}?` : `Delete ${certificates.length} certificates?`
      }
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="acm-delete-certificate-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>A certificate still associated with another AWS resource cannot be deleted until the association is removed.</p>
      <ul>
        {certificates.map((certificate) => (
          <li key={certificate.certificateArn}>
            <code>{certificate.domainName}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <AwsErrorAlert>
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

export function ACMCertificatesPage() {
  const [deleting, setDeleting] = useState<{ certificates: ACMCertificate[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<ACMCertificate>
        title="Certificates"
        description="AWS Certificate Manager certificates in this account and Region."
        columns={columns}
        queryKey={["acm-certificates"]}
        queryFn={fetchACMCertificates}
        filterPlaceholder="Find certificates"
        emptyTitle="No certificates"
        emptyDescription="No AWS Certificate Manager certificates exist in this account and Region."
        rowKey={(row) => row.certificateArn}
        tableTestId="acm-table"
        errorTestId="acm-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="acm-delete-certificate"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ certificates: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {deleting && (
        <DeleteCertificatesModal
          certificates={deleting.certificates}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
