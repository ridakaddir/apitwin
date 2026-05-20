import type { ReactNode } from "react";
import { PrimaryButton } from "./SectionCard";

export function SaveBar({
  filePicker,
  errorCount,
  dirty,
  saving,
  saveError,
  onSave,
  onCancel,
  extra,
}: {
  filePicker: ReactNode;
  errorCount: number;
  dirty: boolean;
  saving: boolean;
  saveError: string | null;
  onSave: () => void;
  onCancel: () => void;
  extra?: ReactNode;
}) {
  return (
    <div className="sticky bottom-0 left-0 right-0 bg-zinc-950 border-t border-zinc-800 px-6 py-3 flex items-center gap-4 z-20">
      <div className="flex-1 min-w-0">{filePicker}</div>
      {extra}
      <div className="flex items-center gap-3">
        {errorCount > 0 && (
          <span className="text-xs text-red-400">
            {errorCount} issue{errorCount !== 1 && "s"}
          </span>
        )}
        {saveError && (
          <span className="text-xs text-red-400 max-w-xs truncate" title={saveError}>
            {saveError}
          </span>
        )}
        {!dirty && !saveError && (
          <span className="text-xs text-zinc-500">No changes</span>
        )}
        <button
          type="button"
          onClick={onCancel}
          className="text-sm px-3 py-1.5 rounded border border-zinc-700 text-zinc-300 hover:bg-zinc-800 cursor-pointer"
        >
          Cancel
        </button>
        <PrimaryButton
          onClick={onSave}
          disabled={saving || !dirty || errorCount > 0}
          title={
            errorCount > 0
              ? "Fix validation issues before saving"
              : !dirty
                ? "No changes to save"
                : undefined
          }
        >
          {saving ? "Saving…" : "Save"}
        </PrimaryButton>
      </div>
    </div>
  );
}
