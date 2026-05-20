export function PreviewPane({
  text,
  format,
  onFormatChange,
  fileFormat,
}: {
  text: string;
  format: "toml" | "yaml" | "json";
  onFormatChange: (f: "toml" | "yaml" | "json") => void;
  fileFormat?: string;
}) {
  return (
    <div className="border border-zinc-800 rounded-lg bg-zinc-950/40 flex flex-col h-full">
      <header className="flex items-center justify-between px-4 py-3 border-b border-zinc-800">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-400">
          Preview
        </h2>
        <div className="flex items-center gap-1 text-xs">
          {(["toml", "yaml", "json"] as const).map((f) => (
            <button
              key={f}
              type="button"
              onClick={() => onFormatChange(f)}
              className={`px-2 py-1 rounded cursor-pointer transition-colors ${
                format === f
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-500 hover:text-zinc-300"
              }`}
            >
              {f}
            </button>
          ))}
        </div>
      </header>
      {fileFormat && fileFormat !== format && (
        <p className="px-4 py-2 text-xs text-zinc-500 border-b border-zinc-800">
          Save will use <span className="text-zinc-300 font-mono">{fileFormat}</span>{" "}
          (the file's native format).
        </p>
      )}
      <pre className="flex-1 overflow-auto p-4 text-xs font-mono text-zinc-300 whitespace-pre">
        {text || <span className="text-zinc-600">Generating preview…</span>}
      </pre>
    </div>
  );
}
