package main

import tea "github.com/charmbracelet/bubbletea"

// headerHeight is how many lines renderHeader actually produces — 2 (title
// + rule) with no files loaded, 3 (title + rule + selected-path) once
// there's a sidebar to have a selection in. Mouse hit-testing needs this to
// know where the body (sidebar/diff) starts on screen.
func (m model) headerHeight() int {
	if len(m.files) == 0 {
		return 2
	}
	return 3
}

// handleMouse is the whole of rv's mouse support: click a sidebar row to
// select that file, or scroll the wheel over either pane to move through
// it — over the sidebar that means changing the selected file (there's no
// independent sidebar scroll state; the visible window is always derived
// from the selection), over the diff pane it moves the cursor a few lines
// at a time. Ignored outside modeNormal so a stray click while a
// prompt/overlay is open can't do something unexpected.
func (m *model) handleMouse(msg tea.MouseMsg) {
	if len(m.files) == 0 {
		return
	}

	overSidebar := !m.sidebarHidden && msg.X < sidebarWidth

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if overSidebar {
			m.selectVisibleFile(-1)
		} else {
			m.moveCursor(-3)
		}
		return
	case tea.MouseButtonWheelDown:
		if overSidebar {
			m.selectVisibleFile(1)
		} else {
			m.moveCursor(3)
		}
		return
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return
	}
	if overSidebar {
		m.clickSidebarRow(msg.Y)
	} else {
		m.clickDiffRow(msg.Y)
	}
}

// clickDiffRow maps a screen Y coordinate to a row in the current file and
// focuses it, by rebuilding the exact same line list + scroll renderDiff
// drew the frame with (see buildDiffLines) rather than assuming one
// rendered line always equals one row — wrap mode breaks that assumption,
// since one row can span several rendered lines.
func (m *model) clickDiffRow(y int) {
	innerW := m.width - m.diffPaneSidebarWidth() - borderOverheadW
	if innerW < 1 {
		innerW = 1
	}
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}

	lines, cursorLine, rowFor := m.buildDiffLines(innerW)
	scroll := clampScroll(cursorLine, len(lines), innerH)

	row := y - m.headerHeight() - 1 // +1 for the diff panel's own top border row
	if row < 0 || row >= innerH {
		return
	}
	idx := scroll + row
	if idx < 0 || idx >= len(rowFor) {
		return
	}

	rows := m.currentRows()
	target := rowFor[idx]
	if target < 0 || target >= len(rows) {
		return
	}
	// A click can land on a hunk-header line (or, in wrap mode, on a
	// comment/reply line whose row happens to be one) — nearestContentRow
	// keeps the cursor invariant (never on a header) instead of ignoring
	// the click outright.
	m.lineIndex = nearestContentRow(rows, target)
}

// clickSidebarRow maps a screen Y coordinate to a sidebar row, replaying
// the exact same scroll math renderSidebar used to draw the frame the user
// is looking at (clampScroll over the same visible-file list), and selects
// whatever file landed there.
func (m *model) clickSidebarRow(y int) {
	innerH := m.bodyHeight() - borderOverheadH
	if innerH < 1 {
		innerH = 1
	}
	// +1 for the sidebar panel's own top border row.
	row := y - m.headerHeight() - 1
	if row < 0 || row >= innerH {
		return
	}

	vis := m.visibleFileIndices(m.fileFilter)
	if len(vis) == 0 {
		return
	}
	cursorPos := 0
	for i, v := range vis {
		if v == m.fileIndex {
			cursorPos = i
			break
		}
	}
	scroll := clampScroll(cursorPos, len(vis), innerH)
	clicked := scroll + row
	if clicked < 0 || clicked >= len(vis) {
		return
	}
	m.selectFile(vis[clicked])
}
