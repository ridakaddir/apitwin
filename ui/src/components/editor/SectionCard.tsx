import type { ReactNode } from "react";

export function SectionCard({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="border border-zinc-800 rounded-lg bg-zinc-950/40">
      <header className="flex items-start justify-between gap-4 px-4 py-3 border-b border-zinc-800">
        <div>
          <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-400">
            {title}
          </h2>
          {description && (
            <p className="text-xs text-zinc-500 mt-1">{description}</p>
          )}
        </div>
        {action}
      </header>
      <div className="p-4 space-y-3">{children}</div>
    </section>
  );
}

export function FieldLabel({ children }: { children: ReactNode }) {
  return (
    <label className="block text-xs font-medium text-zinc-400 mb-1">
      {children}
    </label>
  );
}

export function TextInput({
  value,
  onChange,
  placeholder,
  className = "",
  error,
  spellCheck = false,
  monospace = true,
  type = "text",
  step,
  min,
}: {
  value: string | number;
  onChange: (v: string) => void;
  placeholder?: string;
  className?: string;
  error?: boolean;
  spellCheck?: boolean;
  monospace?: boolean;
  type?: string;
  step?: number;
  min?: number;
}) {
  return (
    <input
      type={type}
      step={step}
      min={min}
      value={value === undefined || value === null ? "" : String(value)}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      spellCheck={spellCheck}
      className={`w-full bg-zinc-900 border rounded px-2.5 py-1.5 text-sm text-zinc-100 placeholder-zinc-600 focus:outline-none ${
        error ? "border-red-500/60" : "border-zinc-800 focus:border-zinc-600"
      } ${monospace ? "font-mono" : ""} ${className}`}
    />
  );
}

export function Select({
  value,
  onChange,
  options,
  className = "",
  emptyLabel,
  error,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label?: string }[];
  className?: string;
  emptyLabel?: string;
  error?: boolean;
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={`w-full bg-zinc-900 border rounded px-2 py-1.5 text-sm text-zinc-100 focus:outline-none ${
        error ? "border-red-500/60" : "border-zinc-800 focus:border-zinc-600"
      } ${className}`}
    >
      {emptyLabel !== undefined && <option value="">{emptyLabel}</option>}
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label ?? o.value}
        </option>
      ))}
    </select>
  );
}

export function Textarea({
  value,
  onChange,
  placeholder,
  rows = 3,
  error,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
  error?: boolean;
}) {
  return (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      spellCheck={false}
      rows={rows}
      className={`w-full bg-zinc-900 border rounded px-2.5 py-1.5 text-sm text-zinc-100 placeholder-zinc-600 focus:outline-none font-mono resize-y ${
        error ? "border-red-500/60" : "border-zinc-800 focus:border-zinc-600"
      }`}
    />
  );
}

export function ErrorText({ children }: { children: ReactNode }) {
  if (!children) return null;
  return <p className="text-xs text-red-400 mt-1">{children}</p>;
}

export function GhostButton({
  onClick,
  children,
  danger,
  disabled,
  title,
}: {
  onClick: () => void;
  children: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`text-xs px-2 py-1 rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer ${
        danger
          ? "border-red-500/30 text-red-400 hover:bg-red-500/10"
          : "border-zinc-700 text-zinc-300 hover:bg-zinc-800"
      }`}
    >
      {children}
    </button>
  );
}

export function PrimaryButton({
  onClick,
  children,
  disabled,
  title,
}: {
  onClick: () => void;
  children: ReactNode;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="text-sm px-3 py-1.5 rounded bg-emerald-500/20 border border-emerald-500/40 text-emerald-300 hover:bg-emerald-500/30 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
    >
      {children}
    </button>
  );
}
