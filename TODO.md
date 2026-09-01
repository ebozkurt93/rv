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
- [ ] **Config file / remappable keybindings.** `defaultKeymap()` is
      hardcoded; no way to customize.
- [ ] **Watch/auto-reload.** Only reloads on manual `R` or the 1s
      comment-poll; doesn't notice a file changing on disk on its own.
- [ ] **Multi-line comment ranges.** Comments anchor to a single line only.
- [ ] **Syntax highlighting.** Diff lines are colored by +/-/context only,
      no language-aware highlighting.
- [ ] **Intraline (word-level) diff highlighting.** No highlighting of what
      specifically changed within a modified line (what GitHub/Hunk/difit
      all do) — only whole-line +/- coloring.

Explicitly not planned unless it comes up: mouse support.
