import { useState } from "react";
import type { ConfigFileInfo } from "../../types/config";
import { FieldLabel, Select, TextInput } from "./SectionCard";

export function FilePicker({
  files,
  selectedPath,
  onSelect,
  newFileMode,
  onToggleNew,
  newFileName,
  onNewFileName,
  isDir,
}: {
  files: ConfigFileInfo[];
  selectedPath: string;
  onSelect: (path: string) => void;
  newFileMode: boolean;
  onToggleNew: (b: boolean) => void;
  newFileName: string;
  onNewFileName: (s: string) => void;
  isDir: boolean;
}) {
  const [containingExpanded, setContainingExpanded] = useState(false);

  if (!isDir) {
    const f = files[0];
    return (
      <div className="flex items-center gap-2 text-xs text-zinc-400">
        <span className="text-zinc-500">Save to</span>
        <span className="text-zinc-200 font-mono">{f?.name ?? "—"}</span>
        <span className="text-zinc-600">({f?.format})</span>
      </div>
    );
  }

  const containing = files.filter((f) => f.containsRoute);

  return (
    <div className="flex flex-col gap-2 text-xs">
      <div className="flex items-center gap-2">
        <FieldLabel>Save to file</FieldLabel>
        {containing.length > 0 && (
          <button
            type="button"
            onClick={() => setContainingExpanded((b) => !b)}
            className="text-zinc-500 hover:text-zinc-300 cursor-pointer"
          >
            current: <span className="text-emerald-400 font-mono">{containing[0].name}</span>
          </button>
        )}
      </div>

      {!newFileMode ? (
        <div className="flex items-center gap-2">
          <Select
            value={selectedPath}
            onChange={onSelect}
            options={files.map((f) => ({
              value: f.path,
              label: `${f.name}  (${f.routeCount + f.grpcRouteCount} routes)`,
            }))}
            emptyLabel={files.length === 0 ? "(no files yet)" : undefined}
            className="!w-72"
          />
          <button
            type="button"
            onClick={() => onToggleNew(true)}
            className="text-xs px-2 py-1 rounded border border-zinc-700 text-zinc-300 hover:bg-zinc-800 cursor-pointer"
          >
            + New file
          </button>
        </div>
      ) : (
        <div className="flex items-center gap-2">
          <TextInput
            value={newFileName}
            onChange={onNewFileName}
            placeholder="continents.toml"
            className="!w-72"
          />
          <button
            type="button"
            onClick={() => onToggleNew(false)}
            className="text-xs px-2 py-1 rounded border border-zinc-700 text-zinc-300 hover:bg-zinc-800 cursor-pointer"
          >
            Cancel
          </button>
        </div>
      )}

      {containingExpanded && containing.length > 0 && (
        <p className="text-zinc-500">
          This route currently lives in{" "}
          <span className="text-zinc-300 font-mono">{containing[0].path}</span>.
        </p>
      )}
    </div>
  );
}
