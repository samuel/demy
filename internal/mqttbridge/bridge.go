// Package mqttbridge publishes quoin device state to an MQTT broker using the
// Home Assistant MQTT discovery protocol, and subscribes to plugin-select commands.
package mqttbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/samuel/demy/deviceapi"
	"github.com/samuel/demy/internal/model"
)

// pahoLogger adapts slog to the mqtt.Logger interface used by the Paho library's
// internal diagnostics (connections, retries, PINGREQ/RESP, etc.).
type pahoLogger struct{ level slog.Level }

func (l pahoLogger) Println(v ...any) {
	slog.Log(context.Background(), l.level, fmt.Sprint(v...))
}

func (l pahoLogger) Printf(format string, v ...any) {
	slog.Log(context.Background(), l.level, fmt.Sprintf(format, v...))
}

const (
	defaultBaseTopic = "quoin"
	defaultHAPrefix  = "homeassistant"
	qos              = 1
)

// Bridge connects the quoin server to an MQTT broker and publishes Home Assistant
// discovery payloads and device state.
type Bridge struct {
	client   mqtt.Client
	store    model.Store
	svc      SetPlaylistPositioner
	base     string // e.g. "quoin"
	haPrefix string // e.g. "homeassistant"
	ctx      context.Context
}

// SetPlaylistPositioner is the subset of service.Service used by the bridge.
type SetPlaylistPositioner interface {
	SetPlaylistPosition(ctx context.Context, deviceID string, position int) error
}

// New creates a Bridge and connects to the MQTT broker at brokerURL.
// base is the root topic prefix (default "quoin"); haPrefix is the Home Assistant
// discovery prefix (default "homeassistant").
func New(brokerURL, username, password, base, haPrefix string, store model.Store, svc SetPlaylistPositioner) (*Bridge, error) {
	if base == "" {
		base = defaultBaseTopic
	}
	if haPrefix == "" {
		haPrefix = defaultHAPrefix
	}

	mqtt.ERROR = pahoLogger{slog.LevelError}
	mqtt.WARN = pahoLogger{slog.LevelWarn}
	mqtt.CRITICAL = pahoLogger{slog.LevelError}
	mqtt.DEBUG = pahoLogger{slog.LevelDebug}

	b := &Bridge{store: store, svc: svc, base: base, haPrefix: haPrefix}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("quoin-bridge").
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(10*time.Second).
		SetWill(b.bridgeTopic("status"), "offline", qos, true)

	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		slog.Info("mqttbridge: connected to broker")
		b.publish(b.bridgeTopic("status"), "online", true)
		b.subscribeCommands(c)
		b.publishAllDiscovery()
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		slog.Warn("mqttbridge: connection lost", "err", err)
	})
	opts.SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
		slog.Info("mqttbridge: reconnecting to broker")
	})

	b.client = mqtt.NewClient(opts)
	return b, nil
}

// Start connects to the broker, publishes discovery for all known devices,
// and begins background offline-detection. It returns once connected (or fails).
// If the initial connection attempt times out (broker unreachable), it returns nil
// and continues retrying in the background; discovery is published by OnConnectHandler
// when the connection eventually succeeds.
func (b *Bridge) Start(ctx context.Context) error {
	b.ctx = ctx

	tok := b.client.Connect()

	const connectTimeout = 30 * time.Second
	timeout := connectTimeout
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d < timeout {
			timeout = d
		}
	}
	if !tok.WaitTimeout(timeout) {
		slog.Warn("mqttbridge: initial connect timed out; retrying in background")
		go b.runOfflineChecker(ctx)
		return nil
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("mqttbridge: connect: %w", err)
	}

	go b.runOfflineChecker(ctx)
	return nil
}

// Close disconnects from the broker cleanly.
func (b *Bridge) Close() {
	b.publish(b.bridgeTopic("status"), "offline", true)
	b.client.Disconnect(500)
}

