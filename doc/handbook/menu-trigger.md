# ⚡ Trigger

Jobs that run when something happens, not on a clock. Hit **＋ New** and pick a type:
- **File watch** — fires when a file or folder changes.
- **Webhook** — fires when something POSTs to it.

Then choose what runs (an agent or a group), what it should do, and where to deliver the result
(e.g. Telegram). Each trigger card has **Enable/Disable, ▷ Run, ✎ Edit, ▸ History, 🗑 Delete**. A
webhook trigger also shows its URL:

```
POST  http://your-host/api/triggers/hook/<id>?key=<secret>
```

Anything that can make an HTTP POST can fire it — a script, a camera, a web service — and the secret
keeps it private.

**Principle:** the outside world pokes Flowork → an agent does something → you get the answer.
