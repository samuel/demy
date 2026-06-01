CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  is_admin INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS passkey_credentials (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id BLOB NOT NULL UNIQUE,
  public_key BLOB NOT NULL,
  aaguid BLOB NOT NULL,
  sign_count INTEGER NOT NULL DEFAULT 0,
  transports TEXT NOT NULL DEFAULT '[]',
  name TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  last_used_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  last_used_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mac_address TEXT NOT NULL UNIQUE,
  api_key TEXT,
  api_key_issued_at DATETIME,
  name TEXT NOT NULL DEFAULT '',
  refresh_rate INTEGER NOT NULL DEFAULT 300,
  canvas_config TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  last_seen_at DATETIME,
  playlist_position INTEGER NOT NULL DEFAULT 0,
  last_headers TEXT
);

CREATE TABLE IF NOT EXISTS library_plugins (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  plugin_type TEXT NOT NULL,
  config_schema TEXT NOT NULL DEFAULT '{}',
  template TEXT NOT NULL DEFAULT '',
  script TEXT NOT NULL DEFAULT '',
  thumbnail TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS user_plugins (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  library_plugin_id TEXT REFERENCES library_plugins(id) ON DELETE SET NULL,
  external_url TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  plugin_type TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  template TEXT NOT NULL DEFAULT '',
  script TEXT NOT NULL DEFAULT '',
  refresh TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  last_rendered_at DATETIME
);

CREATE TABLE IF NOT EXISTS playlist_items (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  user_plugin_id TEXT NOT NULL REFERENCES user_plugins(id) ON DELETE CASCADE,
  position INTEGER NOT NULL DEFAULT 0,
  dwell TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS resource_cache (
  key TEXT PRIMARY KEY,
  url TEXT NOT NULL,
  user_id TEXT NOT NULL,
  data BLOB NOT NULL,
  content_type TEXT NOT NULL DEFAULT '',
  fetched_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS device_logs (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  logged_at DATETIME NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  wifi_status TEXT NOT NULL DEFAULT '',
  wake_reason TEXT NOT NULL DEFAULT '',
  firmware_version TEXT NOT NULL DEFAULT '',
  battery_voltage REAL NOT NULL DEFAULT 0,
  wifi_signal INTEGER NOT NULL DEFAULT 0,
  free_heap_size INTEGER NOT NULL DEFAULT 0,
  special_function INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id);
CREATE INDEX IF NOT EXISTS idx_playlist_items_device_id ON playlist_items(device_id);
CREATE INDEX IF NOT EXISTS idx_user_plugins_user_id ON user_plugins(user_id);
CREATE INDEX IF NOT EXISTS idx_resource_cache_expires_at ON resource_cache(expires_at);
CREATE INDEX IF NOT EXISTS idx_device_logs_device_id ON device_logs(device_id);
