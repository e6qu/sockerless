export interface MetricsCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
}

export function MetricsCard({ title, value, subtitle }: MetricsCardProps) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <p className="text-sm text-gray-500 dark:text-gray-400">{title}</p>
      <p className="mt-1 text-2xl font-semibold">{value}</p>
      {subtitle && <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">{subtitle}</p>}
    </div>
  );
}
