# Protocol Compatibility Policy

This project uses an additive-first compatibility policy.

## Default rule

- New fields, events, and response members may be added without breaking existing clients.
- Existing endpoints, event names, and config keys should remain stable unless a migration is planned.

## Breaking changes

A change is considered breaking if it:

- renames or removes an endpoint
- renames or removes an event type
- changes auth semantics
- changes a config key without fallback
- changes trust persistence behavior in a way that invalidates existing stored trust

Breaking changes require:

1. a migration plan
2. a doc update
3. a server/client compatibility note
4. tests covering the old and new behavior where practical

## Compatibility target

The current goal is to keep the server and both clients compatible across normal refactor iterations while the code is being reorganized.

