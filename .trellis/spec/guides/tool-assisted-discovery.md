# Tool-assisted Discovery Guide

> **Purpose**: Prefer codegraph, codebase-memory, and Context7 before expensive manual search loops.

---

## Why This Guide Exists

Most wasted agent turns come from:

- Guessing third-party APIs instead of reading current docs
- Grepping for symbols when a graph already knows callers/callees
- Re-reading whole packages to answer “how does this flow work?”

This repository has three discovery tools that should be used when available.

---

## Tool Map

| Tool | Best question shape | Primary entry |
|---|---|---|
| **codegraph** | How does X work? What is on the path from A to B? What will this edit touch? | `codegraph_explore` with `projectPath` pointing at the indexed tree (e.g. repo root or `backend/`) |
| **codebase-memory** | Where is symbol X? Who calls it? What is the architecture? What broke after a change? | `search_graph` / `trace_path` / `get_architecture` / `detect_changes` |
| **Context7** | What is the current official API for library Y? | `resolve-library-id` then `query-docs` |
| **rg / read** | Non-code text, missing index, or graph returned nothing useful | Direct file tools |

---

## Decision Checklist (Before Coding)

- [ ] Is this a third-party library/API question? → **Context7 first**
- [ ] Is this “how does existing local code work / who calls X”? → **codegraph or codebase-memory first**
- [ ] Is the tree unindexed or tool unavailable? → fall back to `rg` + `read`
- [ ] Does a `.trellis/spec/` contract already answer the boundary question? → **read the spec**; tools do not override specs
- [ ] Are you about to edit a multi-file flow? → explore once with codegraph/memory, then edit

---

## Usage Rules

### 1. Context7

Use for Gin, sqlx, pgx, AWS SDK, CloudWeGo Eino, Vue, Vite, Pinia, Ant Design Vue, Element Plus, Vuetify, Playwright, and similar.

Rules:

- Resolve the library ID, then query one focused topic per call
- Prefer version-relevant docs when the project pins a version
- Never paste secrets into Context7 queries

### 2. codegraph

Use when a `.codegraph/` index exists for the target tree.

Rules:

- Pass `projectPath` when the session root is not the indexed project
- Treat returned source as already read for those symbols; do not immediately re-open the same files unless editing
- If no index exists, do **not** silently invent graph results; use file tools or ask to index

### 3. codebase-memory

Use for BM25/symbol search, call traces, architecture clusters, and change impact.

Rules:

- Prefer graph search over raw grep for Go/TS symbols and relationships
- Re-index only when the user requests it or the index is clearly stale/missing for the package you must change
- Do not commit graph DBs or treat them as review artifacts

### 4. Fallback order

```text
spec/task artifacts
  → Context7 (external APIs)
  → codegraph explore / codebase-memory graph
  → rg + read
```

---

## Common Mistakes

### Mistake 1: Grep-first for architecture

**Bad**: `rg NewRouter` across the whole monorepo to learn HTTP assembly  
**Good**: codegraph explore / memory search for the router composition path, then open only the needed files

### Mistake 2: Memorized library APIs

**Bad**: inventing Eino `Runner` / OpenAI client fields from training memory  
**Good**: Context7 (or the pinned module docs) for the version in `go.mod` / `package.json`

### Mistake 3: Tools override project contracts

**Bad**: following a generic framework tutorial that puts DB access in handlers  
**Good**: keep TravelAgent DDD boundaries from `.trellis/spec/backend/`

### Mistake 4: Committing indexes

**Bad**: adding `.codegraph/` or memory DB files to git  
**Good**: keep indexes local; source + specs remain the review surface

---

## When to Update This Guide

- A new MCP discovery tool becomes standard for this repo
- Index locations or tool names change
- A repeated failure mode shows agents still grepping first

---

## Related Specs

- Root `AGENTS.md` → Tool-assisted Discovery
- `.trellis/spec/backend/index.md` → backend pre-development checklist
- `.trellis/spec/guides/cross-layer-thinking-guide.md` → multi-layer flows after discovery
