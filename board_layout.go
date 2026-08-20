package main

import "fyne.io/fyne/v2"

// The board's day columns and hour rows are meant to fill whatever space the
// window gives them, not sit at a size computed once and left to be
// scrolled. Fyne's box/border layouts only ever size children to their own
// MinSize, so getting real resize behavior means implementing fyne.Layout
// directly: Fyne calls Layout() automatically on every resize, and these
// recompute every child's pixel position from day/hour coordinates rather
// than from a fixed pixel table.

// dayHeaderLayout arranges day-name labels into len(weekDays) equal columns
// to the right of a fixed-width gutter, so the header always lines up with
// the grid's day columns beneath it regardless of window width.
type dayHeaderLayout struct {
	gutter float32
	days   map[fyne.CanvasObject]int // label -> day index 0..len(weekDays)-1
}

func (d *dayHeaderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	dayW := dayColumnWidth(size.Width, d.gutter)
	for _, o := range objects {
		day, ok := d.days[o]
		if !ok {
			continue
		}
		o.Move(fyne.NewPos(d.gutter+float32(day)*dayW, 0))
		o.Resize(fyne.NewSize(dayW, size.Height))
	}
}

func (d *dayHeaderLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(d.gutter+float32(len(weekDays))*boardMinDayWidth, boardHeaderHeight)
}

// dayColumnWidth divides the space right of the gutter into len(weekDays)
// equal columns, filling whatever width the window actually has. Keeping
// the window itself a reasonable size is preferredWindowSize's job (using
// boardMaxDayWidth as its target); the grid's own job, once it has a
// container to fill, is to fill it — capping this too just leaves a dead
// margin if the window ends up bigger than that target for any reason.
func dayColumnWidth(totalWidth, gutter float32) float32 {
	return (totalWidth - gutter) / float32(len(weekDays))
}

type gridItemKind int

const (
	kindDayLine gridItemKind = iota
	kindHourLine
	kindHourLabel
	kindDayHighlight
	kindNowLine
	kindBlock
)

// gridItem places one child in day/hour coordinates instead of pixels:
// day is a column index (0..len(weekDays)), or a column boundary for
// kindDayLine; hourFrom/hourTo are clock hours within the visible range.
type gridItem struct {
	kind             gridItemKind
	day              float64
	hourFrom, hourTo float64
}

// boardGridLayout is the schedule grid itself: hour gridlines and labels in
// a fixed gutter, day columns and everything placed within them (the today
// wash, the now line, launch blocks) scaling to fill the container.
type boardGridLayout struct {
	gutter             float32
	startHour, endHour float64
	items              map[fyne.CanvasObject]gridItem
}

func (b *boardGridLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	dayW := dayColumnWidth(size.Width, b.gutter)
	contentW := dayW * float32(len(weekDays))
	// Fills the full container height — see dayColumnWidth's comment on why
	// the grid itself doesn't cap, only preferredWindowSize does.
	contentH := size.Height

	yFor := func(hour float64) float32 {
		return float32((hour-b.startHour)/(b.endHour-b.startHour)) * contentH
	}

	for _, o := range objects {
		it, ok := b.items[o]
		if !ok {
			continue
		}
		switch it.kind {
		case kindDayLine:
			o.Move(fyne.NewPos(b.gutter+float32(it.day)*dayW, 0))
			o.Resize(fyne.NewSize(1, contentH))
		case kindHourLine:
			o.Move(fyne.NewPos(b.gutter, yFor(it.hourFrom)))
			o.Resize(fyne.NewSize(contentW, 1))
		case kindHourLabel:
			o.Move(fyne.NewPos(2, yFor(it.hourFrom)+2))
			o.Resize(fyne.NewSize(b.gutter-4, 14))
		case kindDayHighlight:
			o.Move(fyne.NewPos(b.gutter+float32(it.day)*dayW, 0))
			o.Resize(fyne.NewSize(dayW, contentH))
		case kindNowLine:
			// Only today's column — this marks where "now" falls within one
			// day, not a claim about every day at once.
			o.Move(fyne.NewPos(b.gutter+float32(it.day)*dayW, yFor(it.hourFrom)))
			o.Resize(fyne.NewSize(dayW, 2))
		case kindBlock:
			y0 := yFor(it.hourFrom)
			y1 := yFor(it.hourTo)
			o.Move(fyne.NewPos(b.gutter+float32(it.day)*dayW, y0))
			o.Resize(fyne.NewSize(dayW, y1-y0))
		}
	}
}

func (b *boardGridLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(
		b.gutter+float32(len(weekDays))*boardMinDayWidth,
		float32(b.endHour-b.startHour)*boardMinHourHeight,
	)
}
