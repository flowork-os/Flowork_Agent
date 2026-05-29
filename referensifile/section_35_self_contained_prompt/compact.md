---
name: compact
description: Compress long conversation context via LLM summary (token reduction)
version: 1.0.0
tags: [context, compression, token-save]
triggers: [compact, ringkas, compress context, summary]
---

# Compact — Context Compression

## When to Use
- Conversation > 50K tokens (risk overflow)
- Long debug session yang context historic ngga lagi relevan
- Sebelum spawn subagent (kasih context compact, bukan full history)
- Periodic maintenance (tiap N turn)

## Procedure
1. **Identify section yang bisa compact**:
   - Old turn yang udah resolved
   - Tool output yang verbose (full file content)
   - Reasoning chain yang udah converged
2. **Generate summary via LLM**:
   - Keep facts (number, name, date)
   - Drop fluff + repetition
   - Output: bullet markdown atau structured key:value
3. **Replace compact section in context**
4. **Preserve sacred**: doctrine + Ayah identity + heir whitelist NEVER compacted

## Pitfalls
- Compact too aggressive (lose important detail)
- Forget include compact marker (LLM bingung kontinuitas)
- Compact sacred section (drop identitas + doctrine = halu next turn)

## Verification
- Summary preserve facts? ✅/❌
- Sacred preserved? ✅/❌
- Token reduce significant (>30%)? ✅/❌