// PublishDeviceState is called from the display handler on each device poll.
// currentPluginName is the name of the plugin being rendered right now.
func (b *Bridge) PublishDeviceState(ctx context.Context, d *model.Device, h deviceapi.DeviceHeaders, currentPluginName string) {
	mac := normMAC(d.MACAddress)

	b.publish(b.deviceTopic(mac, "availability"), "online", true)

	if h.PercentCharged != nil {
		b.publish(b.deviceTopic(mac, "battery/percent"), fmt.Sprintf("%d", *h.PercentCharged), true)
	} else if h.BatteryVoltage != nil {
		// rough conversion: 4.2V = 100%, 3.0V = 0%
		pct := int((*h.BatteryVoltage - 3.0) / 1.2 * 100)
		if pct < 0 {
			pct = 0
		} else if pct > 100 {
			pct = 100
		}
		b.publish(b.deviceTopic(mac, "battery/percent"), fmt.Sprintf("%d", pct), true)
	}
	if h.BatteryVoltage != nil {
		b.publish(b.deviceTopic(mac, "battery/voltage"), fmt.Sprintf("%.3f", *h.BatteryVoltage), true)
	}
	if h.BatteryCharging != nil {
		val := "false"
		if *h.BatteryCharging {
			val = "true"
		}
		b.publish(b.deviceTopic(mac, "battery/charging"), val, true)
	}
	if h.BatteryCurrent != nil {
		b.publish(b.deviceTopic(mac, "battery/current"), fmt.Sprintf("%d", *h.BatteryCurrent), true)
	}
	if h.BatteryTemperature != nil {
		b.publish(b.deviceTopic(mac, "battery/temperature"), fmt.Sprintf("%.1f", *h.BatteryTemperature), true)
	}
	if h.BatteryCount != nil {
		b.publish(b.deviceTopic(mac, "battery/count"), fmt.Sprintf("%d", *h.BatteryCount), true)
	}
	if h.CurrentBatteryCapacity != nil {
		b.publish(b.deviceTopic(mac, "battery/capacity_current"), fmt.Sprintf("%d", *h.CurrentBatteryCapacity), true)
	}
	if h.MaxBatteryCapacity != nil {
		b.publish(b.deviceTopic(mac, "battery/capacity_max"), fmt.Sprintf("%d", *h.MaxBatteryCapacity), true)
	}
	if h.RSSI != nil {
		b.publish(b.deviceTopic(mac, "rssi"), fmt.Sprintf("%d", *h.RSSI), true)
	}
	if h.FWVersion != "" {
		b.publish(b.deviceTopic(mac, "firmware"), h.FWVersion, true)
	}
	if h.UpdateSource != "" {
		b.publish(b.deviceTopic(mac, "wake_reason"), h.UpdateSource, true)
	}
	if h.TemperatureProfile != "" {
		b.publish(b.deviceTopic(mac, "temp_profile"), string(h.TemperatureProfile), true)
	}
	if currentPluginName != "" {
		b.publish(b.deviceTopic(mac, "plugin/state"), currentPluginName, true)
	}
}

// NotifyPlaylistChanged is called when a device's playlist is modified.
// It reloads and republishes the plugin options list and HASS discovery.
func (b *Bridge) NotifyPlaylistChanged(ctx context.Context, deviceID string) {
	d, err := b.store.GetDeviceByID(ctx, deviceID)
	if err != nil {
		slog.Warn("mqttbridge: NotifyPlaylistChanged: get device", "err", err)
		return
	}
	items, err := b.store.ListPlaylistItems(ctx, deviceID)
	if err != nil {
		slog.Warn("mqttbridge: NotifyPlaylistChanged: list items", "err", err)
		return
	}
	names := b.pluginNames(ctx, items)
	b.publishDiscovery(d, names)
	mac := normMAC(d.MACAddress)
	if j, err := json.Marshal(names); err == nil {
		b.publish(b.deviceTopic(mac, "plugin/options"), string(j), true)
	}
}

// publishAllDiscovery loads all known devices and publishes their HA discovery
// payloads and last-known state. Called from OnConnectHandler so it runs on
// both initial connect and reconnect.
func (b *Bridge) publishAllDiscovery() {
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	devices, err := b.store.ListAllDevices(ctx)
	if err != nil {
		slog.Warn("mqttbridge: publishAllDiscovery: list devices", "err", err)
		return
	}
	for _, d := range devices {
		items, err := b.store.ListPlaylistItems(ctx, d.ID)
		if err != nil {
			slog.Warn("mqttbridge: publishAllDiscovery: list playlist", "err", err, "device_id", d.ID)
		}
		b.publishDiscovery(d, b.pluginNames(ctx, items))
		b.publishDeviceStateFromStore(ctx, d)
	}
}

