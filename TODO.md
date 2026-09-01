# rv — known gaps

Working list from comparing `rv` against Hunk/difit/tuicr and general git-TUI
conventions (lazygit, tig). Worked through top to bottom; check items off as
they land, with the commit that did it.

- [ ] **Flexible diff target.** Hardcoded to `git diff HEAD` right now — no
      way to review a branch (`rv main..feature`), staged changes only
      (`rv --staged`), or a specific commit. Biggest functional ceiling.
- [ ] **Open in `$EDITOR`.** No way to jump from the line under the cursor
      into an actual editor to make a change while reviewing. Standard in
      lazygit/tig/gh.
- [ ] **Hunk-level jump.** Movement is line-by-line only (`j`/`k`) — no
      `}`/`{` to jump to the next/prev hunk, which gets tedious on large
      diffs.
- [ ] **Cross-file comment navigation.** No way to jump to the next/prev
      unresolved comment regardless of which file it's in — you have to
      manually switch files and scroll.
- [ ] **Search/filter.** No `/` to jump to a file by name or search diff
      content. Sidebar just lists everything with no way to narrow it down.
- [ ] **Help overlay.** No `?` screen — the footer hint line is the only
      reference and it's already crowded with ~10 bindings.
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
