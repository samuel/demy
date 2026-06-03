package deviceapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Canonical HTTP header names as Go's net/http stores them after
// textproto.CanonicalMIMEHeaderKey normalisation.
const (
	HeaderAccessToken        = "Access-Token"
	HeaderBatteryVoltage     = "Battery-Voltage"
	HeaderFWVersion          = "Fw-Version"
	HeaderHeight             = "Height"
	HeaderID                 = "Id"
	HeaderImageCached        = "Image-Cached"
	HeaderModel              = "Model"
	HeaderRSSI               = "Rssi"
	HeaderRefreshRate        = "Refresh-Rate"
	HeaderSensors            = "Sensors"
	HeaderTemperatureProfile = "Temperature-Profile"
	HeaderUpdateSource       = "Update-Source"
	HeaderUserAgent          = "User-Agent"
	HeaderWakeTime           = "Wake-Time"
	HeaderWidth              = "Width"

	// TRMNL X only

	HeaderBatteryCount       = "Battery-Count"
	HeaderBatteryCharging    = "Battery-Charging"
	HeaderBatteryCurrent     = "Battery-Current"
	HeaderBatteryHealth      = "Battery-Health"
	HeaderBatteryTemperature = "Battery-Temp"
	HeaderBatteryCapacity    = "Battery-Capacity"
	HeaderPercentCharged     = "Percent-Charged"
	HeaderUsbConnected       = "Usb-Connected"
)

type UpdateSource string

const (
	UpdateSourcePowerCycle = "powercycle"
	UpdateSourceTimer      = "timer"
	UpdateSourceButton     = "button"
)

// DeviceHeaders holds the values parsed from device request headers.
// Numeric fields are pointers so callers can distinguish absent from zero.
type DeviceHeaders struct {
	ID                 string // MAC address; always non-empty after successful parse
	AccessToken        string
	BatteryVoltage     *float64
	FWVersion          string
	Height             *int
	Host               string // from r.Host, not r.Header (standard HTTP Host)
	ImageCached        *bool  // whether the device used a cached image this cycle
	Model              string
	RefreshRate        *int
	RSSI               *int
	Sensors            string
	TemperatureProfile TemperatureProfile
	UpdateSource       UpdateSource
	UserAgent          string
	WakeTime           *int // total wake time of the last cycle in ms
	Width              *int

	// TRMNL X only

	BatteryCharging        *bool
	BatteryCount           *int
	BatteryCurrent         *int
	BatteryHealth          *int
	BatteryTemperature     *float64
	CurrentBatteryCapacity *int
	MaxBatteryCapacity     *int
	PercentCharged         *int // prefer over BatteryVoltage when non-nil
	UsbConnected           *bool
}

var errMissingID = &headerError{"missing required ID header"}

type headerError struct{ msg string }

func (e *headerError) Error() string { return e.msg }

func parseDeviceHeaders(r *http.Request) (DeviceHeaders, error) {
	id := r.Header.Get(HeaderID)
	if id == "" {
		return DeviceHeaders{}, errMissingID
	}
	var currentBatteryCapacity, maxBatteryCapacity *int
	if cap := r.Header.Get(HeaderBatteryCapacity); cap != "" {
		curCap, maxCap, _ := strings.Cut(cap, "/")
		if v, err := strconv.Atoi(curCap); err == nil {
			currentBatteryCapacity = &v
		}
		if v, err := strconv.Atoi(maxCap); err == nil {
			maxBatteryCapacity = &v
		}
	}
	return DeviceHeaders{
		ID:                 id,
		AccessToken:        r.Header.Get(HeaderAccessToken),
		BatteryVoltage:     parseOptFloat(r, HeaderBatteryVoltage),
		FWVersion:          r.Header.Get(HeaderFWVersion),
		Height:             parseOptInt(r, HeaderHeight),
		Host:               r.Host,
		ImageCached:        parseOptBool(r, HeaderImageCached),
		Model:              r.Header.Get(HeaderModel),
		RefreshRate:        parseOptInt(r, HeaderRefreshRate),
		RSSI:               parseOptInt(r, HeaderRSSI),
		Sensors:            r.Header.Get(HeaderSensors),
		TemperatureProfile: TemperatureProfile(r.Header.Get(HeaderTemperatureProfile)),
		UpdateSource:       UpdateSource(r.Header.Get(HeaderUpdateSource)),
		UserAgent:          r.Header.Get(HeaderUserAgent),
		WakeTime:           parseOptInt(r, HeaderWakeTime),
		Width:              parseOptInt(r, HeaderWidth),

		BatteryCount:           parseOptInt(r, HeaderBatteryCount),
		BatteryCharging:        parseOptBool(r, HeaderBatteryCharging),
		BatteryCurrent:         parseOptInt(r, HeaderBatteryCurrent),
		BatteryHealth:          parseOptInt(r, HeaderBatteryHealth),
		BatteryTemperature:     parseOptFloat(r, HeaderBatteryTemperature),
		CurrentBatteryCapacity: currentBatteryCapacity,
		MaxBatteryCapacity:     maxBatteryCapacity,
		PercentCharged:         parseOptInt(r, HeaderPercentCharged),
		UsbConnected:           parseOptBool(r, HeaderUsbConnected),
	}, nil
}

func parseOptInt(r *http.Request, header string) *int {
	s := r.Header.Get(header)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		slog.Warn("deviceapi: invalid integer header", "header", header, "value", s)
		return nil
	}
	return &v
}

func parseOptFloat(r *http.Request, header string) *float64 {
	s := r.Header.Get(header)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		slog.Warn("deviceapi: invalid float header", "header", header, "value", s)
		return nil
	}
	return &v
}

func parseOptBool(r *http.Request, header string) *bool {
	s := r.Header.Get(header)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		slog.Warn("deviceapi: invalid bool header", "header", header, "value", s)
		return nil
	}
	return &v
}
