import type { AdminRangeKey } from "../../types/api";

interface Props {
  value: AdminRangeKey;
  onChange: (next: AdminRangeKey) => void;
  disabled?: boolean;
}

const OPTIONS: Array<{ key: AdminRangeKey; label: string }> = [
  { key: "7d", label: "7天" },
  { key: "30d", label: "30天" },
];

export default function RangeToggle({ value, onChange, disabled }: Props) {
  return (
    <div className="admin-range-toggle" role="tablist" aria-label="时间范围">
      {OPTIONS.map(opt => (
        <button
          key={opt.key}
          type="button"
          role="tab"
          aria-selected={value === opt.key}
          className={value === opt.key ? "active" : ""}
          disabled={disabled}
          onClick={() => onChange(opt.key)}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
