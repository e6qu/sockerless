import { useParams } from "react-router";
import { GcpResourceDetail, GcpStatus } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudFunction, type CloudFunction } from "../api.js";

export function CloudFunctionDetailPage() {
  const { name = "" } = useParams();
  return (
    <GcpResourceDetail<CloudFunction>
      title={shortName(name)}
      description="Cloud Run function"
      backTo="/ui/functions"
      backLabel="Cloud Run functions"
      queryKey={["cloud-function", name]}
      queryFn={() => fetchCloudFunction(name)}
      properties={(fn) => [
        { label: "State", value: <GcpStatus status={fn.state ?? "UNKNOWN"} /> },
        { label: "Environment", value: fn.environment ?? "—" },
        { label: "Runtime", value: fn.buildConfig?.runtime ?? "—" },
        { label: "Entry point", value: fn.buildConfig?.entryPoint ?? "—" },
        { label: "URL", value: fn.serviceConfig?.uri ?? "—" },
        { label: "Created", value: formatTimestamp(fn.createTime ?? "") },
        { label: "Updated", value: formatTimestamp(fn.updateTime ?? "") },
        { label: "Description", value: fn.description ?? "—" },
      ]}
    />
  );
}
