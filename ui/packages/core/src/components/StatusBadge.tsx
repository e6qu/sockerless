const colorMap: Record<string, string> = {
  running: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  created: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  exited: "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300",
  ok: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  error: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
};

export interface StatusBadgeProps {
  status: string;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const colors = colorMap[status] ?? "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300";
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colors}`}>
      {status}
    </span>
  );
}
