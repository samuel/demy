package mqttbridge

import (
	"github.com/samuel/demy/deviceapi"
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

// publishDiscovery publishes HASS MQTT discovery configs for the entities of a
// device that are backed by actual header data. h may be nil (e.g. first-seen
// before any poll has occurred), in which case only the connectivity sensor and
// the plugin select entity are published. pluginNames is the ordered list of
// plugin names in the device's playlist; pass nil to skip the select entity.
func (b *Bridge) publishDiscovery(d *model.Device, h *deviceapi.DeviceHeaders, pluginNames []string) {
	mac := normMAC(d.MACAddress)
	avail := b.deviceTopic(mac, "availability")
	dev := b.hassDevice(d)

	// Connectivity binary sensor — always publish; uses the availability topic as its state.
	b.publishJSON(b.discoveryTopic("binary_sensor", mac, "connectivity"), hassBinarySensorConfig{
		Name:        "Online",
		UniqueID:    "quoin_" + mac + "_connectivity",
		StateTopic:  avail,
		PayloadOn:   "online",
		PayloadOff:  "offline",
		DeviceClass: "connectivity",
		Device:      dev,
	}, true)

	if h == nil {
		if pluginNames != nil {
			b.publishPluginSelect(mac, avail, dev, pluginNames)
		}
		return
	}

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

	binarySensor := func(slug, name, stateTopic, deviceClass string) {
		b.publishJSON(b.discoveryTopic("binary_sensor", mac, slug), hassBinarySensorConfig{
			Name:              name,
			UniqueID:          "quoin_" + mac + "_" + slug,
			StateTopic:        b.deviceTopic(mac, stateTopic),
			PayloadOn:         "true",
			PayloadOff:        "false",
			DeviceClass:       deviceClass,
			AvailabilityTopic: avail,
			PayloadAvailable:  "online",
			Device:            dev,
		}, true)
	}

	if h.PercentCharged != nil || h.BatteryVoltage != nil {
		sensor("battery_percent", "Battery", "battery/percent", "battery", "%", nil)
	}
	if h.BatteryVoltage != nil {
		sensor("battery_voltage", "Battery Voltage", "battery/voltage", "voltage", "V", precision(3))
	}
	if h.RSSI != nil {
		sensor("rssi", "WiFi Signal", "rssi", "signal_strength", "dBm", nil)
	}
	if h.FWVersion != "" {
		sensor("firmware", "Firmware", "firmware", "", "", nil)
	}
	if h.UpdateSource != "" {
		sensor("wake_reason", "Last Wake Reason", "wake_reason", "", "", nil)
	}
	if h.TemperatureProfile != "" {
		sensor("temp_profile", "Temperature Profile", "temp_profile", "", "", nil)
	}
	if h.WakeTime != nil {
		sensor("wake_time", "Wake Time", "wake_time", "duration", "ms", nil)
	}
	if h.ImageCached != nil {
		binarySensor("image_cached", "Image Cached", "image_cached", "")
	}

	// TRMNL X only sensors.
	if h.BatteryCurrent != nil {
		sensor("battery_current", "Battery Current", "battery/current", "current", "mA", nil)
	}
	if h.BatteryTemperature != nil {
		sensor("battery_temperature", "Battery Temperature", "battery/temperature", "temperature", "°C", precision(1))
	}
	if h.BatteryCount != nil {
		sensor("battery_count", "Battery Count", "battery/count", "", "", nil)
	}
	if h.CurrentBatteryCapacity != nil {
		sensor("battery_capacity_current", "Battery Capacity", "battery/capacity_current", "", "mAh", nil)
	}
	if h.MaxBatteryCapacity != nil {
		sensor("battery_capacity_max", "Battery Capacity Max", "battery/capacity_max", "", "mAh", nil)
	}
	if h.BatteryCharging != nil {
		binarySensor("charging", "Charging", "battery/charging", "battery_charging")
	}
	if h.UsbConnected != nil {
		binarySensor("usb_connected", "USB Connected", "usb_connected", "plug")
	}

	if pluginNames != nil {
		b.publishPluginSelect(mac, avail, dev, pluginNames)
	}
}

func (b *Bridge) publishPluginSelect(mac, avail string, dev hassDevice, pluginNames []string) {
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

func (b *Bridge) hassDevice(d *model.Device) hassDevice {
	mac := normMAC(d.MACAddress)
	return hassDevice{
		Identifiers:  []string{"quoin_" + mac},
		Name:         d.Name,
		Manufacturer: "TRMNL",
	}
}
