---
name: rv-review
description: Address code review feedback left in the rv local review tool — list unresolved comments, fix each issue, and reply/resolve. Use when the user asks to check rv comments, address rv feedback, "see what rv says", or mentions they've left comments in rv for you.
---

# rv review

`rv` is a local git-diff review tool the user runs in a terminal alongside you. They review your working-tree changes there and leave inline comments anchored to specific lines; your job is to read those comments, fix the code, and reply.

## Loop

1. List unresolved comments from the repo root:

   ```
   rv comment list --unresolved --json
   ```

   Each entry has `id`, `file`, `old_line`/`new_line` (1-based, matching the current working tree), `body`, `author`, `resolved`, and any existing `replies`.

2. For each comment, open `file` and look at the code around that line. Make the change the comment is asking for.

3. Reply and resolve in one call:

   ```
   rv comment reply <id> "<one-line summary of what you changed>" --resolve
   ```

   If you disagree, need clarification, or the request doesn't make sense as-is, reply **without** `--resolve` and explain why — leave it open for the user to respond to. Never resolve a comment you haven't actually acted on.

4. After working through all comments, run `rv comment list --unresolved --json` again to confirm none are left, and summarize what you changed.

## Notes

- These commands operate on the current git repo's working tree vs `HEAD` — run them from inside the repo the user is reviewing.
- The user may have the `rv` TUI open in another terminal the whole time. It polls the session file and picks up your replies within ~1s — no need to ask them to reopen it.
- Full command reference:
  - `rv comment list [--file F] [--unresolved] [--json]`
  - `rv comment resolve <id>`
  - `rv comment reply [--author=agent] [--resolve] <id> <text...>` (flags can go anywhere in the args, including after `<id> <text...>`)
  - `rv session get [--json]`
