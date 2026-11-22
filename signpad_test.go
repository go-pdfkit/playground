package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-gfx/gfx/raster"
	"github.com/go-pdfkit/render"
	"github.com/go-widgets/toolkit"
)

// ⛔ THE POINT OF THE PAD IS MEASURED ON THE PAGE, not in the pad's own list of
// strokes: a signature that reached the file at the wrong size or the wrong
// shape would leave the pad looking exactly as it does now. The pad itself is
// toolkit.InkPad and tested there; what is tested here is the wiring.

// penSpan is how wide and how tall the signature ink is on a rendered page, and
// how many pixels of it there are. The sample page is black on white, so a
// pixel that is blue rather than grey can only be the pen's.
func penSpan(img *raster.Image) (wide, tall, count int) {
	minX, maxX, minY, maxY := img.W, -1, img.H, -1
	for y := 0; y < img.H; y++ {
		for x := 0; x < img.W; x++ {
			i := img.PixOffset(x, y)
			if int(img.Pix[i+2])-int(img.Pix[i]) < 40 || img.Pix[i] > 120 {
				continue
			}
			count++
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	if count == 0 {
		return 0, 0, 0
	}
	return maxX - minX + 1, maxY - minY + 1, count
}

// scribble draws a diagonal stroke across the pad the way a pointer does: a
// press, a run of drags, a release, all through the workbench's own pointer
// routing at the pad's place on the surface.
func scribble(t *testing.T, s *state) {
	t.Helper()
	b := s.tools.pad.Bounds()
	if b.W == 0 {
		t.Fatal("the pad has no bounds, so nothing can be drawn on it")
	}
	s.handleClick(b.X+20, b.Y+10)
	for i := 1; i <= 10; i++ {
		s.handleMove(b.X+20+i*20, b.Y+10+i*8)
	}
	s.handleRelease(b.X+220, b.Y+90)
}

// rendered is page 1 of the saved document, at one pixel to the point.
func rendered(t *testing.T, s *state) *raster.Image { return renderedPage(t, s, 1) }

// renderedPage is page n of the saved document, at one pixel to the point.
func renderedPage(t *testing.T, s *state, n int) *raster.Image {
	t.Helper()
	src, msg := s.reopen()
	if msg != "" {
		t.Fatal(msg)
	}
	img, err := render.Page(src, n, render.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// penCentre is where the middle of the signature ink is on a page, as a share
// of the page's width and height, with the ink's own width and height.
func penCentre(img *raster.Image) (cx, cy float64, wide, tall int) {
	minX, maxX, minY, maxY := img.W, -1, img.H, -1
	for y := 0; y < img.H; y++ {
		for x := 0; x < img.W; x++ {
			i := img.PixOffset(x, y)
			if int(img.Pix[i+2])-int(img.Pix[i]) < 40 || img.Pix[i] > 120 {
				continue
			}
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	if maxX < 0 {
		return 0, 0, 0, 0
	}
	return float64(minX+maxX) / 2 / float64(img.W), float64(minY+maxY) / 2 / float64(img.H),
		maxX - minX + 1, maxY - minY + 1
}

func TestASignatureLandsOnTheNamedPageOnlyAndWhereItWasAskedFor(t *testing.T) {
	for _, c := range []struct {
		place         string
		right, bottom bool
	}{
		{"bottom right", true, true},
		{"top left", false, false},
	} {
		t.Run(c.place, func(t *testing.T) {
			s, _ := opened(t, 3)
			openGroup(t, s, groupSign)
			at := -1
			for i, n := range placeNames() {
				if n == c.place {
					at = i
				}
			}
			if at < 0 {
				t.Fatalf("no place is called %q", c.place)
			}
			s.tools.at.Set(at)
			s.tools.markSpec.Set("2")
			s.tools.wide.Set(100)
			scribble(t, s)
			pressVerb(t, s, "signPlace")
			for _, n := range []int{1, 3} {
				if _, _, w, _ := penCentre(renderedPage(t, s, n)); w != 0 {
					t.Errorf("page %d, which was not named, has signature ink %d wide", n, w)
				}
			}
			cx, cy, wide, tall := penCentre(renderedPage(t, s, 2))
			if wide == 0 {
				t.Fatal("page 2 has no signature ink")
			}
			if wide < 94 || wide > 101 || tall < 36 || tall > 44 {
				t.Errorf("the signature is %d by %d, not about 100 by 40 (the 0.4 it was drawn at)", wide, tall)
			}
			if (cx > 0.5) != c.right || (cy > 0.5) != c.bottom {
				t.Errorf("%s: the ink is centred at %.2f, %.2f of the page", c.place, cx, cy)
			}
		})
	}
}

func TestAnEmptyWhichPagesPlacesTheSignatureOnEveryPageAndSaysSo(t *testing.T) {
	s, _ := opened(t, 3)
	openGroup(t, s, groupSign)
	s.tools.markSpec.Set("")
	scribble(t, s)
	pressVerb(t, s, "signPlace")
	if !strings.Contains(s.note, "on all") {
		t.Errorf("the status line says %q, not that it went on all pages", s.note)
	}
	for n := 1; n <= 3; n++ {
		if _, _, w, _ := penCentre(renderedPage(t, s, n)); w == 0 {
			t.Errorf("page %d has no signature ink", n)
		}
	}
}

func TestADrawnSignatureLandsOnThePageAtTheWidthAskedForAndKeepsItsShape(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	s.tools.wide.Set(144)
	scribble(t, s)
	if got := s.tools.pad.Strokes().Len(); got != 1 || s.tools.pad.PointerGrabbed() {
		t.Fatalf("after a press, a drag and a release the pad holds %d strokes, grabbed %v",
			got, s.tools.pad.PointerGrabbed())
	}
	pressVerb(t, s, "signPlace")
	if !strings.Contains(s.note, "signed your signature") {
		t.Fatalf("the status line says %q", s.note)
	}
	wide, tall, count := penSpan(rendered(t, s))
	if count == 0 {
		t.Fatal("no ink reached the page")
	}
	// The picture is cropped to the ink, so the width chosen is the signature's
	// own, less the margin each end.
	if wide < 136 || wide > 145 {
		t.Errorf("the signature is %d points wide, not about the 144 asked for", wide)
	}
	// ⛔ AND ITS PROPORTIONS ARE THE DRAWING'S: the stroke ran 200 across and 80
	// down, so one 144 wide is about 58 tall.
	if tall < 50 || tall > 64 {
		t.Errorf("the signature is %d points tall, which is not the 0.4 of its width it was drawn at", tall)
	}
}

func TestASignatureFollowsTheWidthBesideIt(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	scribble(t, s)
	spinFor(t, s.tools.built[groupSign], "Signature width").SetValue(72)
	pressVerb(t, s, "signPlace")
	wide, _, count := penSpan(rendered(t, s))
	if count == 0 {
		t.Fatal("no ink reached the page")
	}
	if wide < 66 || wide > 73 {
		t.Errorf("the signature is %d points wide, not about the 72 asked for", wide)
	}
}

func TestThePadsControlsAreFoundByTheirLabels(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	found := map[string]bool{}
	var walk func(toolkit.Widget)
	walk = func(w toolkit.Widget) {
		if f, ok := w.(*toolkit.FormField); ok {
			found[f.Label] = true
		}
		if c, ok := w.(kids); ok {
			for _, k := range c.Children() {
				walk(k)
			}
		}
	}
	walk(s.tools.built[groupSign])
	for _, want := range []string{"Draw your signature here", "Where it goes", "Signature width"} {
		if !found[want] {
			t.Errorf("no row on the Sign panel is labelled %q; there are %v", want, found)
		}
	}
	for _, id := range []string{"signPlace", "signUndo", "signClear"} {
		if _, shown := s.ribbon.ControlRect(verbs[id].label); !shown {
			t.Errorf("%q is not on the ribbon", verbs[id].label)
		}
	}
}

func TestPlacingWithNothingDrawnIsRefusedInPlain(t *testing.T) {
	s, _ := opened(t, 1)
	pressVerb(t, s, "signPlace")
	if !strings.Contains(s.note, "draw your signature in the box first") {
		t.Errorf("the status line says %q", s.note)
	}
	if _, _, count := penSpan(rendered(t, s)); count != 0 {
		t.Error("a refused placement left ink on the page")
	}
}

func TestPlacingWithNoDocumentOpenSaysSo(t *testing.T) {
	s := newState(surfaceW, surfaceH, &fakeHost{})
	s.signPlace()
	if !strings.Contains(s.note, "open a document first") {
		t.Errorf("the status line says %q", s.note)
	}
}

func TestPlacingWhenThePictureFailsSaysWhy(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	scribble(t, s)
	old := padPNG
	defer func() { padPNG = old }()
	padPNG = func(*toolkit.InkPad) ([]byte, error) { return nil, errors.New("disk full") }
	s.signPlace()
	if !strings.Contains(s.note, "disk full") {
		t.Errorf("the status line says %q", s.note)
	}
}

func TestUndoTakesBackOneStrokeAndClearTakesAll(t *testing.T) {
	s, _ := opened(t, 1)
	s.signUndo()
	if !strings.Contains(s.note, "nothing has been drawn") {
		t.Errorf("undo on an empty pad said %q", s.note)
	}
	openGroup(t, s, groupSign)
	for i := 0; i < 3; i++ {
		scribble(t, s)
	}
	p := s.tools.pad
	if p.Strokes().Len() != 3 {
		t.Fatalf("three scribbles are %d strokes", p.Strokes().Len())
	}
	pressVerb(t, s, "signUndo")
	if p.Strokes().Len() != 2 || s.note != "took back the last stroke" {
		t.Errorf("after one undo: %d strokes, status %q", p.Strokes().Len(), s.note)
	}
	pressVerb(t, s, "signClear")
	if !p.Empty().Get() || s.note != "cleared the pad" {
		t.Errorf("after clear: empty %v, status %q", p.Empty().Get(), s.note)
	}
}

func TestUndoAndClearWorkWithNoDocumentOpen(t *testing.T) {
	// The pad is a drawing board before it is anything else: its two edits do
	// not need a document, and a greyed Undo would strand a bad first stroke.
	s := newState(surfaceW, surfaceH, &fakeHost{})
	s.tools.pad.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 5})
	s.tools.pad.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp})
	s.signUndo()
	if !s.tools.pad.Empty().Get() {
		t.Error("undo did not take the stroke back")
	}
}

func TestUndoAndClearAreEnabledExactlyWhilePadHoldsInk(t *testing.T) {
	// ⛔ Bound to the pad's Empty observable, so a stroke enables them without
	// any refresh of the document: with a document open and with none.
	for _, withDoc := range []bool{true, false} {
		var s *state
		if withDoc {
			s, _ = opened(t, 1)
		} else {
			s = newState(surfaceW, surfaceH, &fakeHost{})
		}
		on := func() (bool, bool) {
			return s.acts["signUndo"].Enabled.Get(), s.acts["signClear"].Enabled.Get()
		}
		if u, c := on(); u || c {
			t.Errorf("doc %v: on an empty pad Undo is %v and Clear is %v", withDoc, u, c)
		}
		s.tools.pad.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 5})
		s.tools.pad.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp})
		if u, c := on(); !u || !c {
			t.Errorf("doc %v: after a stroke Undo is %v and Clear is %v", withDoc, u, c)
		}
		s.signUndo()
		if u, c := on(); u || c {
			t.Errorf("doc %v: after undoing it Undo is %v and Clear is %v", withDoc, u, c)
		}
	}
}

