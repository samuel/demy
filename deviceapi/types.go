package deviceapi

type SpecialFunction string

const (
	SpecialFunctionNone            = SpecialFunction("none")
	SpecialFunctionIdentify        = SpecialFunction("identify")
	SpecialFunctionSleep           = SpecialFunction("sleep")
	SpecialFunctionAddWifi         = SpecialFunction("add_wifi")
	SpecialFunctionRestartPlaylist = SpecialFunction("restart_playlist")
	SpecialFunctionRewind          = SpecialFunction("rewind")
	SpecialFunctionSendToMe        = SpecialFunction("send_to_me")
	SpecialFunctionGuestMode       = SpecialFunction("guest_mode")
)

type TemperatureProfile string

const (
	TemperatureProfileDefault TemperatureProfile = "default"
	TemperatureProfileA       TemperatureProfile = "a"
	TemperatureProfileB       TemperatureProfile = "b"
	TemperatureProfileC       TemperatureProfile = "c"
)

type TouchbarMode string

const (
	TouchbarModeTap   TouchbarMode = "tap"
	TouchbarModeSlide TouchbarMode = "slide"
)

// DisplayResponse is the JSON body sent back to the device on GET /api/display.
type DisplayResponse struct {
	Action               string             `json:"action,omitempty"`
	Filename             string             `json:"filename"`
	FirmwareURL          string             `json:"firmware_url"`
	FirmwareVersion      string             `json:"firmware_version"`
	ImageURL             string             `json:"image_url"`
	ImageURLTimeout      int                `json:"image_url_timeout"`
	MaximumCompatibility bool               `json:"maximum_compatibility"`
	RefreshRate          int                `json:"refresh_rate"`
	ResetFirmware        bool               `json:"reset_firmware"`
	SpecialFunction      SpecialFunction    `json:"special_function"`
	Status               int                `json:"status,omitempty"`
	TemperatureProfile   TemperatureProfile `json:"temperature_profile,omitempty"`
	TouchbarMode         TouchbarMode       `json:"touchbar_mode,omitempty"`
	UpdateFirmware       bool               `json:"update_firmware"`
}

// SetupResponse is the JSON body sent back on GET /api/setup/.
type SetupResponse struct {
	APIKey     string `json:"api_key"`
	FriendlyID string `json:"friendly_id"`
	ImageURL   string `json:"image_url"`
	Message    string `json:"message"`
}

// LogEntry is one record within the POST /api/log request body.
type LogEntry struct {
	ID              int     `json:"id"`
	Message         string  `json:"message"`
	WiFiStatus      string  `json:"wifi_status"`
	CreatedAt       int64   `json:"created_at"`
	SleepDuration   int     `json:"sleep_duration"`
	RefreshRate     int     `json:"refresh_rate"`
	FreeHeapSize    int     `json:"free_heap_size"`
	MaxAllocSize    int     `json:"max_alloc_size"`
	SourcePath      string  `json:"source_path"`
	WakeReason      string  `json:"wake_reason"`
	FirmwareVersion string  `json:"firmware_version"`
	Retry           int     `json:"retry"`
	BatteryVoltage  float64 `json:"battery_voltage"`
	SourceLine      int     `json:"source_line"`
	SpecialFunction string  `json:"special_function"`
	WiFiSignal      int     `json:"wifi_signal"`
}

type logRequest struct {
	Logs []LogEntry `json:"logs"`
}
