import { useMemo, useState } from "react";
import type { Route, GRPCRoute, GRPCMethodSchema, GRPCInvokeResult } from "../types/config";
import { invokeGRPC } from "../api/client";

interface Response {
  status: number;
  statusText: string;
  headers: [string, string][];
  body: string;
  latency: number;
}

export function RequestTester({
  route,
  kind,
  grpcSchemas,
}: {
  route: Route | GRPCRoute;
  kind: "http" | "grpc";
  grpcSchemas: GRPCMethodSchema[] | null;
}) {
  if (kind === "grpc") {
    return <GRPCTester route={route as GRPCRoute} schemas={grpcSchemas} />;
  }
  return <HTTPTester route={route as Route} />;
}

function HTTPTester({ route }: { route: Route }) {
  const method = route.method;
  const hasBody = !["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase());

  const defaultPath = route.match.replace(/\{(\w+)\}/g, ":$1").replace(/\*/g, "example");

  const [path, setPath] = useState(defaultPath);
  const [body, setBody] = useState("");
  const [headersText, setHeadersText] = useState("");
  const [queryText, setQueryText] = useState("");
  const [response, setResponse] = useState<Response | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const send = async () => {
    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      let url = path.startsWith("/") ? path : `/${path}`;
      if (queryText.trim()) {
        const params = new URLSearchParams();
        queryText
          .split("\n")
          .filter((l) => l.includes("="))
          .forEach((l) => {
            const idx = l.indexOf("=");
            params.append(l.slice(0, idx).trim(), l.slice(idx + 1).trim());
          });
        url += (url.includes("?") ? "&" : "?") + params.toString();
      }

      const headers: Record<string, string> = {};
      if (hasBody && body.trim()) {
        headers["Content-Type"] = "application/json";
      }
      headersText
        .split("\n")
        .filter((l) => l.includes(":"))
        .forEach((l) => {
          const idx = l.indexOf(":");
          headers[l.slice(0, idx).trim()] = l.slice(idx + 1).trim();
        });

      const start = performance.now();
      const res = await fetch(url, {
        method: method.toUpperCase(),
        headers,
        body: hasBody && body.trim() ? body : undefined,
      });
      const latency = Math.round(performance.now() - start);

      const resHeaders: [string, string][] = [];
      res.headers.forEach((v, k) => resHeaders.push([k, v]));

      const text = await res.text();
      let formattedBody = text;
      try {
        formattedBody = JSON.stringify(JSON.parse(text), null, 2);
      } catch {
        // not JSON, keep as-is
      }

      setResponse({
        status: res.status,
        statusText: res.statusText,
        headers: resHeaders,
        body: formattedBody,
        latency,
      });
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  const statusColor = response
    ? response.status < 300
      ? "text-emerald-400"
      : response.status < 400
        ? "text-amber-400"
        : "text-red-400"
    : "";

  return (
    <div className="space-y-3">
      <div>
        <label className="text-xs text-zinc-500 block mb-1">Path</label>
        <div className="flex gap-2">
          <input
            type="text"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            className="flex-1 bg-zinc-900 border border-zinc-700 rounded px-3 py-1.5 text-sm font-mono text-zinc-100 focus:outline-none focus:border-zinc-500"
          />
          <button
            onClick={send}
            disabled={loading}
            className="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm font-medium rounded transition-colors cursor-pointer"
          >
            {loading ? "..." : "Send"}
          </button>
        </div>
      </div>

      <div>
        <label className="text-xs text-zinc-500 block mb-1">
          Query params <span className="text-zinc-600">(key=value, one per line)</span>
        </label>
        <textarea
          value={queryText}
          onChange={(e) => setQueryText(e.target.value)}
          rows={2}
          placeholder={"status=active\nlimit=10"}
          className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-1.5 text-sm font-mono text-zinc-100 placeholder-zinc-700 focus:outline-none focus:border-zinc-500 resize-y"
        />
      </div>

      <div>
        <label className="text-xs text-zinc-500 block mb-1">
          Headers <span className="text-zinc-600">(Name: value, one per line)</span>
        </label>
        <textarea
          value={headersText}
          onChange={(e) => setHeadersText(e.target.value)}
          rows={2}
          placeholder={"Authorization: Bearer token123"}
          className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-1.5 text-sm font-mono text-zinc-100 placeholder-zinc-700 focus:outline-none focus:border-zinc-500 resize-y"
        />
      </div>

      {hasBody && (
        <div>
          <label className="text-xs text-zinc-500 block mb-1">Body (JSON)</label>
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={4}
            placeholder={'{\n  "name": "example"\n}'}
            className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-1.5 text-sm font-mono text-zinc-100 placeholder-zinc-700 focus:outline-none focus:border-zinc-500 resize-y"
          />
        </div>
      )}

      {error && (
        <div className="text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded px-3 py-2">
          {error}
        </div>
      )}

      {response && (
        <div className="border border-zinc-700 rounded-lg overflow-hidden">
          <div className="flex items-center gap-3 px-3 py-2 bg-zinc-800/50 border-b border-zinc-700">
            <span className={`font-mono font-semibold text-sm ${statusColor}`}>
              {response.status} {response.statusText}
            </span>
            <span className="text-xs text-zinc-500">{response.latency}ms</span>
          </div>

          {response.headers.length > 0 && (
            <details className="border-b border-zinc-800">
              <summary className="px-3 py-1.5 text-xs text-zinc-500 cursor-pointer hover:text-zinc-400">
                Headers ({response.headers.length})
              </summary>
              <div className="px-3 pb-2">
                <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs font-mono">
                  {response.headers.map(([k, v], i) => (
                    <div key={i} className="contents">
                      <dt className="text-zinc-500">{k}</dt>
                      <dd className="text-zinc-400 truncate">{v}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            </details>
          )}

          <pre className="px-3 py-2 text-xs font-mono text-zinc-300 overflow-x-auto max-h-80 overflow-y-auto whitespace-pre-wrap break-words">
            {response.body || "(empty)"}
          </pre>
        </div>
      )}
    </div>
  );
}

function GRPCTester({
  route,
  schemas,
}: {
  route: GRPCRoute;
  schemas: GRPCMethodSchema[] | null;
}) {
  const schema = useMemo(
    () => schemas?.find((s) => s.method === route.match) ?? null,
    [schemas, route.match],
  );

  const [body, setBody] = useState("{}");
  const [metadataText, setMetadataText] = useState("");
  const [result, setResult] = useState<GRPCInvokeResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const streaming = schema?.streaming ?? false;
  const notLoaded = schemas !== null && schema === null;

  const send = async () => {
    setLoading(true);
    setError(null);
    setResult(null);

    try {
      let message: unknown = {};
      if (body.trim()) {
        try {
          message = JSON.parse(body);
        } catch (err) {
          throw new Error(`request body is not valid JSON: ${(err as Error).message}`);
        }
      }

      const metadata: Record<string, string> = {};
      metadataText
        .split("\n")
        .filter((l) => l.includes(":"))
        .forEach((l) => {
          const idx = l.indexOf(":");
          metadata[l.slice(0, idx).trim()] = l.slice(idx + 1).trim();
        });

      const r = await invokeGRPC(route.match, message, metadata);
      setResult(r);
    } catch (err) {
      setError(String((err as Error).message ?? err));
    } finally {
      setLoading(false);
    }
  };

  const codeColor = !result
    ? ""
    : result.code === "OK"
      ? "text-emerald-400"
      : "text-red-400";

  return (
    <div className="space-y-3">
      <div>
        <label className="text-xs text-zinc-500 block mb-1">Method</label>
        <div className="flex gap-2">
          <input
            type="text"
            value={route.match}
            readOnly
            className="flex-1 bg-zinc-900 border border-zinc-800 rounded px-3 py-1.5 text-sm font-mono text-zinc-400"
          />
          <button
            onClick={send}
            disabled={loading || streaming}
            className="px-4 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm font-medium rounded transition-colors cursor-pointer"
          >
            {loading ? "..." : "Invoke"}
          </button>
        </div>
        {schema && (
          <p className="mt-1 text-xs text-zinc-600 font-mono">
            {schema.inputType} → {schema.outputType}
          </p>
        )}
      </div>

      {streaming && (
        <div className="text-xs text-amber-400 bg-amber-500/10 border border-amber-500/20 rounded px-3 py-2">
          Streaming RPCs are not supported by the devtool client yet.
        </div>
      )}

      {notLoaded && (
        <div className="text-xs text-amber-400 bg-amber-500/10 border border-amber-500/20 rounded px-3 py-2">
          This route has no matching proto descriptor — did you forget to pass
          it to <code>--grpc-proto</code>?
        </div>
      )}

      <div>
        <label className="text-xs text-zinc-500 block mb-1">Request message (JSON)</label>
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={6}
          placeholder={'{\n  "country_code": "morocco"\n}'}
          className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-1.5 text-sm font-mono text-zinc-100 placeholder-zinc-700 focus:outline-none focus:border-zinc-500 resize-y"
        />
      </div>

      {schema && schema.fields.length > 0 && (
        <details>
          <summary className="text-xs text-zinc-500 cursor-pointer hover:text-zinc-400">
            Input fields ({schema.fields.length})
          </summary>
          <div className="mt-2 border border-zinc-800 rounded overflow-hidden">
            <table className="w-full text-xs font-mono">
              <tbody>
                {schema.fields.map((f) => (
                  <tr key={f.name} className="border-b border-zinc-800 last:border-0">
                    <td className="px-3 py-1.5 text-zinc-300">{f.jsonName}</td>
                    <td className="px-3 py-1.5 text-zinc-500">
                      {f.repeated ? `repeated ${f.kind}` : f.kind}
                      {f.messageType ? ` <${f.messageType}>` : ""}
                      {f.enumType ? ` <${f.enumType}>` : ""}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}

      <div>
        <label className="text-xs text-zinc-500 block mb-1">
          Metadata <span className="text-zinc-600">(Name: value, one per line)</span>
        </label>
        <textarea
          value={metadataText}
          onChange={(e) => setMetadataText(e.target.value)}
          rows={2}
          placeholder={"authorization: Bearer token"}
          className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-1.5 text-sm font-mono text-zinc-100 placeholder-zinc-700 focus:outline-none focus:border-zinc-500 resize-y"
        />
      </div>

      {error && (
        <div className="text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded px-3 py-2">
          {error}
        </div>
      )}

      {result && (
        <div className="border border-zinc-700 rounded-lg overflow-hidden">
          <div className="flex items-center gap-3 px-3 py-2 bg-zinc-800/50 border-b border-zinc-700">
            <span className={`font-mono font-semibold text-sm ${codeColor}`}>
              {result.code}
            </span>
            {result.message && (
              <span className="text-xs text-zinc-400 truncate">{result.message}</span>
            )}
            <span className="text-xs text-zinc-500 ml-auto">{result.latencyMs}ms</span>
          </div>
          <pre className="px-3 py-2 text-xs font-mono text-zinc-300 overflow-x-auto max-h-80 overflow-y-auto whitespace-pre-wrap break-words">
            {result.response !== undefined
              ? JSON.stringify(result.response, null, 2)
              : "(empty)"}
          </pre>
        </div>
      )}
    </div>
  );
}
