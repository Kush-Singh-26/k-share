# Current Mismatches Audit

This document records known mismatches between the codebase and older checked-in examples or legacy naming.

## Resolved in this pass

- The repository is now named around `server` and `desktop-app` instead of Windows-specific product names.
- The top-level README now describes the products in cross-platform terms.
- The desktop app manifest uses the new desktop naming.
- Runtime artifacts were removed from the source tree.

## Old example config mismatch

The server example config previously used legacy keys:

- `from_phone_dir`
- `to_phone_dir`
- `pairing_code`

The current server config uses:

- `port`
- `shared_dir`
- `admin_code`
- `guest_code`

## Legacy client settings

The desktop client still preserves `pairing_code` as a fallback field for compatibility with older stored settings.

## Remaining rule

When a docs example differs from the code, the docs are the bug. Keep examples aligned with the live config and API contract.

