# rv — known gaps

Working list from comparing `rv` against Hunk/difit/tuicr and general git-TUI
conventions (lazygit, tig). Worked through top to bottom; check items off as
they land, with the commit that did it.

- [x] **Flexible diff target.** `rv <spec>` now passes any extra argument(s)
      straight through to `git diff` — `rv main..feature`, `rv --staged`,
      `rv HEAD~3`, etc. — instead of hardcoding `HEAD`. The header shows
      what you're diffing against (`vs HEAD`, `vs main..feature`, ...), and
      `R` re-diffs against the same spec.
- [x] **Open in `$EDITOR`.** `o` suspends the TUI and opens the file under
      the cursor at that line, via `$VISUAL`/`$EDITOR`/`vi` (matching git's
      own fallback chain) — `+N file` for terminal editors, `-g file:N` for
      VS Code.
- [x] **Hunk-level jump.** `}`/`{` jump to the next/prev hunk header within
      the current file (respects a count, e.g. `3}`). Doesn't wrap or cross
      files — reaching the last hunk and pressing `}` again is a no-op.
- [x] **Cross-file comment navigation.** `n`/`N` jump to the next/prev
      *unresolved* comment across all files, in sidebar order, wrapping
      around at either end. Resolved comments are skipped.
- [x] **Search/filter.** `/` opens a live-filtering sidebar search by file
      path (case-insensitive substring). Enter commits it and snaps the
      cursor onto a match; esc leaves whatever filter was already active
      untouched. `tab`/`[`/`]` only move between visible (matching) files
      while a filter is active. Scoped to file names, not diff content —
      `n`/`N` were already claimed for cross-file comment navigation, so a
      vim-style "/ searches content, n/N cycle matches" model would have
      collided with that.
- [x] **Help overlay.** `?` (or `esc`) opens/closes a full keybinding
      reference, grouped by Movement/Comments/Other. Footer trimmed back
      down to the essentials + "? help" now that it doesn't need to carry
      every binding.
- [x] **Config file / remappable keybindings.** `~/.config/rv/config` (or
      `$XDG_CONFIG_HOME/rv/config`), a plain `action = key1, key2` format —
      no new dependency for a TOML/YAML parser. A bad config doesn't block
      startup; it falls back to defaults and surfaces the parse error. The
      `?` help overlay and footer hint are now generated from the actual
      resolved Keymap rather than a hardcoded list, so a remap shows up
      correctly instead of the help screen lying about what a key does.
- [x] **Sidebar toggle.** `t` shows/hides the sidebar; the diff pane
      reclaims the full width when it's hidden.
- [x] **Delete confirmation.** `d` no longer deletes a comment immediately —
      it prompts "delete this comment? y/n" first, since it was a single
      destructive keystroke with no undo.
- [x] **Refresh moved off `R`.** Was one shift-key away from `r` (toggle
      resolved) on the same physical key, easy to mistype. Now `ctrl+r`.
- [x] **Line numbers.** `#` toggles a gutter showing old/new line numbers
      before each diff line.
- [x] **Line wrap.** `w` toggles wrapping long lines instead of truncating
      them with an ellipsis.
- [x] **Persisted UI prefs.** Sidebar visibility, line numbers, and wrap are
      saved to `~/.config/rv/ui-prefs.json` and restored on the next launch,
      in any repo — these are user prefs, not per-repo session state.
- [x] **File navigation wraps around.** `tab`/`[`/`]` now wrap at either end
      instead of clamping (matching `n`/`N`'s existing wraparound).
- [ ] **Watch/auto-reload.** Only reloads on manual `ctrl+r` or the 1s
      comment-poll; doesn't notice a file changing on disk on its own.
- [ ] **Multi-line comment ranges.** Comments anchor to a single line only.
- [ ] **Syntax highlighting.** Diff lines are colored by +/-/context only,
      no language-aware highlighting.
- [ ] **Intraline (word-level) diff highlighting.** No highlighting of what
      specifically changed within a modified line (what GitHub/Hunk/difit
      all do) — only whole-line +/- coloring.
- [ ] **Mouse support.** At least click-to-select in the sidebar. Was
      "explicitly not planned" before; came up, so it's queued now.
- [ ] **Show/hide untracked/new files.** Needs scoping first — `rv` only
      ever diffs `git diff <spec>`, which never includes untracked files at
      all (git's own behavior), so "new files" today means files git tracks
      as newly-added in the diff (status `A`), not untracked working-tree
      files. Clarify which is actually wanted before building a toggle.
- [ ] **Fragile comment re-anchoring.** Comments match purely by file + line
      number, nothing else (no content hash, no context). If the commented
      line stops appearing at that position (edit reverted, or unrelated
      lines shifted above it), the comment either silently disappears from
      the diff pane (still present in the session file and in
      `rv comment list`, just not shown attached to anything) or — worse —
      silently reattaches to a different, unrelated line that happens to
      land at the same number. No orphaned-comment indicator, no
      content-based matching. Worth fixing if this bites in practice.
