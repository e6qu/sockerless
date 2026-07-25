/**
 * The AWS service catalog the console's side navigation renders.
 *
 * The real AWS Management Console groups its "All services" menu the way AWS
 * itself categorizes its services — Compute, Containers, Storage, Database,
 * Networking & Content Delivery, and so on — and lists every service AWS
 * offers, not only the ones a given account uses. This simulator follows the
 * same shape and is honest about the gap between the two: a service
 * sockerless implements links to its real page; every other service AWS
 * offers is still listed, under its real category, but links to a page that
 * says plainly that sockerless has not implemented it. Hiding the
 * unimplemented services would make the simulator's coverage look broader
 * than it is; leaving AWS's other categories out entirely would make the
 * navigation read as sockerless's own invention rather than AWS's console.
 */

export interface NavService {
  /** The service's real, fully-qualified AWS name — the link's visible and
   * accessible text. */
  label: string;
  to: string;
  /** Services sockerless has built a page for. Defaults to true; every entry
   * that sockerless has not implemented links to the honest "not supported"
   * page instead of a real console page, and its link renders the
   * "Not supported" pill. */
  supported?: boolean;
}

export interface NavGroup {
  /** The category AWS's own "All services" menu groups this service under. */
  label: string;
  items: NavService[];
}

const NOT_SUPPORTED = "/ui/not-supported/";

export const NAV_GROUPS: NavGroup[] = [
  { label: "Dashboard", items: [{ label: "Overview", to: "/ui/" }] },
  {
    label: "Compute",
    items: [
      { label: "EC2", to: NOT_SUPPORTED + "ec2", supported: false },
      { label: "Lambda", to: "/ui/lambda" },
      { label: "Elastic Beanstalk", to: NOT_SUPPORTED + "elastic-beanstalk", supported: false },
      { label: "Lightsail", to: NOT_SUPPORTED + "lightsail", supported: false },
      { label: "AWS Batch", to: NOT_SUPPORTED + "batch", supported: false },
    ],
  },
  {
    label: "Containers",
    items: [
      { label: "Elastic Container Service", to: "/ui/ecs" },
      { label: "Elastic Kubernetes Service", to: NOT_SUPPORTED + "eks", supported: false },
      { label: "Elastic Container Registry", to: "/ui/ecr" },
    ],
  },
  {
    label: "Storage",
    items: [
      { label: "Simple Storage Service", to: "/ui/s3" },
      { label: "Elastic File System", to: NOT_SUPPORTED + "efs", supported: false },
      { label: "S3 Glacier", to: NOT_SUPPORTED + "s3-glacier", supported: false },
      { label: "AWS Backup", to: NOT_SUPPORTED + "backup", supported: false },
    ],
  },
  {
    label: "Database",
    items: [
      { label: "RDS", to: NOT_SUPPORTED + "rds", supported: false },
      { label: "DynamoDB", to: NOT_SUPPORTED + "dynamodb", supported: false },
      { label: "ElastiCache", to: NOT_SUPPORTED + "elasticache", supported: false },
    ],
  },
  {
    label: "Networking & content delivery",
    items: [
      { label: "VPC", to: NOT_SUPPORTED + "vpc", supported: false },
      { label: "CloudFront", to: NOT_SUPPORTED + "cloudfront", supported: false },
      { label: "Route 53", to: NOT_SUPPORTED + "route-53", supported: false },
      { label: "API Gateway", to: NOT_SUPPORTED + "api-gateway", supported: false },
    ],
  },
  {
    label: "Application integration",
    items: [
      { label: "Simple Notification Service", to: NOT_SUPPORTED + "sns", supported: false },
      { label: "Simple Queue Service", to: NOT_SUPPORTED + "sqs", supported: false },
    ],
  },
  {
    label: "Management & Governance",
    items: [
      { label: "CloudWatch Logs", to: "/ui/logs" },
      { label: "AWS Organizations", to: "/ui/organizations" },
      { label: "CloudFormation", to: NOT_SUPPORTED + "cloudformation", supported: false },
      { label: "Systems Manager", to: NOT_SUPPORTED + "systems-manager", supported: false },
    ],
  },
  {
    label: "Security, identity, and compliance",
    items: [
      { label: "Identity and Access Management", to: "/ui/iam" },
      { label: "Secrets Manager", to: NOT_SUPPORTED + "secrets-manager", supported: false },
      { label: "Key Management Service", to: NOT_SUPPORTED + "kms", supported: false },
      { label: "GuardDuty", to: NOT_SUPPORTED + "guardduty", supported: false },
    ],
  },
];

/** Looks up a not-supported service's real name from the slug in its route
 * (`/ui/not-supported/:service`), so the honest page and the breadcrumb trail
 * state the service AWS itself calls it, not the URL slug. */
export function findNavService(slug: string): NavService | undefined {
  const to = NOT_SUPPORTED + slug;
  for (const group of NAV_GROUPS) {
    const found = group.items.find((item) => item.to === to);
    if (found) return found;
  }
  return undefined;
}
