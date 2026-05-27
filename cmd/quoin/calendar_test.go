package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/samuel/demy"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestRenderCalendar(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.Local)

	events := []calendarEvent{
		// Today: all-day + timed
		{title: "Memorial Day", start: now, end: now.AddDate(0, 0, 1), allDay: true},
		{title: "Doctor Appointment", start: now.Add(2 * time.Hour), end: now.Add(3 * time.Hour)},
		{title: "Team Standup", start: now.Add(4 * time.Hour), end: now.Add(5 * time.Hour)},
		{title: "Extra Event (should be hidden)", start: now.Add(6 * time.Hour), end: now.Add(7 * time.Hour)},
		// Tomorrow: one event
		{title: "Dentist", start: now.AddDate(0, 0, 1).Add(10 * time.Hour), end: now.AddDate(0, 0, 1).Add(11 * time.Hour)},
		// Day 3: empty (no events)
		// Day 4: two events
		{title: "Family Dinner", start: now.AddDate(0, 0, 3).Add(18 * time.Hour), end: now.AddDate(0, 0, 3).Add(20 * time.Hour)},
		{title: "Movie Night", start: now.AddDate(0, 0, 3).Add(20 * time.Hour), end: now.AddDate(0, 0, 3).Add(22 * time.Hour)},
		// Day 5: all-day
		{title: "Conference", start: now.AddDate(0, 0, 4), end: now.AddDate(0, 0, 5), allDay: true},
		// Day 6: nothing
		// Day 7: one event
		{title: "Weekly Review", start: now.AddDate(0, 0, 6).Add(14 * time.Hour), end: now.AddDate(0, 0, 6).Add(15 * time.Hour)},
	}

	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		t.Fatal(err)
	}
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}

	b := &bytes.Buffer{}
	if err := renderCalendar(b, events, now, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("calendar.png", b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}
