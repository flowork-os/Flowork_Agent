# Group template — a colony of ants (§F2)

A **group** is a module whose only job is to route ONE task to its MEMBER ants over
the loket bus (`bus.broadcast`) and gather their answers. It owns no domain logic and
never touches a member's folder — isolation holds, and a new team is just `copy folder
+ set members`.

## Wire a working group

1. **Members** — make the member ants (copy `templates/ant-template`, give each its own
   `workspace/prompt.md` persona). Each member just needs to answer a `{text}` message.
2. **Group config** — the group reads its roster from `FLOWORK_AGENT_CONFIG.kv.members`
   (comma-separated module ids). Set it in the group's own store:
   ```sql
   -- in <group>.fwagent/workspace/state.db
   INSERT INTO kv(k,v) VALUES('members','title-writer,hashtag-writer')
     ON CONFLICT(k) DO UPDATE SET v=excluded.v;
   ```
   Restart so the config is injected.
3. **Run** — invoke the group's `handle_message` with `{text}`; it broadcasts to every
   member and returns each reply.

## Proven example (2026-06-06)

`content-team` (this template) with members `title-writer` + `hashtag-writer`
(both `ant-template` copies). One call fanned the same content out to both ants:

```
POST /api/kernel/rpc {"plugin":"content-team","function":"handle_message",
                      "args":{"text":"<content>"}}
→ { "group":"content-team", "members":["title-writer","hashtag-writer"],
    "result": { "replies": [
        {"target":"title-writer",  "reply":{"reply":"<a catchy title>"}},
        {"target":"hashtag-writer","reply":{"reply":"#five #relevant #hash #tags #here"}}
    ]}}
```

The group reached its members ONLY through `bus.broadcast` (kernel-routed) — no module
touched another module's folder. That is the "pasukan semut" colony in one call.
