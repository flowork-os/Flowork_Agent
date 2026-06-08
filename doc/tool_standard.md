# Tool Standard — discipline for system-touching tools

> The convention every **new** tool that touches the real system (shell, file-write, network,
> power, exec) must follow, so tool maturity does not decay as tools are added ad-hoc. Existing
> tools are not retrofitted (many are owner-LOCKED); this is the bar for everything **new**.
>
> Flowork's structural defenses (WASM isolation + capability-broker + guardian + SandboxRunV3)
> already contain most blast radius — so a tool needs *correct, focused* checks, not exhaustive
> ones. Depth where it buys safety; lean everywhere else (the "pasukan semut" edge).

## The five obligations (every system-touching tool)

1. **Declared capability** — `Capability()` returns the *narrowest* cap that fits
   (`exec:shell`, `net:fetch:<host>`, `fs:write:/shared/*`, `exec:power`, …). The broker +
   SandboxRunV3 gate on this; never return `""` for a system-touching tool.

2. **Input validation at the boundary** — validate/normalize every argument before it reaches the
   OS. Concretely:
   - **Paths**: reject absolute paths + `..` traversal; resolve under the agent's shared dir and
     re-check with `filepath.Rel`/`HasPrefix` (defense in depth). Never trust a caller path.
   - **Commands**: classify by *structure*, not substring (see P1 `cmdsem`): parse program + args +
     operators; decide destructive / read-only / needs-sandbox from the parse, not a denylist scan.
   - **URLs**: SSRF guard (block loopback/private/metadata IPs, re-validate redirects).
   - **Numbers/enums**: clamp to a sane range / known set (timeouts, delays, actions).

3. **Bounded execution** — a timeout (ctx.WithTimeout), an output cap (truncate, don't OOM), and
   a resource limit where the OS allows it (rlimit). No unbounded read, no unbounded run.

4. **Safe-by-default for irreversible actions** — anything destructive/irreversible (power,
   delete, overwrite) defaults to **dry-run** or carries a **cancel window** + is **audit-logged**.
   Real execution only behind an explicit arm/confirm. (See `system_power`: ARM switch + dry-run +
   cancel-window + audit + argv-exec.)

5. **No secret leakage** — scrub the child env of tokens/credentials before spawn; never echo
   secrets into a tool result (the loket bridge also redacts, but the tool must not rely on it).

## Structure (mirror the reference's folder-per-tool, kept lean)
A non-trivial tool lives as its own small unit: the tool struct + schema, a `*_validate.go` /
`*_semantics.go` for the parsing/classification, and a `*_test.go` with the bypass/edge cases.
Trivial tools (read a value, return time) stay a single small struct — don't over-engineer.

## Testing obligation
Every system-touching tool ships with a Go unit test that asserts the **bypass cases** are caught
(e.g. `rm  -rf  /` with doubled spaces, `$IFS` indirection, `../` path escape, redirect-to-private
SSRF). A tool without a bypass test is not "done". This runs offline (no LLM, no network) so it is
part of `go test` and cannot regress silently.

## Why this exists
Without a written bar, each new tool is as safe as whoever wrote it that day. A standard makes new
system-touching tools **safe-by-construction** instead of safe-by-luck, and keeps the lean-but-correct
philosophy explicit so nobody "hardens" a tool into 2500 lines it doesn't need.
