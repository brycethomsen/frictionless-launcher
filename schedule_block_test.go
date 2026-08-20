package main

import "testing"

func TestScheduleBlock_TimeHiddenUntilHover(t *testing.T) {
	b := newScheduleBlockWidget("Stardew Valley", "19:00-21:00", func() {})

	if b.timeText.Visible() {
		t.Error("expected time text hidden by default, so a short block's text can't overlap the row below it")
	}

	b.MouseIn(nil)
	if !b.timeText.Visible() {
		t.Error("expected time text visible after MouseIn")
	}

	b.MouseOut()
	if b.timeText.Visible() {
		t.Error("expected time text hidden again after MouseOut")
	}
}

func TestScheduleBlock_Tapped_InvokesCallback(t *testing.T) {
	tapped := false
	b := newScheduleBlockWidget("Stardew Valley", "19:00-21:00", func() { tapped = true })

	b.Tapped(nil)

	if !tapped {
		t.Error("expected Tapped to invoke the onTapped callback")
	}
}
