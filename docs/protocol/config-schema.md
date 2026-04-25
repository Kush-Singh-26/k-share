# Config Schema

> **Version**: 6.0
> **Last Updated**: 2026-04-25

This document captures the current config and settings shapes used by the three products.

## Server config

Stored at:

- OS user config directory
- `K-Share/config.json`

Schema:

```json
{
  "port": "26260",
  "shared_dir": "C:\\Users\\you\\Documents\\K-Share-Files",
  "admin_code": "123456",
  "guest_code": "guest"
}
```

Fields:

- `port`: HTTPS listen port as a string
- `shared_dir`: shared root folder
- `admin_code`: admin pairing code
- `guest_code`: guest pairing code

Rules:

- Missing config is created with defaults.
- Default `admin_code` is generated randomly.
- Default `guest_code` is `guest`.

## Desktop config

Stored at:

- OS user config directory
- `K-Share/settings.json`

Schema:

```json
{
  "server_ip": "localhost:26260",
  "pairing_code": "",
  "download_folder": "C:\\Users\\you\\Downloads\\K-Share",
  "auto_sync_clipboard": true,
  "saved_networks": {},
  "known_servers": {}
}
```

Fields:

- `server_ip`: last known server host and port
- `pairing_code`: legacy fallback code field used by older flows
- `download_folder`: local download target
- `auto_sync_clipboard`: clipboard sync toggle
- `saved_networks`: cached subnet-to-IP mapping
- `known_servers`: TOFU trust store keyed by cert hash

`known_servers` entries store:

- `cert_hash`
- `auth_code`
- `last_ip`
- `display_name`

## Android settings

Android persists app state through its local storage layer rather than a shared config file.

Current logical keys include:

- `server_ip`
- `server_port`
- `pairing_code`
- `dark_mode`
- `download_uri`
- `known_servers`
- `network_ip_cache`

`known_servers` stores trust metadata keyed by certificate hash.
`network_ip_cache` stores subnet-to-IP discovery hints.

## Migration rules

- Additive fields are allowed.
- Missing fields must fall back to defaults.
- Old config files should continue to load when possible.
- If a field must change shape, document the migration and preserve prior values when feasible.

