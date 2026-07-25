import { Link, useParams } from "react-router";
import { GcpPageHeader } from "../console/GcpConsole.js";
import { findCatalogItem } from "../console/catalog.js";

// The destination for every "Not supported" catalog entry: a short, honest
// page rather than a dead link or a faked resource list. It names what the
// simulator actually implements, so an operator who followed the real
// console's navigation to, say, Compute Engine learns why nothing loads
// instead of guessing.
export function NotSupportedPage() {
  const { service } = useParams<{ service: string }>();
  const item = findCatalogItem(service);
  const name = item?.name ?? "This product";

  return (
    <>
      <GcpPageHeader title={name} description="Not supported in this simulator." />
      <div className="gc-empty" role="status">
        <svg className="gc-empty-illustration" viewBox="0 0 160 96" role="img" aria-label="Not supported illustration">
          <path
            d="M44 74 a20 20 0 0 1 2 -40 a26 26 0 0 1 50 -6 a18 18 0 0 1 18 22 a16 16 0 0 1 -4 24 z"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeDasharray="6 6"
            strokeLinejoin="round"
          />
        </svg>
        <p className="gc-empty-headline">{name} isn't implemented by the Sockerless simulator</p>
        <p className="gc-empty-description">
          The simulator faithfully implements a slice of Google Cloud — Cloud Run, Cloud Run functions,
          Artifact Registry, Cloud Storage, IAM service accounts, Cloud Logging, and Resource Manager
          projects — rather than approximating the rest with synthetic data.
        </p>
        <div className="gc-empty-actions">
          <Link className="gc-button-primary" to="/ui/">Back to Overview</Link>
        </div>
      </div>
    </>
  );
}
