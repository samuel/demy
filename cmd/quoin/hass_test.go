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

	states, err := client.GetStateForEntity(ctx, "weather.pirateweather")
	// states, err := client.GetStateForEntity(ctx, "weather.home")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))

	res, err := client.GetWeatherForecasts(ctx, "weather.pirateweather", "daily")
	b, err = json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}
