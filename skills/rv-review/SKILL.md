# rv review — instructions for a coding agent

`rv` is a local git-diff review tool the user runs in a terminal alongside you. They review your working-tree changes there and leave inline comments anchored to specific lines. When the user says something like "check rv comments," "address the rv feedback," or "see what rv says," follow the loop below: read those comments, fix the code, and reply.

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
- If the user wants you to keep watching for feedback as they review (e.g. "let me know as I leave comments," "keep an eye on rv while I go through this"), don't just run the loop once — poll `rv comment list --unresolved --json` on an interval (a few seconds is enough) and work through anything new that shows up, for as long as the user wants you watching. Watch both: brand-new top-level comments, and new replies the user adds to a thread that's already open (e.g. following up on your own reply) — an unchanged `resolved: false` doesn't tell you whether `replies` grew since your last check, so diff each comment's replies against what you saw last poll rather than only looking at which comment `id`s are new.
- Full command reference:
  - `rv comment list [--file F] [--unresolved] [--json]`
  - `rv comment resolve <id>`
  - `rv comment reply [--author=agent] [--resolve] <id> <text...>` (flags can go anywhere in the args, including after `<id> <text...>`)
  - `rv session get [--json]`
