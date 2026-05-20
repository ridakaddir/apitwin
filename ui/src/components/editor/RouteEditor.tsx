import { useEffect, useMemo, useState } from "react";
import type {
  ConfigFileInfo,
  GRPCRoute,
  Route,
  RouteKind,
} from "../../types/config";
import { fetchConfigFiles, saveRoute } from "../../api/configApi";
import {
  emptyGRPCRoute,
  emptyHTTPRoute,
  useRouteForm,
} from "../../hooks/useRouteForm";
import { HeaderSection } from "./HeaderSection";
import { CasesSection } from "./CasesSection";
import { ConditionsSection } from "./ConditionsSection";
import { TransitionsSection } from "./TransitionsSection";
import { PreviewPane } from "./PreviewPane";
import { FilePicker } from "./FilePicker";
import { SaveBar } from "./SaveBar";

export interface RouteEditorProps {
  mode: "new" | "edit";
  kind: RouteKind;
  initialRoute?: Route | GRPCRoute;
  onClose: () => void;
  onSaved: () => void;
}

function slugFromMatch(match: string): string {
  if (!match) return "";
  // gRPC: "/package.Service/Method" -> "service-method"
  if (match.startsWith("/") && match.includes(".") && match.split("/").length === 3) {
    const last = match.split("/").pop() ?? "";
    return last.toLowerCase();
  }
  // HTTP: "/api/team/{id}" -> "team"
  const parts = match
    .replace(/^~?\/+/, "")
    .split("/")
    .filter((p) => p && !p.startsWith("{") && !p.includes("*") && p !== "api");
  const slug = parts[0] ?? "";
  return slug.replace(/[^a-zA-Z0-9_-]/g, "").toLowerCase();
}

function suggestedFileName(
  kind: "http" | "grpc",
  existing: { name: string; format: string }[],
): string {
  const ext = existing[0]?.format === "yaml" ? "yaml" : "toml";
  const base = kind === "grpc" ? "grpc-routes" : "routes";
  const existingNames = new Set(existing.map((f) => f.name));
  if (!existingNames.has(`${base}.${ext}`)) return `${base}.${ext}`;
  let i = 2;
  while (existingNames.has(`${base}-${i}.${ext}`)) i++;
  return `${base}-${i}.${ext}`;
}

