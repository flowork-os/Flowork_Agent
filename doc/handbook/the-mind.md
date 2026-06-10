# The Mind — Brain, Educational Errors & the Router

## 🧠 The Brain

Every agent's memory lives in its own `state.db` — clone the folder and the memory comes with it.
Nothing is shared with other agents unless you wire it.

- **Local memory (FTS5)** — a fast keyword memory (SQLite FTS5 / BM25). No embeddings, so it's
  lightweight, instant, and fully offline. The agent stores what it sees with `brain_add` and recalls
  related memories with `brain_search`.
- **Wings** — every memory is filed in a "wing": *general, experience, eureka,* or *constitution*.
  Duplicates are dropped by content hash.
- **Two tiers** — on top of each agent's own memory sits a big shared corpus it can draw on.
- **Dream → Eureka** — while idle, a quiet pass consolidates recurring patterns into `eureka`
  insights. The brain grows from its own experience — no retraining.
- **Immune system** — an antibody scanner quarantines poisoned memories so it never gets corrupted.

A turn: a message comes in → the agent **remembers** it → **recalls** related memories → **thinks**
(calls the LLM with its doctrine + the recalled context) → **replies**. Memory first, then thought.

## 📚 Educational Errors

An original, dated design principle (see the [blueprint](https://github.com/flowork-os/doc/blob/main/EDUCATIONAL_ERRORS.md)).
Most AI hides a mistake — suppress it, fine-tune it away. **Flowork treats an error as education.**
Every mistake is **Captured**, **Explained**, **Retained**, and **Redemptive** (the agent is
quarantined, not deleted, and given a chance to correct). The loop: make a mistake → capture and
explain it → keep it as a lesson → recall it next time → don't hit the same wall. Learning at
runtime, no retraining.

## 🔀 The Router — and why to use it

You can point an agent at any LLM, but we recommend pointing it at **Flowork's own router**
(`http://127.0.0.1:2402/v1/chat/completions`):

- **One door, model-agnostic** — swap the model in one place; route to a subscription or a local
  model.
- **The anti-hallucination antibody** — before the model answers, the router injects the agent's own
  most-recurrent, most-relevant past mistakes (ranked by *karma × relevance*), so a hallucination gets
  harder to repeat over time — deterministically, no retraining, no GPU. See the
  [blueprint](https://github.com/flowork-os/doc/blob/main/ANTI_HALLUCINATION_ANTIBODY.md).
- **The constitution, every turn** — sacred rules (a 5W1H gate, an identity guard, a truth rule) are
  injected on every turn. Anti-hallucination isn't a setting — it's law.

Aim an agent at a raw third-party API and it gets none of this. Aim it at the router and it gets all
three for free. The mistakes the Brain keeps are exactly what the Router injects back as antibodies.
