package mqttbridge

import (
	"github.com/samuel/demy/internal/model"
)

// hassDevice is the device block included in every discovery payload.
type hassDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
}

type hassSensorConfig struct {
	Name                      string     `json:"name"`
	UniqueID                  string     `json:"unique_id"`
	StateTopic                string     `json:"state_topic"`
	AvailabilityTopic         string     `json:"availability_topic"`
	PayloadAvailable          string     `json:"payload_available"`
	PayloadNotAvailable       string     `json:"payload_not_available"`
	DeviceClass               string     `json:"device_class,omitempty"`
	UnitOfMeasurement         string     `json:"unit_of_measurement,omitempty"`
	SuggestedDisplayPrecision *int       `json:"suggested_display_precision,omitempty"`
	Device                    hassDevice `json:"device"`
}

type hassBinarySensorConfig struct {
	Name              string     `json:"name"`
	UniqueID          string     `json:"unique_id"`
	StateTopic        string     `json:"state_topic"`
	PayloadOn         string     `json:"payload_on"`
	PayloadOff        string     `json:"payload_off"`
	DeviceClass       string     `json:"device_class,omitempty"`
	AvailabilityTopic string     `json:"availability_topic,omitempty"`
	PayloadAvailable  string     `json:"payload_available,omitempty"`
	Device            hassDevice `json:"device"`
}

type hassSelectConfig struct {
	Name                string     `json:"name"`
	UniqueID            string     `json:"unique_id"`
	StateTopic          string     `json:"state_topic"`
	CommandTopic        string     `json:"command_topic"`
	AvailabilityTopic   string     `json:"availability_topic"`
	PayloadAvailable    string     `json:"payload_available"`
	PayloadNotAvailable string     `json:"payload_not_available"`
	Options             []string   `json:"options"`
	Device              hassDevice `json:"device"`
}

// publishDiscovery publishes HASS MQTT discovery configs for all entities of a device.
// pluginNames is the ordered list of plugin names in the device's playlist; pass nil to
// skip publishing the select entity (e.g. on first-seen before playlist is set up).
func (b *Bridge) publishDiscovery(d *model.Device, pluginNames []string) {
	mac := normMAC(d.MACAddress)
	avail := b.deviceTopic(mac, "availability")
	dev := b.hassDevice(d)

	// Connectivity binary sensor — uses the device availability topic as its state.
	b.publishJSON(b.discoveryTopic("binary_sensor", mac, "connectivity"), hassBinarySensorConfig{
		Name:        "Online",
		UniqueID:    "quoin_" + mac + "_connectivity",
		StateTopic:  avail,
		PayloadOn:   "online",
		PayloadOff:  "offline",
		DeviceClass: "connectivity",
		Device:      dev,
	}, true)

	precision := func(n int) *int { return &n }

	sensor := func(slug, name, stateTopic, deviceClass, unit string, prec *int) {
		b.publishJSON(b.discoveryTopic("sensor", mac, slug), hassSensorConfig{
			Name:                      name,
			UniqueID:                  "quoin_" + mac + "_" + slug,
			StateTopic:                b.deviceTopic(mac, stateTopic),
			AvailabilityTopic:         avail,
			PayloadAvailable:          "online",
			PayloadNotAvailable:       "offline",
			DeviceClass:               deviceClass,
			UnitOfMeasurement:         unit,
			SuggestedDisplayPrecision: prec,
			Device:                    dev,
		}, true)
	}

	sensor("battery_percent", "Battery", "battery/percent", "battery", "%", nil)
	sensor("battery_voltage", "Battery Voltage", "battery/voltage", "voltage", "V", precision(3))
	sensor("battery_current", "Battery Current", "battery/current", "current", "mA", nil)
	sensor("battery_temperature", "Battery Temperature", "battery/temperature", "temperature", "°C", precision(1))
	sensor("battery_count", "Battery Count", "battery/count", "", "", nil)
	sensor("battery_capacity_current", "Battery Capacity", "battery/capacity_current", "", "mAh", nil)
	sensor("battery_capacity_max", "Battery Capacity Max", "battery/capacity_max", "", "mAh", nil)
	sensor("rssi", "WiFi Signal", "rssi", "signal_strength", "dBm", nil)
	sensor("firmware", "Firmware", "firmware", "", "", nil)
	sensor("wake_reason", "Last Wake Reason", "wake_reason", "", "", nil)
	sensor("temp_profile", "Temperature Profile", "temp_profile", "", "", nil)

	// Charging binary sensor.
	b.publishJSON(b.discoveryTopic("binary_sensor", mac, "charging"), hassBinarySensorConfig{
		Name:              "Charging",
		UniqueID:          "quoin_" + mac + "_charging",
		StateTopic:        b.deviceTopic(mac, "battery/charging"),
		PayloadOn:         "true",
		PayloadOff:        "false",
		DeviceClass:       "battery_charging",
		AvailabilityTopic: avail,
		PayloadAvailable:  "online",
		Device:            dev,
	}, true)

	// Active plugin select entity.
	if pluginNames != nil {
		b.publishJSON(b.discoveryTopic("select", mac, "plugin"), hassSelectConfig{
			Name:                "Active Plugin",
			UniqueID:            "quoin_" + mac + "_plugin",
			StateTopic:          b.deviceTopic(mac, "plugin/state"),
			CommandTopic:        b.deviceTopic(mac, "plugin/set"),
			AvailabilityTopic:   avail,
			PayloadAvailable:    "online",
			PayloadNotAvailable: "offline",
			Options:             pluginNames,
			Device:              dev,
		}, true)
	}
}

func (b *Bridge) hassDevice(d *model.Device) hassDevice {
	mac := normMAC(d.MACAddress)
	return hassDevice{
		Identifiers:  []string{"quoin_" + mac},
		Name:         d.Name,
		Manufacturer: "TRMNL",
	}
}
