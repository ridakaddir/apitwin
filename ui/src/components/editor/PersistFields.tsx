import { useState } from "react";
import type { Case } from "../../types/config";
import { FieldLabel, Select, TextInput } from "./SectionCard";

const MERGE_OPTIONS = [
  { value: "append", label: "append — add to array" },
  { value: "update", label: "update — modify by key" },
  { value: "delete", label: "delete — remove by key" },
  { value: "cascade", label: "cascade — multi-file" },
];

export function PersistFields({
  caseData,
  onPatch,
  isGRPC,
}: {
  caseData: Case;
  onPatch: (fn: (c: Case) => Case) => void;
  isGRPC: boolean;
}) {
  const [showAdvanced, setShowAdvanced] = useState(
    Boolean(caseData.key || caseData.array_key || caseData.defaults),
  );
  const fileMissing = !caseData.file;
  const merge = (caseData.merge || "append").toLowerCase();
  const isAppend = merge === "append";
  const dirExpected = isAppend;
  const fileWrongShape =
    !fileMissing && dirExpected && !caseData.file.endsWith("/");
  return (
    <div className="mt-3 pt-3 border-t border-zinc-800 space-y-3">
      <div className="bg-zinc-900/60 border border-zinc-800 rounded px-3 py-2 text-xs text-zinc-400">
        <span className="text-zinc-300">Persist</span> records each request to
        the stub <span className="text-zinc-300 font-mono">file</span> so later
        reads see what was written. Useful for POST/PUT/DELETE flows that need
        to feel stateful.
        {isAppend && (
          <>
            {" "}
            For a <span className="text-zinc-300">stateful collection</span>,
            point this case's{" "}
            <span className="text-zinc-300 font-mono">file</span> at a directory
            (e.g. <span className="text-zinc-300 font-mono">stubs/teams/</span>)
            and give the matching GET route the{" "}
            <span className="text-zinc-300">same directory</span> — GET
            aggregates every file POST appends.
          </>
        )}
      </div>
      {fileMissing && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded px-3 py-2 text-xs text-amber-300">
          Set a <span className="font-mono">file</span> path under{" "}
          <span className="font-mono">Body source → File on disk</span> so
          persisted records have somewhere to land
          {isAppend && " — append needs a directory path ending in /"}.
        </div>
      )}
      {fileWrongShape && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded px-3 py-2 text-xs text-amber-300">
          <span className="font-mono">merge = append</span> writes one file per
          request, so <span className="font-mono">file</span> must be a
          directory path ending in <span className="font-mono">/</span> (you
          have <span className="font-mono">{caseData.file}</span>).
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div>
          <FieldLabel>Merge strategy</FieldLabel>
          <Select
            value={caseData.merge ?? ""}
            onChange={(v) => onPatch((c) => ({ ...c, merge: v }))}
            options={MERGE_OPTIONS}
            emptyLabel="append (default)"
          />
          <p className="text-xs text-zinc-500 mt-1">
            {merge === "append" &&
              "Each request becomes a new file in the directory."}
            {merge === "update" &&
              "Shallow-merges the request into the file matched by key."}
            {merge === "delete" && "Removes the file matched by key."}
            {merge === "cascade" && "Multi-file mutation (configured in-file)."}
          </p>
        </div>
        {isGRPC && (
          <div>
            <FieldLabel>Wrap (envelope field for gRPC)</FieldLabel>
            <TextInput
              value={caseData.wrap ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, wrap: v }))}
              placeholder="continent"
            />
          </div>
        )}
        {isGRPC && (
          <div className="md:col-span-2">
            <FieldLabel>Source (dot-path into request body)</FieldLabel>
            <TextInput
              value={caseData.source ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, source: v }))}
              placeholder="continent"
            />
          </div>
        )}
      </div>

      <button
        type="button"
        onClick={() => setShowAdvanced((b) => !b)}
        className="text-xs text-zinc-400 hover:text-zinc-200 cursor-pointer flex items-center gap-1"
      >
        <svg
          className={`w-3 h-3 transition-transform ${showAdvanced ? "rotate-90" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        Advanced (key, array_key, defaults)
      </button>

      {showAdvanced && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 pl-4 border-l border-zinc-800">
          <div>
            <FieldLabel>Key</FieldLabel>
            <TextInput
              value={caseData.key ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, key: v }))}
              placeholder="id"
            />
            <p className="text-xs text-zinc-500 mt-1">
              Lookup field for update / delete merge.
            </p>
          </div>
          <div>
            <FieldLabel>Array key</FieldLabel>
            <TextInput
              value={caseData.array_key ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, array_key: v }))}
              placeholder="items"
            />
            <p className="text-xs text-zinc-500 mt-1">
              The array field inside the stub file (when not the top level).
            </p>
          </div>
          <div>
            <FieldLabel>Defaults file</FieldLabel>
            <TextInput
              value={caseData.defaults ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, defaults: v }))}
              placeholder="defaults/created.json"
            />
            <p className="text-xs text-zinc-500 mt-1">
              JSON defaults merged into appended / updated records.
            </p>
          </div>
        </div>
      )}

      {caseData.merge === "cascade" && (
        <div className="text-xs text-amber-400/80 bg-amber-500/10 border border-amber-500/30 rounded px-3 py-2">
          Cascade mutations (primary + cascade targets) are configured directly
          in the config file in v1. The form preserves any existing cascade
          settings on this case unchanged.
        </div>
      )}
    </div>
  );
}
