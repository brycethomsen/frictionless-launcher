package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// scheduleBlock is one launch window on the board — a custom widget rather
// than a widget.Button wrapping a card, for two reasons that both come back
// to the same thing: a plain button doesn't give enough control over what
// happens on hover.
//
//  1. A ghost button still paints a hover-fill on itself, and since it was
//     the tap target stacked on top of the block's own text, hovering
//     painted right over the game name and made it unreadable.
//  2. A short block (a 30-60 minute window) isn't tall enough for both the
//     name and the time range without the time visually overlapping the row
//     below it. Rather than shrink text or truncate, the time is hidden by
//     default and only revealed on hover — the name alone is enough to
//     recognize the block at a glance; the exact time is there when you
//     want it.
type scheduleBlock struct {
	widget.BaseWidget
	card     *canvas.Rectangle
	nameText *canvas.Text
	timeText *canvas.Text
	onTapped func()
}

func newScheduleBlockWidget(name, timeRange string, onTapped func()) *scheduleBlock {
	b := &scheduleBlock{onTapped: onTapped}

	b.card = canvas.NewRectangle(withAlpha(colorReady, 0x2a))
	b.card.CornerRadius = 3

	b.nameText = canvas.NewText(name, colorReady)
	b.nameText.TextStyle = fyne.TextStyle{Bold: true}
	b.nameText.TextSize = theme.CaptionTextSize()

	b.timeText = canvas.NewText(timeRange, colorReady)
	b.timeText.TextStyle = fyne.TextStyle{Monospace: true}
	b.timeText.TextSize = theme.CaptionTextSize() - 1
	b.timeText.Hide()

	b.ExtendBaseWidget(b)
	return b
}

func (b *scheduleBlock) CreateRenderer() fyne.WidgetRenderer {
	labels := container.NewPadded(container.NewVBox(b.nameText, b.timeText))
	return widget.NewSimpleRenderer(container.NewStack(b.card, labels))
}

func (b *scheduleBlock) Tapped(*fyne.PointEvent) {
	if b.onTapped != nil {
		b.onTapped()
	}
}

// MouseIn reveals the time range and gives the card a touch more presence,
// both cheap to do correctly now that this widget owns its own rendering
// instead of layering a button's opaque hover state over the text.
func (b *scheduleBlock) MouseIn(*desktop.MouseEvent) {
	b.timeText.Show()
	b.card.FillColor = withAlpha(colorReady, 0x40)
	b.card.Refresh()
	b.Refresh()
}

func (b *scheduleBlock) MouseMoved(*desktop.MouseEvent) {}

func (b *scheduleBlock) MouseOut() {
	b.timeText.Hide()
	b.card.FillColor = withAlpha(colorReady, 0x2a)
	b.card.Refresh()
	b.Refresh()
}
