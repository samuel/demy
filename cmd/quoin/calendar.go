package main

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"sort"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// CalendarPlugin implements Plugin for the Home Assistant calendar screen.
type CalendarPlugin struct {
	hassURL   string
	hassToken string
	calendars []string
	loc       *time.Location
}

func newCalendarPlugin(haCfg HAConfig, loc *time.Location, cfg map[string]any) (*CalendarPlugin, error) {
	var cals []string
	if v, ok := cfg["calendars"]; ok {
		if slice, ok := v.([]any); ok {
			for _, item := range slice {
				if s, ok := item.(string); ok && s != "" {
					cals = append(cals, s)
				}
			}
		}
	}
	if len(cals) == 0 {
		return nil, fmt.Errorf("calendar plugin requires at least one entry in 'calendars'")
	}
	return &CalendarPlugin{
		hassURL:   haCfg.URL,
		hassToken: haCfg.Token,
		calendars: cals,
		loc:       loc,
	}, nil
}

func (p *CalendarPlugin) Render(ctx context.Context, cfg demy.Config) ([]byte, error) {
	events, err := fetchCalendarFromHA(ctx, p.hassURL, p.hassToken, p.calendars)
	if err != nil {
		return nil, err
	}
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return nil, fmt.Errorf("loading bold font: %w", err)
	}
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	var buf bytes.Buffer
	if err := renderCalendar(&buf, events, time.Now().In(p.loc), cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const calendarTemplate = `<scene>
<styles>
  <style name="day-header" font="sans" size="13" weight="bold" />
  <style name="event-title" font="sans" size="9" />
  <style name="event-time" font="sans" size="7" color="#555555" />
  <style name="more" font="sans" size="6" color="#888888" />
</styles>
<vstack padding="16">
	<hstack>
	<text font="sans" size="16" weight="bold">CALENDAR</text>
	<spacer weight="1"></spacer>
	<text font="sans" size="12" color="#555555">{{.CurrentDate}}</text>
	</hstack>
	<box padding="4" />
	<divider />
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

func renderCalendar(w io.Writer, events []calendarEvent, now time.Time, cfg demy.Config) error {
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

	cfg.Styles = mergeStyles(cfg.Styles, styles)
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

func fetchCalendarFromHA(ctx context.Context, hassURL, hassToken string, calendars []string) ([]calendarEvent, error) {
	client, err := ha.NewClient(hassURL, ha.WithToken(hassToken), ha.WithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("creating HA client: %w", err)
	}

	now := time.Now()
	end := now.AddDate(0, 0, 7)

	var events []calendarEvent
	for _, calID := range calendars {
		haEvents, err := client.GetCalendarEvents(ctx, calID, now, end)
		if err != nil {
			slog.Warn("failed to get calendar events", "calendar", calID, "err", err)
			continue
		}
		for _, e := range haEvents {
			ce, ok := parseCalendarEvent(e)
			if !ok {
				continue
			}
			events = append(events, ce)
		}
	}

	return events, nil
}

func parseCalendarEvent(e ha.CalendarEvent) (calendarEvent, bool) {
	var start, end time.Time
	var allDay bool

	if e.Start.DateTime != "" {
		t, err := time.Parse(time.RFC3339, e.Start.DateTime)
		if err != nil {
			return calendarEvent{}, false
		}
		start = t
	} else if e.Start.Date != "" {
		t, err := time.Parse("2006-01-02", e.Start.Date)
		if err != nil {
			return calendarEvent{}, false
		}
		start = t
		allDay = true
	} else {
		return calendarEvent{}, false
	}

	if e.End.DateTime != "" {
		t, err := time.Parse(time.RFC3339, e.End.DateTime)
		if err == nil {
			end = t
		}
	} else if e.End.Date != "" {
		t, err := time.Parse("2006-01-02", e.End.Date)
		if err == nil {
			end = t
		}
	}

	return calendarEvent{
		title:  e.Summary,
		start:  start,
		end:    end,
		allDay: allDay,
	}, true
}
