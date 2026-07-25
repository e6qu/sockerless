import { useLocation } from "react-router";
import { AzureEssentials, AzureNotSupportedBadge } from "../portal/index.js";
import { AzureCommandBar } from "../portal/AzurePortal.js";
import { catalogItemForPath } from "../catalog.js";

// The destination every "Not supported" service-menu item links to: a small,
// honest page rather than a dead end. It states plainly that the Sockerless
// simulator does not implement the service, so an operator who followed the
// real Azure portal's information architecture here learns why the resource
// list is missing rather than hitting a blank pane or a broken link.
export function NotSupportedPage() {
  const { pathname } = useLocation();
  const item = catalogItemForPath(pathname);
  const label = item?.label ?? "This service";

  return (
    <>
      <AzureCommandBar commands={[{ label: "Feedback", icon: "feedback" }]} />
      <div className="az-main">
        <AzureEssentials
          properties={[
            { label: "Service", value: label },
            { label: "Resource type", value: item?.kind ?? "—" },
            { label: "Status", value: <AzureNotSupportedBadge /> },
          ]}
        />
        <div className="az-message az-message-warning" role="status" data-testid="not-supported-message">
          <strong>{label} is not implemented by the Sockerless simulator.</strong>
          <p>
            This entry stays in the service menu because the real Azure portal offers it, so the menu reads as
            Azure&rsquo;s own catalog. Sockerless implements the resources it stands services up on — Container Apps,
            Function Apps, Container Registry, Storage accounts, Log Analytics, and Microsoft Entra ID app
            registrations — and does not simulate {label.toLowerCase()}.
          </p>
        </div>
      </div>
    </>
  );
}