export function RouteEditor({
  mode,
  kind: initialKind,
  initialRoute,
  onClose,
  onSaved,
}: RouteEditorProps) {
  const seed =
    initialRoute ?? (initialKind === "http" ? emptyHTTPRoute() : emptyGRPCRoute());
  const form = useRouteForm(initialKind, seed);

  const [files, setFiles] = useState<ConfigFileInfo[]>([]);
  const [selectedFile, setSelectedFile] = useState<string>("");
  const [newFileMode, setNewFileMode] = useState(false);
  const [newFileName, setNewFileName] = useState("");
  const [isDir, setIsDir] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const filter =
      mode === "edit"
        ? {
            kind: form.kind,
            match: (initialRoute?.match ?? "") as string,
            method:
              form.kind === "http"
                ? (initialRoute as Route | undefined)?.method
                : undefined,
          }
        : undefined;
    fetchConfigFiles(filter)
      .then((fs) => {
        if (!active) return;
        setFiles(fs);
        const dirMode = fs.length !== 1 || fs[0].path !== fs[0].name;
        setIsDir(dirMode);
        if (mode === "edit") {
          const current = fs.find((f) => f.containsRoute) ?? fs[0];
          if (current) setSelectedFile(current.path);
          if (fs.length === 0) setNewFileMode(true);
        } else {
          // New routes default to a new file with an auto-suggested name
          // derived from the route match (e.g. /team -> team.toml). This
          // avoids the confusing "drop my new POST into product-detail.toml"
          // default.
          setNewFileMode(true);
          setNewFileName(
            suggestedFileName(initialKind, fs) ||
              (fs[0]?.format === "yaml" ? "routes.yaml" : "routes.toml"),
          );
          if (fs.length > 0 && !dirMode) setSelectedFile(fs[0].path);
        }
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [mode, form.kind, initialRoute, initialKind]);

  // When the user types into the match field for a new route, keep the
  // suggested filename in sync (until they edit it manually).
  const [filenameTouched, setFilenameTouched] = useState(false);
  useEffect(() => {
    if (mode !== "new" || !newFileMode || filenameTouched) return;
    const ext = files[0]?.format === "yaml" ? "yaml" : "toml";
    const slug = slugFromMatch(form.route.match);
    if (slug) setNewFileName(`${slug}.${ext}`);
  }, [mode, newFileMode, filenameTouched, form.route.match, files]);

  const caseNames = useMemo(
    () => Object.keys(form.route.cases ?? {}),
    [form.route.cases],
  );

  const fileFormat = useMemo(() => {
    if (newFileMode) {
      const ext = newFileName.split(".").pop()?.toLowerCase();
      if (ext === "toml" || ext === "json") return ext;
      if (ext === "yaml" || ext === "yml") return "yaml";
      return undefined;
    }
    return files.find((f) => f.path === selectedFile)?.format;
  }, [files, selectedFile, newFileMode, newFileName]);

  const handleSave = async () => {
    setSaveError(null);
    let targetPath = selectedFile;
    if (newFileMode) {
      if (!newFileName.trim()) {
        setSaveError("Enter a filename for the new file");
        return;
      }
      const dir = files[0]?.path?.replace(/\/[^/]+$/, "") ?? "";
      targetPath = dir ? `${dir}/${newFileName.trim()}` : newFileName.trim();
    }
    if (!targetPath) {
      setSaveError("Pick a file to save to");
      return;
    }
    const containingFile = files.find((f) => f.containsRoute);
    const replace =
      mode === "edit" && initialRoute
        ? {
            match: initialRoute.match,
            method:
              form.kind === "http"
                ? (initialRoute as Route).method
                : undefined,
          }
        : undefined;

    setSaving(true);
    try {
      await saveRoute({
        file: targetPath,
        kind: form.kind,
        route: form.route,
        replace: replace && containingFile?.path === targetPath ? replace : undefined,
        ifMatch: containingFile?.mtime,
      });
      if (
        replace &&
        containingFile &&
        containingFile.path !== targetPath
      ) {
        // moved between files: explicit move not supported in v1
        setSaveError(
          "Move-between-files is not supported in v1. Save kept the route in its original file.",
        );
        return;
      }
      onSaved();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const errorCount = Object.keys(form.errors).length;
  const heading =
    mode === "new"
      ? `New ${form.kind === "http" ? "HTTP" : "gRPC"} route`
      : `Edit ${form.kind === "http" ? "HTTP" : "gRPC"} route — ${form.route.match}`;

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b border-zinc-800 px-6 py-4 flex items-center justify-between bg-zinc-950 sticky top-0 z-10">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={onClose}
            className="text-zinc-400 hover:text-zinc-100 cursor-pointer flex items-center gap-1"
            title="Back to routes"
          >
            <svg
              className="w-4 h-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
            </svg>
            <span className="text-sm">Back</span>
          </button>
          <h1 className="text-lg font-semibold tracking-tight">{heading}</h1>
        </div>
        {form.kind === "grpc" && form.grpcSchemaAvailable === false && (
          <span className="text-xs text-amber-400 bg-amber-500/10 border border-amber-500/30 rounded px-2 py-1">
            structural validation only — start apitwin with --grpc-proto for schema checks
          </span>
        )}
      </header>

      <div className="flex-1 grid grid-cols-1 lg:grid-cols-2 gap-6 px-6 py-6 max-w-7xl w-full mx-auto">
        <div className="space-y-5">
          <HeaderSection
            kind={form.kind}
            route={form.route}
            caseNames={caseNames}
            onMethod={form.setMethod}
            onMatch={form.setMatch}
            onEnabled={form.setEnabled}
            onFallback={form.setFallback}
            errors={form.errors}
          />
          <CasesSection
            cases={form.route.cases}
            kind={form.kind}
            errors={form.errors}
            onAdd={form.addCase}
            onRemove={form.removeCase}
            onRename={form.renameCase}
            onPatchCase={form.patchCase}
          />
          <ConditionsSection
            conditions={form.route.conditions}
            caseNames={caseNames}
            onAdd={form.addCondition}
            onRemove={form.removeCondition}
            onPatch={form.patchCondition}
          />
          <TransitionsSection
            transitions={form.route.transitions}
            caseNames={caseNames}
            onAdd={form.addTransition}
            onRemove={form.removeTransition}
            onPatch={form.patchTransition}
            onMove={form.moveTransition}
          />
        </div>
        <div className="lg:sticky lg:top-[73px] lg:self-start lg:h-[calc(100vh-9rem)]">
          <PreviewPane
            text={form.previewText}
            format={form.previewFormat}
            onFormatChange={form.setPreviewFormat}
            fileFormat={fileFormat}
          />
        </div>
      </div>

      <SaveBar
        filePicker={
          <FilePicker
            files={files}
            selectedPath={selectedFile}
            onSelect={setSelectedFile}
            newFileMode={newFileMode}
            onToggleNew={setNewFileMode}
            newFileName={newFileName}
            onNewFileName={(v) => {
              setFilenameTouched(true);
              setNewFileName(v);
            }}
            isDir={isDir}
          />
        }
        errorCount={errorCount}
        dirty={form.dirty}
        saving={saving}
        saveError={saveError}
        onSave={handleSave}
        onCancel={onClose}
      />
    </div>
  );
}
