import type { Case } from "../types/config";

export function CaseDetail({ name, caseData }: { name: string; caseData: Case }) {
  const fields: [string, string][] = [];

  if (caseData.status) fields.push(["status", String(caseData.status)]);
  if (caseData.file) fields.push(["file", caseData.file]);
  if (caseData.json) fields.push(["json", caseData.json.length > 60 ? caseData.json.slice(0, 60) + "..." : caseData.json]);
  if (caseData.delay) fields.push(["delay", `${caseData.delay}s`]);
  if (caseData.persist) fields.push(["persist", "true"]);
  if (caseData.merge) fields.push(["merge", caseData.merge]);
  if (caseData.key) fields.push(["key", caseData.key]);
  if (caseData.array_key) fields.push(["array_key", caseData.array_key]);
  if (caseData.defaults) fields.push(["defaults", caseData.defaults]);

  return (
    <div className="bg-zinc-900/50 rounded-md px-3 py-2">
      <div className="text-sm font-mono text-zinc-200 mb-1">{name}</div>
      {fields.length > 0 ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
          {fields.map(([key, val]) => (
            <div key={key} className="contents">
              <dt className="text-zinc-500">{key}</dt>
              <dd className="text-zinc-400 font-mono truncate">{val}</dd>
            </div>
          ))}
        </dl>
      ) : (
        <p className="text-xs text-zinc-600">200 (default)</p>
      )}
    </div>
  );
}
