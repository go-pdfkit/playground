// The Sign tab: a box to draw a signature in with the mouse, a pen or a finger,
// and the verb that puts what was drawn on the pages.
//
// The box is toolkit.InkPad. This file only wires it: what to say when it is
// full, and how its picture reaches ops.Place.
//
// ⛔ A DRAWN SIGNATURE IS A PICTURE, NOT CRYPTOGRAPHY. ops.Place says the same
// of itself, and this panel inherits the sentence: what lands on the page is ink
// placed where the reader chose, and nothing in the file proves who drew it. The
// cryptographic kind is `ops.Sign`, a different job with a different name.
//
// ⛔ RASTER, AND THAT IS MEASURED RATHER THAN CHOSEN FOR EASE. ops has no verb
// that places a vector path: its Shape kinds are a line, a rectangle and an
// ellipse, so a hand-drawn curve would be one line annotation per segment. The
// one thing ops DOES place is a picture, which InkPad.PNG draws, cropped to the
// ink on a transparent ground.

package main

import (
	"github.com/go-widgets/toolkit"
)

// padPNG is the pad's picture, replaceable so a test can watch a failed one.
var padPNG = (*toolkit.InkPad).PNG

// newSignPad is the drawing box. When it stops taking ink it says why on the
// status line, once.
func newSignPad(say func(string)) *toolkit.InkPad {
	pad := toolkit.NewInkPad()
	pad.OnFull = func(why toolkit.InkLimit) {
		if why == toolkit.InkStrokesFull {
			say("the pad holds as many strokes as it can — undo one or clear the box")
			return
		}
		say("the pad is full of ink — undo a stroke or clear the box")
	}
	return pad
}

// signRowH is the room the pad's row is given: its caption, and a box about
// as tall as the panel is wide over two and a half.
func signRowH() int { return textH() + labelGap + rowsW*2/5 }

// signGroup is the panel beside the tab that draws a signature and places it.
//
// ⛔ THE PLACE AND THE WIDTH ARE THE STAMP TAB'S, bound to the same two data:
// where a signature goes and how wide it is are the questions a picture asks,
// and a signature IS a picture to the library.
func (s *state) signGroup() *column {
	box := newColumn()
	box.add(toolkit.NewFormField("Draw your signature here", s.tools.pad), signRowH())
	box.add(s.whichPages(), labelledH())
	box.add(buttons(
		s.chooseRow("Where it goes", placeNames(), s.tools.at),
		s.spinRow("Signature width", 12, s.tools.wide),
	), labelledH())
	s.signGroupCert(box)
	return box
}

// signPlace puts the drawn signature on the pages the range names.
func (s *state) signPlace() {
	if s.doc == nil {
		s.fail("open a document first")
		return
	}
	if s.tools.pad.Empty().Get() {
		s.fail("draw your signature in the box first — there is nothing on it to place")
		return
	}
	data, err := padPNG(s.tools.pad)
	if err != nil {
		s.fail(err.Error())
		return
	}
	s.drawPlaced("signed", "your signature", data)
}

// padHasInk is the rule for Undo and Clear: there is something to take back.
// It needs no document, because the pad is a drawing board before it is
// anything else; the pad's own Empty observable says when it changes.
func (s *state) padHasInk() bool { return !s.tools.pad.Empty().Get() }

// signUndo takes back the last stroke.
func (s *state) signUndo() {
	if !s.tools.pad.Undo() {
		s.fail("nothing has been drawn on the pad to undo")
		return
	}
	s.dirty = true
	s.note = "took back the last stroke"
}

// signClear empties the pad.
func (s *state) signClear() {
	s.tools.pad.Clear()
	s.dirty = true
	s.note = "cleared the pad"
}
