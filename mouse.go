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
	}
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
