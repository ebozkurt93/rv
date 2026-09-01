package main

import tea "github.com/charmbracelet/bubbletea"

type mode int

const (
	modeNormal mode = iota
	modeComment
)

// rowKind distinguishes the two kinds of row the diff pane can show.
type rowKind int

const (
	rowHunkHeader rowKind = iota
	rowLine
)

// diffRow is one flattened, cursor-addressable row of a file's diff: either
// a hunk header (not commentable) or a Line (commentable).
type diffRow struct {
	kind       rowKind
	hunkHeader string
	line       Line
}

// fileRows is a file's diff flattened into rows, precomputed once per file
// so cursor movement is just index arithmetic.
type fileRows struct {
	file FileDiff
	rows []diffRow
}

func flattenFile(fd FileDiff) fileRows {
	fr := fileRows{file: fd}
	for _, h := range fd.Hunks {
		fr.rows = append(fr.rows, diffRow{kind: rowHunkHeader, hunkHeader: h.Header})
		for _, l := range h.Lines {
			fr.rows = append(fr.rows, diffRow{kind: rowLine, line: l})
		}
	}
	return fr
}

type model struct {
	repoRoot string
	files    []fileRows
	session  Session

	width, height int

	fileIndex int
	lineIndex int // index into files[fileIndex].rows
	scroll    int

	pendingG bool // mid-"gg" chord

	mode  mode
	input string

	status string
	err    error

	keys Keymap
}

func newModel(repoRoot string, diffFiles []FileDiff, session Session) model {
	files := make([]fileRows, 0, len(diffFiles))
	for _, fd := range diffFiles {
		files = append(files, flattenFile(fd))
	}
	return model{
		repoRoot: repoRoot,
		files:    files,
		session:  session,
		keys:     defaultKeymap(),
	}
}

func (m model) Init() tea.Cmd { return nil }

// currentRows returns the rows for the currently selected file, or nil if
// there are no changed files.
func (m model) currentRows() []diffRow {
	if m.fileIndex < 0 || m.fileIndex >= len(m.files) {
		return nil
	}
	return m.files[m.fileIndex].rows
}

// currentLine returns the Line under the cursor, if the cursor is on a
// commentable row.
func (m model) currentLine() (Line, bool) {
	rows := m.currentRows()
	if m.lineIndex < 0 || m.lineIndex >= len(rows) {
		return Line{}, false
	}
	row := rows[m.lineIndex]
	if row.kind != rowLine {
		return Line{}, false
	}
	return row.line, true
}

// commentsForCurrentLine returns comments anchored to the cursor's current
// line (matched by file + whichever of old/new line the comment carries).
func (m model) commentsForCurrentLine() []Comment {
	line, ok := m.currentLine()
	if !ok {
		return nil
	}
	return commentsOnLine(m.session.Comments, m.files[m.fileIndex].file.Path, line)
}

// commentsOnLine returns comments in comments anchored to line within file.
// A comment matches on new-line number if it has one, otherwise on
// old-line number (mirroring how a comment is created in
// (*model).addCommentUnderCursor).
func commentsOnLine(comments []Comment, file string, line Line) []Comment {
	var out []Comment
	for _, c := range comments {
		if c.File != file {
			continue
		}
		if line.NewLine != nil && c.NewLine != nil && *line.NewLine == *c.NewLine {
			out = append(out, c)
		} else if line.OldLine != nil && c.OldLine != nil && line.NewLine == nil && c.NewLine == nil && *line.OldLine == *c.OldLine {
			out = append(out, c)
		}
	}
	return out
}
