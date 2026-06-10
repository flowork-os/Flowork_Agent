# ▦ App

Little self-contained programs that live inside Flowork — each is **both a screen you click and a set
of tools your agents can use** ("one state, two drivers"). A quant desk and a notepad ship as
examples.

Two tabs: **Installed** and **Store**.
- **Install** — upload a `.fwpack`. Because an app can run a real program on your computer, installing
  asks for your consent first.
- **Open** — launches the app in a locked-down sandboxed frame; it can only talk to Flowork through
  validated *ops* (it asks `{op, args}`, the host checks the op is declared in the app's manifest,
  runs it, returns the result).
- **Uninstall** — remove it.

## For developers — make an app
A folder under `apps/<id>/` with three things:
```
apps/my-app/
├─ manifest.json   kind:"app" + the list of ops
├─ core.py         the headless logic (talks over stdin/stdout, line-JSON)
└─ ui/index.html   the screen (sandboxed iframe)
```
Every op you declare becomes **both a GUI button and an agent tool** at once.

**Principle:** write the logic once; a human clicking and an agent calling both run it — same state,
two drivers. (See the [blueprint](https://github.com/flowork-os/doc/blob/main/ONE_STATE_TWO_DRIVERS.md).)
