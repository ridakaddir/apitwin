import { useState } from "react";
import type { Case, RouteKind } from "../../types/config";
import { GhostButton, SectionCard, TextInput } from "./SectionCard";
import { CaseForm } from "./CaseForm";

export function CasesSection({
  cases,
  kind,
  errors,
  onAdd,
  onRemove,
  onRename,
  onPatchCase,
}: {
  cases: Record<string, Case>;
  kind: RouteKind;
  errors: Record<string, string>;
  onAdd: (name: string) => void;
  onRemove: (name: string) => void;
  onRename: (oldName: string, newName: string) => void;
  onPatchCase: (name: string, fn: (c: Case) => Case) => void;
}) {
  const [newName, setNewName] = useState("");
  const names = Object.keys(cases);

  const addNew = () => {
    const trimmed = newName.trim();
    if (!trimmed || trimmed in cases) return;
    onAdd(trimmed);
    setNewName("");
  };

  return (
    <SectionCard
      title={`Cases (${names.length})`}
      description="Define the named responses this route can return. Other sections reference these by name."
      action={
        <div className="flex items-center gap-2">
          <TextInput
            value={newName}
            onChange={setNewName}
            placeholder="case name"
            className="!w-40"
          />
          <GhostButton
            onClick={addNew}
            disabled={!newName.trim() || newName.trim() in cases}
          >
            + Add case
          </GhostButton>
        </div>
      }
    >
      {names.length === 0 && (
        <p className="text-sm text-zinc-500 text-center py-4">
          No cases yet. Add one above to start defining responses.
        </p>
      )}
      {names.map((name) => (
        <CaseForm
          key={name}
          name={name}
          caseData={cases[name]}
          kind={kind}
          errorReason={errors["cases." + name]}
          onRename={(newName) => onRename(name, newName)}
          onPatch={(fn) => onPatchCase(name, fn)}
          onRemove={() => onRemove(name)}
        />
      ))}
    </SectionCard>
  );
}
