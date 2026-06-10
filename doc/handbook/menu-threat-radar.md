# 🛡️ Threat Radar

Flowork's built-in security scanner — a live dashboard that watches the code your agents run, and
lets you scan your own code or an authorized external target. No other agent framework ships this.

## The screen
A radar sweep on the left with three numbers — **runs**, **findings**, and **critical** (red if
anything critical is live, green if clean). The critical number is the worst result from the *latest*
scan of each target, so it goes back down when you fix something. On the right: **Scan Log** (every
scan, newest first) and **Findings** (click any run to see what it found). It refreshes every few
seconds.

## Buttons (top-right)
- **⟳ Refresh** — reload the scan list.
- **⊕ Scan Target** — open the scan form: pick a *Tool*, a *Target*, optional *Args*, and a *Category*
  (`immune` = hardening your own code, `pentest` = an authorized external target). The tool list and
  target list come from an **owner-editable allowlist** — Flowork won't run a tool or touch a target
  that isn't on it, and there's no shell in the middle.
- **≣ Arsenal** — the catalog of everything the scanner can use: defensive code auditors (the core,
  marked `CORE`, can't be removed), tools, and thousands of detection checks. Search it, install /
  uninstall any pack.

## For developers — make your own scanner
A check is a **nuclei template** (a small YAML that says "look for this"):

```yaml
id: exposed-env-file
info:
  name: Exposed .env file
  author: you
  severity: high
http:
  - method: GET
    path:
      - "{{BaseURL}}/.env"
    matchers:
      - type: word
        words:
          - "DB_PASSWORD"
```

Two ways to add it:
1. **One check** — POST it to `/api/scanner/checks/add` with `{ name, yaml }`. It runs through
   `nuclei -validate`; a good one lands in `<nuclei-templates>/flowork-private/` and shows up in the
   Arsenal.
2. **A pack** — bundle many checks into a `kind:scanner` `.fwpack`:

```
my-scanner.fwpack (zip)
├─ plugin.json   { "id": "my-scanner", "kind": "scanner", "scanner": { "name": "...", "description": "..." } }
└─ checks/
   ├─ check-1.yaml
   └─ check-2.yaml
```

Install with `/api/scanner/packs/install`. Every check is validated; the rest snap into the Arsenal.

**Safety:** everything is owner-only and local; every check is validated; templates run inert; scans
only ever touch the tools and targets on your allowlist.
