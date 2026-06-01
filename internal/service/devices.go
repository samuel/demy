package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/samuel/demy/internal/model"
)

var (
	ErrDeviceNotFound      = errors.New("device not found")
	ErrAPIKeyAlreadyIssued = errors.New("API key already issued")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrNotFound            = errors.New("not found")
)

// normMAC normalises a MAC address to uppercase colon-separated form.
func normMAC(mac string) string {
	return strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))
}

// CreateDevice creates a new device (without an API key — waiting for first contact).
func (s *Service) CreateDevice(ctx context.Context, userID, name, macAddress string, refreshRate int) (*model.Device, error) {
	d := &model.Device{
		ID:          uuid.New().String(),
		UserID:      userID,
		MACAddress:  normMAC(macAddress),
		Name:        name,
		RefreshRate: refreshRate,
		CreatedAt:   time.Now().UTC(),
	}
	if d.RefreshRate == 0 {
		d.RefreshRate = 300
	}
	if err := s.store.CreateDevice(ctx, d); err != nil {
		return nil, fmt.Errorf("create device: %w", err)
	}
	return d, nil
}

// GetDevice fetches a device by ID, verifying it belongs to userID.
func (s *Service) GetDevice(ctx context.Context, userID, deviceID string) (*model.Device, error) {
	d, err := s.store.GetDeviceByID(ctx, deviceID)
	if errors.Is(err, model.ErrNotFound) {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	if d.UserID != userID {
		return nil, ErrUnauthorized
	}
	return d, nil
}

// ListDevices returns all devices for a user.
func (s *Service) ListDevices(ctx context.Context, userID string) ([]*model.Device, error) {
	devices, err := s.store.ListDevicesByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	return devices, nil
}

// UpdateDevice updates name, refresh_rate, canvas_config, and timezone.
func (s *Service) UpdateDevice(ctx context.Context, d *model.Device) error {
	if err := s.store.UpdateDevice(ctx, d); err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	return nil
}

// DeleteDevice removes a device and all its playlist items.
func (s *Service) DeleteDevice(ctx context.Context, userID, deviceID string) error {
	d, err := s.store.GetDeviceByID(ctx, deviceID)
	if errors.Is(err, model.ErrNotFound) {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if d.UserID != userID {
		return ErrUnauthorized
	}
	if err := s.store.DeleteDevice(ctx, deviceID); err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	return nil
}

// SetupDevice handles GET /api/setup: looks up by MAC, issues API key if nil.
// Returns (device, wasJustIssued, error).
// First call (no api_key stored): issue a new UUIDv4 api_key.
// Already issued: return ErrAPIKeyAlreadyIssued.
// Not found: return ErrDeviceNotFound.
func (s *Service) SetupDevice(ctx context.Context, mac, accessToken string) (*model.Device, bool, error) {
	mac = normMAC(mac)
	d, err := s.store.GetDeviceByMAC(ctx, mac)
	if errors.Is(err, model.ErrNotFound) {
		return nil, false, ErrDeviceNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("get device: %w", err)
	}

	if d.APIKey != "" {
		// Already issued — validate the presented token.
		if d.APIKey != accessToken {
			return nil, false, ErrUnauthorized
		}
		return d, false, ErrAPIKeyAlreadyIssued
	}
	// First contact: issue a new API key.
	apiKey := uuid.New().String()
	if accessToken != "" {
		apiKey = accessToken
	}
	now := time.Now().UTC()
	if err := s.store.IssueDeviceAPIKey(ctx, d.ID, apiKey, now); err != nil {
		return nil, false, fmt.Errorf("issue api key: %w", err)
	}
	d.APIKey = apiKey
	d.APIKeyIssuedAt = now

	return d, true, nil
}

// AuthenticateDevice looks up a device by API key and validates the MAC address.
func (s *Service) AuthenticateDevice(ctx context.Context, mac, apiKey string) (*model.Device, error) {
	d, err := s.store.GetDeviceByAPIKey(ctx, apiKey)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if normMAC(d.MACAddress) != normMAC(mac) {
		return nil, ErrUnauthorized
	}
	return d, nil
}

// RevokeAPIKey resets the device's API key so it can re-register.
func (s *Service) RevokeAPIKey(ctx context.Context, userID, deviceID string) error {
	d, err := s.store.GetDeviceByID(ctx, deviceID)
	if errors.Is(err, model.ErrNotFound) {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if d.UserID != userID {
		return ErrUnauthorized
	}
	if err := s.store.RevokeDeviceAPIKey(ctx, deviceID); err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}

// ListPlaylistItems returns playlist items for a device in order.
func (s *Service) ListPlaylistItems(ctx context.Context, deviceID string) ([]*model.PlaylistItem, error) {
	items, err := s.store.ListPlaylistItems(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list playlist items: %w", err)
	}
	return items, nil
}

// AddPlaylistItem adds a user plugin to a device's playlist.
func (s *Service) AddPlaylistItem(ctx context.Context, userID, deviceID, userPluginID, dwell string) error {
	// Verify device ownership.
	d, err := s.store.GetDeviceByID(ctx, deviceID)
	if errors.Is(err, model.ErrNotFound) {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if d.UserID != userID {
		return ErrUnauthorized
	}
	item := &model.PlaylistItem{
		ID:           uuid.New().String(),
		DeviceID:     deviceID,
		UserPluginID: userPluginID,
		Dwell:        dwell,
	}
	if err := s.store.AddPlaylistItem(ctx, item); err != nil {
		return fmt.Errorf("add playlist item: %w", err)
	}
	return nil
}

// RemovePlaylistItem removes a playlist item.
func (s *Service) RemovePlaylistItem(ctx context.Context, userID, deviceID, itemID string) error {
	// Verify device ownership.
	d, err := s.store.GetDeviceByID(ctx, deviceID)
	if errors.Is(err, model.ErrNotFound) {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if d.UserID != userID {
		return ErrUnauthorized
	}
	if err := s.store.RemovePlaylistItem(ctx, itemID); err != nil {
		return fmt.Errorf("remove playlist item: %w", err)
	}
	return nil
}

// ReorderPlaylist updates playlist positions.
func (s *Service) ReorderPlaylist(ctx context.Context, userID, deviceID string, orderedIDs []string) error {
	d, err := s.store.GetDeviceByID(ctx, deviceID)
	if errors.Is(err, model.ErrNotFound) {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}
	if d.UserID != userID {
		return ErrUnauthorized
	}
	if err := s.store.ReorderPlaylist(ctx, deviceID, orderedIDs); err != nil {
		return fmt.Errorf("reorder playlist: %w", err)
	}
	return nil
}

// AdvanceDisplay handles the double-advance guard and returns (playlistPosition, advanced, error).
// Debounce: only advance if time.Since(device.LastSeenAt) > 30s.
func (s *Service) AdvanceDisplay(ctx context.Context, device *model.Device, playlistLen int) (int, bool, error) {
	now := time.Now().UTC()
	advanced := time.Since(device.LastSeenAt) > 30*time.Second

	var pos int
	if advanced && playlistLen > 0 {
		var err error
		pos, err = s.store.AdvancePlaylistPosition(ctx, device.ID, playlistLen)
		if err != nil {
			return 0, false, fmt.Errorf("advance playlist position: %w", err)
		}
	} else {
		pos = device.PlaylistPosition
	}

	if err := s.store.UpdateDeviceLastSeen(ctx, device.ID, now, device.LastHeaders); err != nil {
		return pos, advanced, fmt.Errorf("update device last seen: %w", err)
	}

	return pos, advanced, nil
}
