# Config Migration Plan

This document describes the current approach for config and settings evolution.

## Server

- Preserve `port`, `shared_dir`, `admin_code`, and `guest_code` as the canonical schema.
- Future additions should be optional and defaulted.
- Avoid renaming keys unless a migration helper is added.

## Desktop app

- Preserve `server_ip`, `pairing_code`, `download_folder`, `auto_sync_clipboard`, `saved_networks`, and `known_servers`.
- Keep legacy `pairing_code` loading behavior until it is explicitly removed in a migration release.

## Android app

- Keep settings keys stable where possible.
- Extend settings using additive keys instead of schema rewrites.

## Migration rule

Any config change should define:

1. the old shape
2. the new shape
3. how old values are mapped
4. what happens if migration fails

