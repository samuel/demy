package model

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

// PluginType classifies how a plugin renders its output.
type PluginType string

const (
	PluginTypeBuiltin            PluginType = "builtin"
	PluginTypeJSONTemplate       PluginType = "json_template"
	PluginTypeStarlark           PluginType = "starlark"
	PluginTypeDatasourceTemplate PluginType = "datasource_template"
)

// User is an authenticated account owner.
type User struct {
	ID          string
	DisplayName string
	Email       string
	IsAdmin     bool
	CreatedAt   time.Time
}

// PasskeyCredential holds a WebAuthn credential for a user.
type PasskeyCredential struct {
	ID           string
	UserID       string
	CredentialID []byte
	PublicKey    []byte
	AAGUID       []byte
	SignCount    uint32
	Transports   []string
	Name         string
	CreatedAt    time.Time
	LastUsedAt   time.Time
}

// Session is a browser session tied to a user.
type Session struct {
	ID         string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt time.Time
}

// Device is a registered TRMNL display.
type Device struct {
	ID               string
	UserID           string
	MACAddress       string
	APIKey           string // empty until first /api/setup
	APIKeyIssuedAt   time.Time
	Name             string
	RefreshRate      int    // seconds; default 300
	CanvasConfig     string // JSON-encoded CanvasConfig
	Timezone         string // IANA; empty = UTC
	CreatedAt        time.Time
	LastSeenAt       time.Time
	PlaylistPosition int
	LastHeaders      string // JSON snapshot of most recent DeviceHeaders
}

// PlaylistItem links a user plugin instance to a device playlist slot.
type PlaylistItem struct {
	ID           string
	DeviceID     string
	UserPluginID string
	Position     int
	Dwell        string // override duration string; empty = plugin default
}

// LibraryPlugin is an admin-managed template plugin visible to all users.
type LibraryPlugin struct {
	ID           string
	Slug         string
	Name         string
	Description  string
	PluginType   PluginType
	ConfigSchema string // JSON Schema (draft-7 subset)
	Template     string // XML demy template; empty for builtin/starlark
	Script       string // Starlark script; empty for others
	Thumbnail    string
	IsActive     bool
	SortOrder    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserPlugin is one user's configured instance of a plugin.
type UserPlugin struct {
	ID              string
	UserID          string
	LibraryPluginID string // empty if external/custom
	ExternalURL     string
	Name            string
	PluginType      PluginType
	Config          string // JSON object
	Template        string
	Script          string
	Refresh         string // duration string; empty = 15m
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastRenderedAt  time.Time
}

// CachedResource is a cached HTTP response body.
type CachedResource struct {
	Key         string // SHA-256 of "userID|url"
	URL         string
	UserID      string
	Data        []byte
	ContentType string
	FetchedAt   time.Time
	ExpiresAt   time.Time
}

// DeviceLogEntry is one record stored from POST /api/log.
type DeviceLogEntry struct {
	ID              string
	DeviceID        string
	LoggedAt        time.Time
	Message         string
	WiFiStatus      string
	WakeReason      string
	FirmwareVersion string
	BatteryVoltage  float64
	WiFiSignal      int
	FreeHeapSize    int
	SpecialFunction int
}

// Store is the data access interface. All database operations go through it.
type Store interface {
	// Users
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, u *User) error
	CountUsers(ctx context.Context) (int, error)
	ListUsers(ctx context.Context) ([]*User, error)

	// Passkeys
	CreatePasskey(ctx context.Context, pk *PasskeyCredential) error
	GetPasskeysByUserID(ctx context.Context, userID string) ([]*PasskeyCredential, error)
	GetPasskeyByCredentialID(ctx context.Context, credID []byte) (*PasskeyCredential, error)
	UpdatePasskey(ctx context.Context, pk *PasskeyCredential) error
	DeletePasskey(ctx context.Context, id string) error
	CountPasskeysByUserID(ctx context.Context, userID string) (int, error)

	// Sessions
	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	TouchSession(ctx context.Context, id string, lastUsed time.Time) error
	DeleteSession(ctx context.Context, id string) error
	DeleteExpiredSessions(ctx context.Context) error

	// Devices
	CreateDevice(ctx context.Context, d *Device) error
	GetDeviceByID(ctx context.Context, id string) (*Device, error)
	GetDeviceByMAC(ctx context.Context, mac string) (*Device, error)
	GetDeviceByAPIKey(ctx context.Context, apiKey string) (*Device, error)
	ListDevicesByUser(ctx context.Context, userID string) ([]*Device, error)
	ListAllDevices(ctx context.Context) ([]*Device, error)
	UpdateDevice(ctx context.Context, d *Device) error
	DeleteDevice(ctx context.Context, id string) error
	IssueDeviceAPIKey(ctx context.Context, deviceID, apiKey string, issuedAt time.Time) error
	RevokeDeviceAPIKey(ctx context.Context, deviceID string) error
	UpdateDeviceLastSeen(ctx context.Context, deviceID string, seenAt time.Time, headersJSON string) error
	AdvancePlaylistPosition(ctx context.Context, deviceID string, length int) (int, error)
	SetPlaylistPosition(ctx context.Context, deviceID string, position int) error

	// Playlist items
	ListPlaylistItems(ctx context.Context, deviceID string) ([]*PlaylistItem, error)
	AddPlaylistItem(ctx context.Context, item *PlaylistItem) error
	UpdatePlaylistItem(ctx context.Context, item *PlaylistItem) error
	RemovePlaylistItem(ctx context.Context, id string) error
	ReorderPlaylist(ctx context.Context, deviceID string, orderedIDs []string) error

	// Library plugins
	ListLibraryPlugins(ctx context.Context, activeOnly bool) ([]*LibraryPlugin, error)
	GetLibraryPluginBySlug(ctx context.Context, slug string) (*LibraryPlugin, error)
	GetLibraryPluginByID(ctx context.Context, id string) (*LibraryPlugin, error)
	CreateLibraryPlugin(ctx context.Context, p *LibraryPlugin) error
	UpdateLibraryPlugin(ctx context.Context, p *LibraryPlugin) error

	// User plugin instances
	CreateUserPlugin(ctx context.Context, p *UserPlugin) error
	GetUserPluginByID(ctx context.Context, id string) (*UserPlugin, error)
	ListUserPlugins(ctx context.Context, userID string) ([]*UserPlugin, error)
	UpdateUserPlugin(ctx context.Context, p *UserPlugin) error
	DeleteUserPlugin(ctx context.Context, id string) error

	// Resource cache
	GetCachedResource(ctx context.Context, key string) (*CachedResource, error)
	SetCachedResource(ctx context.Context, r *CachedResource) error
	EvictExpiredResources(ctx context.Context) error

	// Device logs
	InsertDeviceLog(ctx context.Context, entry *DeviceLogEntry) error
}
