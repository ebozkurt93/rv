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
- [x] **Hunk-level jump.** `}`/`{` jump to the next/prev hunk, landing on
      its first line (never the header — you can't comment there, so
      stopping on it just cost an extra keypress). Respects a count (`3}`).
      Runs off the current file into the next/prev *visible* file once it's
      out of hunks, wrapping around at either end, rather than stopping at
      the file boundary.
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
- [x] **Cursor never rests on a hunk header.** Was reachable via file-open,
      `gg`/`G` (including `{count}gg`/`{count}G` landing exactly on a
      header's row index), and even plain `j`/`k` stepping through one —
      not just `}`/`{`. Every cursor-setting path now funnels through
      shared first/last/nearest-content-row helpers so the invariant holds
      everywhere, locked in with a dedicated test file.
- [x] **Mark a file reviewed.** `v` toggles a per-file reviewed mark
      (`Session.Reviewed`, keyed by file path → a content hash of that
      file's diff). If the file's diff changes at all afterward, it reads
      as unreviewed again automatically — the hash just won't match, no
      explicit invalidation needed. Shown as a dimmed path + `✓` in the
      sidebar, and an "N/M files reviewed" count in the header.
- [ ] **Watch/auto-reload.** Not needed for now — deprioritized. Only
      reloads on manual `ctrl+r` or the 1s comment-poll; doesn't notice a
      file changing on disk on its own.
- [ ] **Multi-line comment ranges.** Comments anchor to a single line only.
- [x] **Syntax highlighting.** Diff lines run through chroma (lexer picked
      per file, cached on fileRows). Token foreground colors still reduce to
      the basic 16 ANSI colors (`nearestANSI16`, same Lab-distance metric as
      `formatters.TTY16`) so they follow the terminal theme like the rest of
      `rv`. Added/removed/cursor row backgrounds are a fixed, subtle
      truecolor tint instead (`tintedFormatter`, baked per-token like the
      foreground — not an outer wrap, since chroma resets after every
      token) — the basic 16-color palette has no "dark green," only fully
      saturated "green," so reducing backgrounds to it the same way as
      foregrounds made every added/removed line look like a neon
      highlighter. Dark/light tint set is picked via a cheap `$COLORFGBG`
      guess rather than a live terminal background query, which can hang
      for seconds (`OSCTimeout`) on a terminal that never answers it. The
      gutter's line numbers carry the same tint and color as the +/- prefix
      (not just the code), the tint's raw truecolor escape is built by hand
      for the gutter/prefix instead of via `lipgloss.Style.Background` (which
      downgrades a hex color to the nearest ANSI-16 shade under a
      non-truecolor profile, visibly mismatching the syntax-highlighted
      text's own untouched truecolor tint), and a token whose own color
      nearly matches the tint (e.g. a comment) falls back to a contrast-safe
      black/white instead of rendering as next-to-invisible text. The diff
      pane draws its own border by hand (`renderBorderedRaw`) instead of
      `lipgloss.Style.Border(...).Padding(...).Render(...)`, since lipgloss's
      layout engine reprocesses embedded ANSI when computing per-line
      padding and silently drops truecolor backgrounds it doesn't recognize
      as its own styling. Tabs in diff content are expanded to spaces before
      any width-sensitive rendering (`ansi.StringWidth`/`ansi.Truncate`
      count a literal tab as 0-1 cells, not the terminal's actual tab-stop
      jump), which was causing the border to visibly shift/misalign on
      tab-indented lines once wrap was on.
- [ ] **Intraline (word-level) diff highlighting.** No highlighting of what
      specifically changed within a modified line (what GitHub/Hunk/difit
      all do) — only whole-line +/- coloring.
- [x] **Mouse support.** Click a sidebar row to select that file (hit-tests
      through the exact same scroll math renderSidebar drew the frame
      with, so it's correct even when the list has scrolled). Wheel scroll
      over the sidebar changes the selected file; over the diff pane it
      moves the cursor a few lines at a time. Ignored outside modeNormal —
      a stray click while an overlay/prompt is open does nothing.
- [x] **Show/hide untracked/new files.** `u` toggles files git isn't
      tracking at all (`git ls-files --others --exclude-standard`,
      read-only), synthesized as an all-added FileDiff — hidden by default,
      persisted like the other display toggles. Marked "?" in the sidebar,
      distinct from a tracked file's own "A" status. A binary untracked
      file shows a placeholder instead of dumping raw bytes as added lines.
- [x] **Fragile comment re-anchoring.** Comment now records the exact line
      content at creation time (`LineContent`). `commentAnchorRow` prefers
      the original file+line-number position if its content still matches
      there, falls back to wherever in the file that content now uniquely
      appears (an ambiguous or absent match orphans the comment rather than
      guessing wrong), and falls back further to pure line-number matching
      for comments predating this (empty `LineContent`). Orphaned comments
      show as a count in the header instead of silently vanishing.
- [ ] **Split (side-by-side) view vs. current stacked/unified view.** Real
      rendering feature, not a small toggle: needs old/new lines paired into
      columns, handling unequal add/remove counts per hunk, a width
      recalculation per column, and reworking wrap + the mouse click-to-
      focus row mapping (both currently assume one unified column). Wanted;
      not started — needs its own scoped pass rather than being folded into
      something else.
- [x] **Comment editor shouldn't be a bordered box.** renderCommentEditor
      now renders as a plain "  ▏✎ ..." inline row matching
      renderComment/renderReply's own indent convention, no border.
- [x] **`?` help overlay needs to scroll.** `j`/`k`/`ctrl+d`/`ctrl+u` scroll
      a window over the flattened help lines (`helpScroll`, reset each time
      the overlay opens), clamped at either end — entries past one screen's
      height are reachable instead of silently cut off.
- [x] **Cursor-line highlight only covers the gutter, not the whole line.**
      Fixed as part of syntax highlighting: the cursor row now picks a
      dedicated chroma style (bright-black background baked into every
      token, taking priority over the row's added/removed tint) instead of
      an outer `styleCursor.Render(...)` wrap, and `renderDiff` tints
      `fitLine`'s padding to match via `fitLineWithBackground` so the
      highlight spans the full row width, not just the text.
- [x] **Multi-line comment editing.** `alt+enter` inserts a newline instead
      of submitting (shift+enter isn't reliably detectable across terminals
      without Kitty's keyboard protocol, so it's not bound); `ctrl+e`
      suspends into `$EDITOR` to compose the body there and reads it back;
      renderComment/renderReply/the inline editor all handle multi-line
      bodies now, continuation lines indented under the first. `c` on a line
      with your own unresolved comment now edits it in place instead of
      duplicating it, or adds a new reply if something's already responded
      (editing the body then would silently change what the reply was
      responding to) — only your own comments are offered this way, an
      agent-authored one is left alone.
- [ ] **Session state isn't scoped to the diff being reviewed.** A session
      is keyed only by repo root (`sessionDir`), so two different diff specs
      reviewed from the *same* checkout (not separate `git worktree`s — e.g.
      `rv main..featureA` and later `rv main..featureB` from one directory)
      share one `session.json`. Comments carry over by file+line regardless
      of which branch/spec is actually showing, and could misattach to
      unrelated code. Worth scoping the session key to (repo root, diff
      spec) or similar — needs its own design pass.
