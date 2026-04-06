# Hot Reload

[Home](/) > [Features](index) > Hot Reload

---

apitwin watches the config file or directory for changes using filesystem notifications. Edit a case, change a `fallback`, or drop a new `.toml` file into the config directory — the next request picks up the changes with no restart.

## How it works

- apitwin uses `fsnotify` to watch all config files
- When a file change is detected, the config is reloaded before the next request
- No server restart, no connection drops, no downtime

## What triggers a reload

- Editing any config file (`.toml`, `.yaml`, `.json`)
- Adding a new config file to the config directory
- Removing a config file from the config directory
- Renaming a config file

## Cross-reference dependency tracking

apitwin automatically tracks **cross-reference dependencies** between stub files. When a stub file contains a <code v-pre>{{ref:...}}</code> token that points to a directory, apitwin monitors that directory for changes and ensures dependent data stays up to date.

**Example:** A continent file references its countries:

```json
{
  "name": "Africa",
  "countries": "{{ref:stubs/countries/?filter=continent:africa&template=stubs/templates/country-summary.json}}"
}
```

When a country is created, updated, or deleted in `stubs/countries/`, apitwin detects the change and the next GET on the continent returns the updated `countries` array — no restart needed.

### How it works

1. On startup, apitwin scans all stub files for <code v-pre>{{ref:...}}</code> tokens pointing to directories
2. It builds a dependency map: which stub files depend on which directories
3. When a persist operation (create, update, delete) modifies a file in a tracked directory, the change is detected
4. Rapid changes are batched (100ms window) to avoid redundant processing
5. Dependent files are flagged and the next read resolves the refs against fresh data

### What is tracked

- Directory references in stub files (paths ending with `/`)
- Newly created stub files are registered for tracking automatically
- Changes via all persist operations: `merge = "append"`, `merge = "update"`, `merge = "delete"`
- Background transitions (e.g. `pending` to `verified`) also trigger dependency updates

## Terminal output

apitwin logs indicate where each response comes from:

```
via=stub   — response served from a local mock file
via=proxy  — response forwarded to the upstream API
```

When cross-reference dependencies change, apitwin logs the update:

```
INFO stub watcher: dependencies changed directories=1 affected_files=2
```

## Notes

- [Response transitions](response-transitions.md) are **reset** when a config hot reload occurs — the timeline restarts from the beginning
- Stub files (in `stubs/`) are read on each request, so changes to stub files always take effect immediately
- Cross-reference directory dependencies are tracked automatically — no configuration needed

---

**See also:** [Configuration](../configuration/) | [Response Transitions](response-transitions.md) | [Cross-Endpoint References](cross-endpoint-references.md)