func TestThePadSaysOnceOnTheStatusLineWhyItStoppedTakingInk(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	b := s.tools.pad.Bounds()
	for i := 0; i < toolkit.InkMaxStrokes; i++ {
		s.handleClick(b.X+5, b.Y+5)
		s.handleRelease(b.X+5, b.Y+5)
	}
	s.note = ""
	s.handleClick(b.X+5, b.Y+5)
	s.handleRelease(b.X+5, b.Y+5)
	if !strings.Contains(s.note, "as many strokes") || s.tools.pad.Strokes().Len() != toolkit.InkMaxStrokes {
		t.Errorf("past the stroke cap the status says %q with %d strokes", s.note, s.tools.pad.Strokes().Len())
	}
	s.signClear()
	// One long stroke, 200 pixels from the last point every time, until the
	// point budget is spent.
	s.handleClick(b.X+5, b.Y+5)
	for i := 0; i < toolkit.InkMaxPoints+10; i++ {
		s.handleMove(b.X+5+(i%2)*200, b.Y+50)
	}
	s.handleRelease(b.X+5, b.Y+50)
	if !strings.Contains(s.note, "full of ink") {
		t.Errorf("past the point cap the status says %q", s.note)
	}
}

func TestTheInkIsOnScreenWhereItWasDrawn(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	scribble(t, s)
	buf := buffer()
	s.draw(buf)
	b := s.tools.pad.Bounds()
	in := 0
	for y := b.Y; y < b.Y+b.H; y++ {
		for x := b.X; x < b.X+b.W; x++ {
			i := (y*surfaceW + x) * 4
			if buf[i+2] >= 60 && int(buf[i+2])-int(buf[i]) >= 40 && buf[i] <= 90 {
				in++
			}
		}
	}
	if in < 100 {
		t.Errorf("only %d pen-coloured pixels are inside the pad", in)
	}
}

