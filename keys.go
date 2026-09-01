package main

import tea "charm.land/bubbletea/v2"

// Keymap follows vim conventions throughout: j/k for line movement, gg/G for
// jump-to-top/bottom, and single-letter mnemonics for actions (c)omment,
// (d)elete, (r)esolve, (e)xport — chosen to match tmux-mover's Keymap-of-
// key-lists shape (keys.go here plays the same role as its keys.go).
type Keymap struct {
	Quit               []string
	MoveDown           []string
	MoveUp             []string
	HalfPageDown       []string
	HalfPageUp         []string
	NextFile           []string
	PrevFile           []string
	NextHunk           []string
	PrevHunk           []string
	NextComment        []string
	PrevComment        []string
	Top                []string // second half of "gg"
	Bottom             []string
	AddComment         []string
	DeleteComment      []string
	ToggleResolved     []string
	ToggleReviewed     []string
	OpenEditor         []string
	Refresh            []string
	Export             []string
	CopyMarkdown       []string
	CopyJSON           []string
	Help               []string
	Search             []string
	ToggleSidebar      []string
	ToggleLineNumbers  []string
	ToggleWrap         []string
	Confirm            []string
	Cancel             []string
	Backspace          []string
	CommentNewline     []string // insert a newline while composing a comment, instead of submitting
	CommentEditor      []string // edit the in-progress comment body in $EDITOR
	ToggleCommentScope []string // n/N include resolved comments too, instead of skipping them
	ToggleUntracked    []string // show/hide files git isn't tracking at all
	ClearSession       []string // wipe every comment and reviewed mark for this repo's session (asks to confirm)
}

func defaultKeymap() Keymap {
	return Keymap{
		Quit:               []string{"q", "ctrl+c"},
		MoveDown:           []string{"j", "down"},
		MoveUp:             []string{"k", "up"},
		HalfPageDown:       []string{"ctrl+d"},
		HalfPageUp:         []string{"ctrl+u"},
		NextFile:           []string{"tab", "]"},
		PrevFile:           []string{"shift+tab", "["},
		NextHunk:           []string{"}"},
		PrevHunk:           []string{"{"},
		NextComment:        []string{"n"},
		PrevComment:        []string{"N"},
		Top:                []string{"g"},
		Bottom:             []string{"G"},
		AddComment:         []string{"c"},
		DeleteComment:      []string{"d"},
		ToggleResolved:     []string{"r"},
		ToggleReviewed:     []string{"v"},
		OpenEditor:         []string{"o"},
		Refresh:            []string{"ctrl+r"},
		Export:             []string{"e"},
		CopyMarkdown:       []string{"y"},
		CopyJSON:           []string{"Y"},
		Help:               []string{"?"},
		Search:             []string{"/"},
		ToggleSidebar:      []string{"t"},
		ToggleLineNumbers:  []string{"#"},
		ToggleWrap:         []string{"w"},
		Confirm:            []string{"enter"},
		Cancel:             []string{"esc"},
		Backspace:          []string{"backspace"},
		CommentNewline:     []string{"alt+enter"},
		CommentEditor:      []string{"alt+e"},
		ToggleCommentScope: []string{"C"},
		ToggleUntracked:    []string{"u"},
		ClearSession:       []string{"D"},
	}
}

func keyMatches(msg tea.KeyMsg, keys []string) bool {
	value := msg.String()
	for _, key := range keys {
		if value == key {
			return true
		}
	}
	return false
}
