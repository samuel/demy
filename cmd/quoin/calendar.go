package main

import (
	"fmt"
	"image/png"
	"io"
	"sort"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const calendarTemplate = `<scene>
<styles>
  <style name="day-header" font="sans" size="13" weight="bold" />
  <style name="event-title" font="sans" size="9" />
  <style name="event-time" font="sans" size="7" color="#555555" />
  <style name="more" font="sans" size="6" color="#888888" />
</styles>
<vstack padding="16">
	<box background="muted" border-width="1" border-color="#000000" corner-radius="10" padding="6">
		<hstack>
		<text font="sans" size="16" weight="bold">CALENDAR</text>
		<spacer weight="1"></spacer>
		<text font="sans" size="12" color="#555555">{{.CurrentDate}}</text>
		</hstack>
	</box>
  <box padding="4"></box>
	<hstack gap="8">
		{{- range $i, $day := .Days}}
		{{- if $i}}<divider/>{{end}}
		<weighted weight="1">
			<vstack gap="4">
				<text class="day-header">{{$day.Header}}</text>
				{{- range $day.Events}}
					<box padding="3"></box>
					<text class="event-title" max-lines="1" overflow="ellipsis">{{.Title}}</text>
					<text class="event-time">{{.Time}}</text>
				{{- end}}
				{{- if $day.More}}
					<text class="more">+{{$day.More}} more</text>
				{{- end}}
			</vstack>
		</weighted>
		{{- end}}
	</hstack>
</vstack>
</scene>`

type calendarTemplateData struct {
	CurrentDate string
	Days        []calDayData
}

type calDayData struct {
	Header string
	Events []calEventData
	Empty  bool
	More   int
}

type calEventData struct {
	Time  string
	Title string
}

type calendarEvent struct {
	title  string
	start  time.Time
	end    time.Time
	allDay bool
}

const (
	calMaxDays         = 4
	calMaxEventsPerDay = 8
)

func renderCalendar(w io.Writer, events []calendarEvent, now time.Time) error {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return fmt.Errorf("loading bold font: %w", err)
	}

	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Sort: all-day first within each day, then by start time.
	sort.Slice(events, func(i, j int) bool {
		if events[i].allDay != events[j].allDay {
			return events[i].allDay
		}
		return events[i].start.Before(events[j].start)
	})

	days := make([]calDayData, 7)
	for i := range 7 {
		day := today.AddDate(0, 0, i)
		var header string
		switch i {
		case 0:
			header = "Today"
		case 1:
			header = "Tomorrow"
		default:
			header = day.Format("Mon 1/2")
		}
		days[i] = calDayData{Header: header, Empty: true}

		var dayEvents []calEventData
		for _, e := range events {
			var eDay time.Time
			if e.allDay {
				eDay = time.Date(e.start.Year(), e.start.Month(), e.start.Day(), 0, 0, 0, 0, loc)
			} else {
				ls := e.start.In(loc)
				eDay = time.Date(ls.Year(), ls.Month(), ls.Day(), 0, 0, 0, 0, loc)
			}
			if !eDay.Equal(day) {
				continue
			}
			var timeStr string
			if e.allDay {
				timeStr = "All Day"
			} else {
				timeStr = e.start.In(loc).Format("3:04 PM")
			}
			dayEvents = append(dayEvents, calEventData{
				Time:  xmlEsc(timeStr),
				Title: xmlEsc(e.title),
			})
		}

		if len(dayEvents) > 0 {
			days[i].Empty = false
			more := 0
			if len(dayEvents) > calMaxEventsPerDay {
				more = len(dayEvents) - calMaxEventsPerDay
				dayEvents = dayEvents[:calMaxEventsPerDay]
			}
			days[i].Events = dayEvents
			days[i].More = more
		}
	}
	filteredDays := days[:0]
	for _, d := range days {
		if !d.Empty {
			filteredDays = append(filteredDays, d)
			if len(filteredDays) == calMaxDays {
				break
			}
		}
	}
	days = filteredDays

	tmplData := calendarTemplateData{
		CurrentDate: xmlEsc(now.Format("Mon, Jan 2")),
		Days:        days,
	}

	root, styles, err := xmlscene.ExecuteScene([]byte(calendarTemplate), tmplData)
	if err != nil {
		return fmt.Errorf("executing calendar template: %w", err)
	}

	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	cfg.Styles = styles
	c, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	outImg, err := c.Render(root, demy.TwoBit)
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
}