// ─── internal helpers ────────────────────────────────────────────────────────

func (b *Bridge) bridgeTopic(suffix string) string {
	return b.base + "/bridge/" + suffix
}

func (b *Bridge) deviceTopic(mac, suffix string) string {
	return b.base + "/devices/" + mac + "/" + suffix
}

func (b *Bridge) discoveryTopic(component, mac, entity string) string {
	return fmt.Sprintf("%s/%s/%s_%s/config", b.haPrefix, component, mac, entity)
}

// normMAC returns a lowercase MAC without colons for use in MQTT topics.
func normMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(mac, ":", ""))
}

func (b *Bridge) publish(topic, payload string, retain bool) {
	t := b.client.Publish(topic, qos, retain, payload)
	if !t.WaitTimeout(5 * time.Second) {
		slog.Warn("mqttbridge: publish timeout", "topic", topic)
	} else if err := t.Error(); err != nil {
		slog.Warn("mqttbridge: publish error", "topic", topic, "err", err)
	}
}

func (b *Bridge) publishJSON(topic string, v any, retain bool) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Warn("mqttbridge: marshal error", "topic", topic, "err", err)
		return
	}
	b.publish(topic, string(data), retain)
}

// pluginNames looks up the user plugin name for each playlist item in order.
func (b *Bridge) pluginNames(ctx context.Context, items []*model.PlaylistItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		p, err := b.store.GetUserPluginByID(ctx, item.UserPluginID)
		if err != nil {
			names = append(names, item.UserPluginID) // fallback to ID
			continue
		}
		names = append(names, p.Name)
	}
	return names
}

// publishDeviceStateFromStore loads the device's last headers and publishes state.
func (b *Bridge) publishDeviceStateFromStore(ctx context.Context, d *model.Device) {
	mac := normMAC(d.MACAddress)

	// Publish availability based on last seen time.
	avail := "offline"
	grace := time.Duration(d.RefreshRate*2) * time.Second
	if grace == 0 {
		grace = 10 * time.Minute
	}
	if !d.LastSeenAt.IsZero() && time.Since(d.LastSeenAt) < grace {
		avail = "online"
	}
	b.publish(b.deviceTopic(mac, "availability"), avail, true)

	if d.LastHeaders == "" {
		return
	}
	var h deviceapi.DeviceHeaders
	if err := json.Unmarshal([]byte(d.LastHeaders), &h); err != nil {
		return
	}

	// Look up current plugin name.
	items, err := b.store.ListPlaylistItems(ctx, d.ID)
	if err != nil || len(items) == 0 {
		b.PublishDeviceState(ctx, d, h, "")
		return
	}
	pos := d.PlaylistPosition
	if pos >= len(items) {
		pos = 0
	}
	p, err := b.store.GetUserPluginByID(ctx, items[pos].UserPluginID)
	var pluginName string
	if err == nil {
		pluginName = p.Name
	}
	b.PublishDeviceState(ctx, d, h, pluginName)

	// Publish plugin options.
	names := b.pluginNames(ctx, items)
	if j, err := json.Marshal(names); err == nil {
		b.publish(b.deviceTopic(mac, "plugin/options"), string(j), true)
	}
}

// runOfflineChecker marks devices as offline when they exceed 2× their refresh period.
func (b *Bridge) runOfflineChecker(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			devices, err := b.store.ListAllDevices(ctx)
			if err != nil {
				slog.Warn("mqttbridge: offline check: list devices", "err", err)
				continue
			}
			for _, d := range devices {
				if d.LastSeenAt.IsZero() {
					continue
				}
				grace := time.Duration(d.RefreshRate*2) * time.Second
				if grace == 0 {
					grace = 10 * time.Minute
				}
				mac := normMAC(d.MACAddress)
				if time.Since(d.LastSeenAt) > grace {
					b.publish(b.deviceTopic(mac, "availability"), "offline", true)
				}
			}
		}
	}
}
