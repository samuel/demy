package mqttbridge

import (
	"context"
	"log/slog"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// subscribeCommands subscribes to plugin/set command topics for all devices.
// Called each time the MQTT connection is (re-)established.
func (b *Bridge) subscribeCommands(c mqtt.Client) {
	topic := b.base + "/devices/+/plugin/set"
	tok := c.Subscribe(topic, qos, b.handlePluginSet)
	if !tok.Wait() {
		slog.Warn("mqttbridge: subscribe timeout", "topic", topic)
	} else if err := tok.Error(); err != nil {
		slog.Warn("mqttbridge: subscribe error", "topic", topic, "err", err)
	}
}

// handlePluginSet is called when HASS sends a plugin name on a device's plugin/set topic.
// Topic format: {base}/devices/{mac}/plugin/set
func (b *Bridge) handlePluginSet(c mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	// parts: [base, "devices", mac, "plugin", "set"]
	if len(parts) < 3 {
		return
	}
	mac := parts[len(parts)-3]
	pluginName := strings.TrimSpace(string(msg.Payload()))

	ctx := context.Background()

	d, err := b.store.GetDeviceByMAC(ctx, strings.ToUpper(insertColons(mac)))
	if err != nil {
		slog.Warn("mqttbridge: plugin/set: device not found", "mac", mac, "err", err)
		return
	}

	items, err := b.store.ListPlaylistItems(ctx, d.ID)
	if err != nil || len(items) == 0 {
		slog.Warn("mqttbridge: plugin/set: no playlist items", "device_id", d.ID)
		return
	}

	// Find the playlist position whose plugin name matches.
	for i, item := range items {
		p, err := b.store.GetUserPluginByID(ctx, item.UserPluginID)
		if err != nil {
			continue
		}
		if p.Name == pluginName {
			if err := b.svc.SetPlaylistPosition(ctx, d.ID, i); err != nil {
				slog.Warn("mqttbridge: plugin/set: set position", "device_id", d.ID, "pos", i, "err", err)
				return
			}
			b.publish(b.deviceTopic(mac, "plugin/state"), pluginName, true)
			slog.Info("mqttbridge: plugin/set: position updated", "device", d.Name, "plugin", pluginName, "pos", i)
			return
		}
	}
	slog.Warn("mqttbridge: plugin/set: plugin not in playlist", "device_id", d.ID, "plugin", pluginName)
}

// insertColons converts "aabbccddeeff" → "AA:BB:CC:DD:EE:FF" for store lookup.
func insertColons(mac string) string {
	mac = strings.ToUpper(mac)
	if len(mac) != 12 {
		return mac
	}
	return mac[0:2] + ":" + mac[2:4] + ":" + mac[4:6] + ":" + mac[6:8] + ":" + mac[8:10] + ":" + mac[10:12]
}