func TestADragLeavingThePadStillDrawsAndTheReleaseEndsIt(t *testing.T) {
	// The scroll view around the panel used to keep the drag for itself; the
	// pad now holds the pointer, and a stroke that wanders off the box is held
	// at its edge and ended by a release wherever it happens.
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	b := s.tools.pad.Bounds()
	s.handleClick(b.X+10, b.Y+10)
	s.handleMove(b.X+10, b.Y+b.H+30)
	s.handleRelease(b.X+10, b.Y+b.H+30)
	line := s.tools.pad.Strokes().At(0)
	if last := line[len(line)-1]; last.Y != 1 {
		t.Errorf("a stroke dragged below the box ends at %v, not on its bottom edge", last)
	}
	if s.tools.pad.PointerGrabbed() || s.pressing {
		t.Errorf("after the release grabbed %v, pressing %v", s.tools.pad.PointerGrabbed(), s.pressing)
	}
}

func TestAPressElsewhereDoesNotBeginAStroke(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	s.handleClick(2, 2)
	s.handleRelease(2, 2)
	if !s.tools.pad.Empty().Get() {
		t.Error("a press on the ribbon's edge began a stroke")
	}
}

func TestWhatWasDrawnSurvivesAVisitToAnotherTab(t *testing.T) {
	s, _ := opened(t, 1)
	openGroup(t, s, groupSign)
	scribble(t, s)
	openGroup(t, s, groupStamp)
	openGroup(t, s, groupSign)
	if s.tools.pad.Empty().Get() {
		t.Error("the stroke was gone after leaving the tab and coming back")
	}
}
