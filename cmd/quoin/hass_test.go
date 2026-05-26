package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
)

func TestHomeAssistant(t *testing.T) {
	hassToken := os.Getenv("HASS_TOKEN")
	if hassToken == "" {
		t.Skip("HASS_TOKEN not set")
	}
	client, err := ha.NewClient(
		"https://hass.internal.descolada.com:8123/",
		ha.WithToken(hassToken),
		ha.WithTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	cfg, err := client.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Connected to Home Assistant %s", cfg.Version)

	cals, err := client.GetCalendars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(cals, "", "  ")
	t.Logf("Calendars: %s", b)
	events, err := client.GetCalendarEvents(ctx, "calendar.family", time.Now(), time.Now().AddDate(0, 0, 14))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(events, "", "  ")
	t.Logf("Events: %s", b)
}
