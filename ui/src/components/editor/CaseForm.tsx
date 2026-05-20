import { useEffect, useState } from "react";
import type { Case, RouteKind } from "../../types/config";
import {
  ErrorText,
  FieldLabel,
  GhostButton,
  Select,
  Textarea,
  TextInput,
} from "./SectionCard";
import { PersistFields } from "./PersistFields";

export function CaseForm({
  name,
  caseData,
  kind,
  errorReason,
  onRename,
  onPatch,
  onRemove,
}: {
  name: string;
  caseData: Case;
  kind: RouteKind;
  errorReason?: string;
  onRename: (newName: string) => void;
  onPatch: (fn: (c: Case) => Case) => void;
  onRemove: () => void;
}) {
  const [draftName, setDraftName] = useState(name);
  const bodyMode: "json" | "file" = caseData.file ? "file" : "json";

  // Keep the draft in sync if the case is renamed elsewhere (cascade from a
  // sibling, undo, etc.).
  useEffect(() => {
    setDraftName(name);
  }, [name]);

  const commitRename = () => {
    const trimmed = draftName.trim();
    if (trimmed && trimmed !== name) {
      onRename(trimmed);
    } else if (!trimmed) {
      setDraftName(name);
    }
  };

  return (
    <div className="border border-zinc-800 rounded-lg p-3 bg-zinc-950/60">
      <div className="flex items-start gap-3">
        <div className="flex-1">
          <FieldLabel>Case name</FieldLabel>
          <input
            type="text"
            value={draftName}
            onChange={(e) => setDraftName(e.target.value)}
            onBlur={commitRename}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                commitRename();
                (e.target as HTMLInputElement).blur();
              }
              if (e.key === "Escape") {
                setDraftName(name);
                (e.target as HTMLInputElement).blur();
              }
            }}
            placeholder="success"
            spellCheck={false}
            className="w-full bg-zinc-900 border border-zinc-800 rounded px-2.5 py-1.5 text-sm text-zinc-100 placeholder-zinc-600 focus:outline-none focus:border-zinc-600 font-mono"
          />
          <p className="text-xs text-zinc-500 mt-1">
            {draftName !== name
              ? "Tab or Enter to rename — all references update."
              : "Referenced by conditions, transitions, and fallback by this name."}
          </p>
        </div>
        <div className="mt-5">
          <GhostButton onClick={onRemove} danger title="Remove this case">
            Remove
          </GhostButton>
        </div>
      </div>

      {errorReason && <ErrorText>{errorReason}</ErrorText>}

      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mt-3">
        <div>
          <FieldLabel>Status</FieldLabel>
          <TextInput
            type="number"
            value={caseData.status || 0}
            onChange={(v) => onPatch((c) => ({ ...c, status: Number(v) || 0 }))}
            placeholder={kind === "grpc" ? "0 = OK" : "200"}
          />
        </div>
        <div>
          <FieldLabel>Delay (seconds)</FieldLabel>
          <TextInput
            type="number"
            min={0}
            value={caseData.delay || 0}
            onChange={(v) => onPatch((c) => ({ ...c, delay: Number(v) || 0 }))}
          />
        </div>
        <div className="flex items-end">
          <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
            <input
              type="checkbox"
              checked={!!caseData.persist}
              onChange={(e) => onPatch((c) => ({ ...c, persist: e.target.checked }))}
              className="accent-emerald-500"
            />
            Persist request to stub file
          </label>
        </div>
      </div>

      <div className="mt-3">
        <FieldLabel>Body source</FieldLabel>
        <div className="flex gap-2 text-xs">
          <Select
            value={bodyMode}
            onChange={(v) => {
              if (v === "json") {
                onPatch((c) => ({ ...c, file: "" }));
              } else {
                onPatch((c) => ({ ...c, json: "" }));
              }
            }}
            options={[
              { value: "json", label: "Inline JSON" },
              { value: "file", label: "File on disk" },
            ]}
            className="max-w-[200px]"
          />
        </div>
        {bodyMode === "json" ? (
          <div className="mt-2">
            <Textarea
              value={caseData.json ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, json: v }))}
              placeholder='{"id":"af","name":"Africa"}'
              rows={4}
            />
            <p className="text-xs text-zinc-500 mt-1">
              Templates: <code>{"{{uuid}}"}</code>, <code>{"{{timestamp}}"}</code>, <code>{"{{now}}"}</code>
            </p>
          </div>
        ) : (
          <div className="mt-2">
            <TextInput
              value={caseData.file ?? ""}
              onChange={(v) => onPatch((c) => ({ ...c, file: v }))}
              placeholder="stubs/africa.json"
            />
            <p className="text-xs text-zinc-500 mt-1">
              Placeholders: <code>{"{path.id}"}</code> for URL params, <code>{"{source.field}"}</code> for body fields
            </p>
          </div>
        )}
      </div>

      {caseData.persist && (
        <PersistFields
          caseData={caseData}
          onPatch={onPatch}
          isGRPC={kind === "grpc"}
        />
      )}
    </div>
  );
}
