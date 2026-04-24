# K-Share Refactor Plan

This document is a planning artifact only. It describes a full-codebase refactor strategy for K-Share without making behavioral changes during the planning phase.

The target architecture is cross-platform. The current `server` and `desktop-app` modules are the long-term product boundaries. The intended product shape is:

- `server`: cross-platform background service
- `desktop-app`: cross-platform desktop client
- `android-app`: Android client

Longer term, the architecture should support more clients without changing core protocol or trust semantics.

## 1. Goals

### Functional goals

- Preserve existing user-visible capabilities:
  - LAN discovery
  - HTTPS file upload/download
  - clipboard sync
  - clipboard image sync
  - guest/admin access separation
  - thumbnail previews
  - clipboard history
  - trust-on-first-use certificate pinning
- Keep current server-client compatibility during most of the refactor.
- Prepare the codebase for future desktop targets beyond Windows.
- Make it realistic to add future clients such as macOS, Linux, iOS, web admin tooling, or CLI/TUI surfaces.

### Non-functional goals

- Reduce coupling between transport, business logic, and UI.
- Remove single-file and activity-heavy concentration of logic.
- Improve testability and regression safety.
- Make config, persistence, and protocol behavior explicit.
- Standardize naming, data contracts, and error handling across all apps.

### Constraints

- No protocol break unless explicitly scheduled as a migration.
- No data loss in config, trust store, clipboard history, or shared files.
- No rewrite-from-scratch approach.
- Refactor in phases with working software after each milestone.

## 2. Current State Summary

### Current repository shape

- `server`: Go HTTP/WebSocket server
- `desktop-app`: Go Fyne desktop GUI client
- `android-app`: Kotlin Android client

### Current architectural issues

- The server is highly concentrated in [`server/main.go`](/C:/Users/KIIT0001/k-share/server/main.go).
- The desktop client mixes UI, persistence, connection, trust, discovery, and file operations in [`desktop-app/ui/app.go`](/C:/Users/KIIT0001/k-share/desktop-app/ui/app.go) and [`desktop-app/ui/operations.go`](/C:/Users/KIIT0001/k-share/desktop-app/ui/operations.go).
- The Android app concentrates orchestration in [`android-app/app/src/main/java/com/kush/kshare/MainActivity.kt`](/C:/Users/KIIT0001/k-share/android-app/app/src/main/java/com/kush/kshare/MainActivity.kt).
- Protocol and documentation are already drifting. The checked-in server example config is inconsistent with the actual server config.
- Runtime artifacts are checked into source folders, which blurs source/runtime boundaries.

### Primary refactor risks

- Breaking auth or role semantics across apps
- Breaking TOFU trust and cert pinning flows
- Breaking path and file access rules for guest/admin roles
- Breaking discovery behavior on real LANs
- Moving code without first defining contracts

## 3. Target Product Model

The refactor should move the repo toward three stable product concepts.

### Server

The server is the cross-platform local service that owns:

- file storage
- auth and role enforcement
- clipboard state
- clipboard history
- thumbnails
- TLS certificate lifecycle
- LAN discovery advertisement
- event fanout over WebSockets

It should run on Windows, macOS, and Linux. Platform-specific integrations such as systray, context menu registration, or native open-folder actions must be adapters, not core design assumptions.

### Desktop app

The desktop app is a cross-platform client shell that owns:

- server discovery
- trust/pinning UX
- file browsing and transfer UX
- clipboard editing and syncing UX
- optional platform integrations such as system clipboard, open-folder, notifications

It should not own business rules that belong to the server protocol or domain model.

### Android app

The Android app remains a dedicated client, but its internal architecture should align with the same domain boundaries and protocol contracts as the desktop app.

## 4. Naming Direction

This plan assumes the repo will gradually migrate away from platform-locked names.

### Current names

- `server`
- `desktop-app`

### Target names

- `server`
- `desktop-app`
- `android-app`

### Migration rule

Do not rename directories at the start of the refactor. First create clean internal boundaries. Rename modules only after:

- contracts are stable
- imports are under control
- build and packaging paths are documented

That avoids mixing structural rename churn with logic refactors.

## 5. Required Shared Contracts

Before large refactors, define the protocol and state model explicitly in a design document and tests.

### 5.1 Auth and role contract

Define:

- `admin` role
- `guest` role
- unauthenticated behavior
- which endpoints allow unauthenticated `ping`
- which endpoints require auth
- whether role is derived only from bearer code
- exact semantics for `401` vs `403`

### 5.2 Server API contract

Define canonical request and response behavior for:

- `GET /ping`
- `GET /files`
- `POST /upload`
- `GET /download/{path}`
- `GET/POST /clipboard`
- `GET/POST /clipboard/image`
- `GET/DELETE /clipboard/history`
- `GET /thumbnail`
- `DELETE /delete`
- `GET /ws`

Specify:

- headers
- request params
- response schemas
- error responses
- guest visibility rules
- folder handling semantics

### 5.3 Event contract

Define WebSocket event types and payloads explicitly. Avoid stringly-typed event behavior without documentation.

Minimum event model:

- `files_changed`
- `clipboard_changed`
- `clipboard_image_changed`
- `history_changed`
- optional future `trust_required`, `server_notice`, `sync_status`

### 5.4 Config contract

Define one current canonical config schema for the server and each client.

Document:

- field names
- defaults
- storage locations per OS
- migration rules for old keys
- required vs optional fields

### 5.5 Trust contract

Define:

- how fingerprint is derived
- what is persisted
- what happens on cert change
- what happens if IP changes but fingerprint stays same
- behavior for first trust, re-trust, and trust reset

### 5.6 Discovery contract

Define discovery phases and fallback order:

- cached IP
- localhost
- mDNS
- subnet scan
- expanded scan
- manual IP entry

This must be shared behavior, even if implementation varies by platform.

## 6. Target Repository Architecture

The long-term repo should be organized around product modules plus shared documents, not platform-specific historical names.

### Proposed top-level layout

```text
/server
/desktop-app
/android-app
/docs
/assets
```

### Proposed documentation layout

```text
/docs/architecture
/docs/protocol
/docs/migrations
/docs/release
```

### Proposed architecture docs

- `docs/architecture/system-overview.md`
- `docs/protocol/http-api.md`
- `docs/protocol/websocket-events.md`
- `docs/protocol/config-schema.md`
- `docs/protocol/trust-model.md`
- `docs/migrations/config-migration-plan.md`
- `docs/release/build-and-package.md`

## 7. Target Server Architecture

The current server monolith should be split into dependency-oriented packages.

### Proposed server layout

```text
server/
  cmd/kshare-server/
  internal/config/
  internal/auth/
  internal/domain/
  internal/storage/
  internal/clipboard/
  internal/history/
  internal/thumbnail/
  internal/discovery/
  internal/ws/
  internal/httpapi/
  internal/platform/
  internal/app/
```

### Package responsibilities

#### `internal/config`

- load config
- validate config
- apply defaults
- migrate old config schema
- expose typed config values

#### `internal/auth`

- role resolution from request/auth token
- guest/admin policy helpers
- shared auth middleware logic

#### `internal/domain`

- domain types:
  - file metadata
  - clipboard entry
  - history item
  - trust-independent event types
  - role and permission enums

This package should contain business types without HTTP or OS dependencies.

#### `internal/storage`

- shared directory management
- path validation
- safe file listing
- unique naming/versioning
- public folder rules
- trash rules
- zip-for-download support

#### `internal/clipboard`

- current clipboard state management
- private vs guest channels
- image clipboard state
- persistence hooks if required

#### `internal/history`

- history append/remove/list
- retention rules
- serialization

#### `internal/thumbnail`

- thumbnail generation
- cache path management
- cache eviction policy

#### `internal/discovery`

- server-side discovery advertisement
- future abstraction for mDNS or alternative advertisements

#### `internal/ws`

- connection hub
- event fanout
- typed server-side event publishing

#### `internal/httpapi`

- route registration
- handlers by concern
- request parsing and response encoding
- shared error mapping

#### `internal/platform`

- systray
- open-folder integration
- native context menu integration
- stubs for unsupported platforms

This package must be optional around the core server, not central to it.

#### `internal/app`

- compose dependencies
- build the server app object
- wire config, services, API, and platform adapters

### Server design rules

- No business logic in `cmd/kshare-server/main.go`.
- No direct package-level mutable globals for app state unless strictly isolated and justified.
- All file access must go through storage boundaries.
- All role-sensitive behavior must go through auth or policy helpers.
- HTTP handlers must call services, not implement storage/auth logic inline.

## 8. Target Desktop App Architecture

The desktop app should become a reusable cross-platform application shell rather than a Fyne-centric bundle of callbacks.

### Proposed desktop layout

```text
desktop-app/
  cmd/kshare-desktop/
  internal/config/
  internal/domain/
  internal/session/
  internal/discovery/
  internal/trust/
  internal/transfers/
  internal/clipboard/
  internal/history/
  internal/thumbnails/
  internal/platform/
  internal/presentation/
  internal/ui/
```

### Package responsibilities

#### `internal/config`

- load/save app settings
- migrate old settings
- resolve platform-specific app data paths

#### `internal/domain`

- view-independent application models
- file list item model
- connection status model
- trust prompt model
- history item model

#### `internal/session`

- current server address
- auth code
- connection status
- current role
- websocket lifecycle orchestration

#### `internal/discovery`

- server discovery orchestration
- cached IP handling
- subnet-based hints
- future mDNS integration

#### `internal/trust`

- TOFU certificate handling
- known-server persistence
- fingerprint verification and re-trust logic

#### `internal/transfers`

- upload
- download
- unzip after download
- progress reporting
- error mapping

#### `internal/clipboard`

- text clipboard sync
- image clipboard sync
- OS clipboard adapter hooks

#### `internal/history`

- load history
- delete history items
- restore-from-history flow

#### `internal/thumbnails`

- thumbnail fetch queue
- thumbnail cache
- cache invalidation

#### `internal/platform`

- clipboard adapters
- open-folder
- notifications
- platform-specific dialogs if retained

#### `internal/presentation`

- app-level state models
- command handlers
- UI-facing view models or presenters

#### `internal/ui`

- actual Fyne UI or any future GUI/TUI surface

### Desktop design rules

- UI code must not call HTTP endpoints directly.
- UI code must not mutate persisted config directly except through app services.
- Trust logic must not live inside widgets.
- Discovery logic must not live inside button handlers.
- File transfer and clipboard orchestration must be reusable without the GUI layer.

### Cross-platform intent

The current Fyne client can remain the first desktop surface, but after refactor it should be only one presentation layer. That leaves room for:

- Fyne GUI on Windows/macOS/Linux
- future TUI
- future CLI utilities

without reimplementing the core logic.

## 9. Target Android App Architecture

The Android app should be split by feature and responsibility. Activities should host UI and delegate orchestration to view models and services.

### Proposed Android package layout

```text
android-app/app/src/main/java/com/kush/kshare/
  core/
  data/
  network/
  discovery/
  trust/
  clipboard/
  files/
  sync/
  feature/main/
  feature/settings/
  feature/share/
  feature/history/
  ui/theme/
```

### Package responsibilities

#### `core`

- shared types
- app-level constants
- error/result wrappers

#### `data`

- repositories
- settings persistence
- known-server persistence
- mapping between raw/network/storage models and domain models

#### `network`

- API client
- request builders
- response parsing
- websocket transport

#### `discovery`

- LAN scanning
- cached IP logic
- mDNS integration
- discovery state machine

#### `trust`

- cert fingerprint storage
- TOFU decision flow
- trust reset and mismatch handling

#### `clipboard`

- clipboard sync orchestration
- OS clipboard integration

#### `files`

- file browsing
- upload/download orchestration
- folder selection and output target rules

#### `sync`

- background sync services/workers
- retry rules
- network-aware reconnect logic

#### `feature/main`

- main screen view model
- screen state
- user actions and side effects

#### `feature/settings`

- settings screen view model
- settings-specific validation and save flow

#### `feature/share`

- inbound share intent handling
- upload action wiring

#### `feature/history`

- history screen/dialog model

### Android design rules

- `MainActivity` should not own long-lived business logic.
- `SettingsManager` should evolve into repository-style access.
- View models should expose screen state and actions.
- Services and workers should call repositories/services, not UI code.
- Discovery, trust, and websocket lifecycle should be coordinated by dedicated components, not manually scattered across screens and services.

## 10. Cross-Cutting Design Decisions

### 10.1 Domain model first

All three apps should converge on a common conceptual model even if the code is not physically shared yet:

- `Role`
- `RemoteFile`
- `ClipboardChannel`
- `HistoryItem`
- `ServerIdentity`
- `TrustState`
- `ConnectionState`
- `DiscoveryResult`
- `AppError`

The names and fields should mean the same thing everywhere.

### 10.2 Error model

Define a consistent error vocabulary:

- `unauthorized`
- `forbidden`
- `trust_required`
- `trust_mismatch`
- `server_unreachable`
- `discovery_failed`
- `invalid_config`
- `transfer_failed`
- `invalid_path`
- `unsupported_platform_feature`

Clients can present these differently, but they should map back to the same conceptual failures.

### 10.3 Persistence boundaries

Separate:

- source files
- generated runtime files
- user config
- app cache
- trust store
- logs

No runtime files should live inside source module directories in normal development flow.

### 10.4 Platform abstraction

Any OS-specific logic should sit behind explicit interfaces:

- clipboard adapter
- open-file/open-folder adapter
- systray adapter
- notification adapter
- context menu adapter
- filesystem location resolver

This is necessary for cross-platform support.

## 11. Migration Strategy

The refactor should be staged to reduce risk.

### Stage 1: Documentation and contract freeze

- Write protocol docs.
- Write config schema docs.
- Write trust model docs.
- Resolve current README/config drift.
- Decide compatibility rules.

### Stage 2: Safety net

- Add server regression tests around auth, role visibility, path validation, files, clipboard, and downloads.
- Add desktop client tests around config/trust/discovery helpers.
- Add Android tests around repositories and discovery/trust logic where feasible.

### Stage 3: Server internal extraction

- Extract modules from current monolith without changing external API.
- Introduce `app` wiring layer.
- Keep route behavior stable.

### Stage 4: Desktop app internal extraction

- Extract config, trust, session, transfer, discovery, and clipboard services.
- Keep existing Fyne surface, but make it thin.

### Stage 5: Android internal extraction

- Move business logic from `MainActivity` into feature/state/repository layers.
- Isolate services/workers from UI details.

### Stage 6: Naming migration

- Rename `server` if needed only after contracts are stable.
- Rename `desktop-app` if needed only after contracts are stable.
- Update docs, build instructions, and package identifiers as needed.

### Stage 7: Product expansion readiness

- Evaluate Linux and macOS packaging requirements.
- Decide whether to add desktop-specific alternate surfaces such as TUI or CLI.
- Consider shared protocol test fixtures across server and clients.

## 12. Detailed Work Breakdown

### Workstream A: Protocol and contract cleanup

Tasks:

- Create API endpoint spec
- Create WebSocket event spec
- Create config schema spec
- Create trust model spec
- Audit current mismatches between docs and code
- Define versioning strategy for protocol changes

Exit criteria:

- Every major endpoint and config file is documented
- Auth and trust behavior are explicit
- Existing clients can be validated against the written spec

### Workstream B: Server architecture

Tasks:

- identify cohesive service boundaries in current `main.go`
- extract domain types from handler logic
- extract config loading and validation
- extract auth middleware and role policy
- extract storage service and path safety
- extract clipboard and history services
- extract thumbnail service and cache handling
- extract WebSocket hub and typed event publishing
- extract route registration and handler grouping
- isolate OS-specific features behind platform adapters

Exit criteria:

- `main.go` becomes bootstrap only
- services are testable in isolation
- no protocol regression

### Workstream C: Desktop app architecture

Tasks:

- separate persistent settings from live session state
- create session/trust/discovery/transfers/clipboard services
- convert direct widget callbacks into service commands
- centralize error mapping and connection state handling
- isolate Fyne dialogs and presentation-only logic
- normalize thumbnail loading as a separate concern

Exit criteria:

- UI is thin and replaceable
- business logic is testable without GUI widgets
- cross-platform desktop direction is no longer blocked by OS-specific assumptions

### Workstream D: Android architecture

Tasks:

- introduce feature-focused view models
- move network orchestration into repositories/services
- centralize settings and known-server persistence
- isolate trust logic
- isolate discovery state machine
- isolate websocket lifecycle and reconnect strategy
- separate upload/download/share flows from `MainActivity`
- align worker/service behavior with repositories

Exit criteria:

- `MainActivity` becomes a host, not the system brain
- background sync is easier to reason about
- feature state is testable

### Workstream E: Build and release cleanup

Tasks:

- remove checked-in runtime files from source module directories
- standardize build outputs
- standardize dev vs release config behavior
- document packaging targets by OS
- define app-data/cache/log locations by platform

Exit criteria:

- source tree contains source, not local runtime debris
- build and package flow is reproducible

### Workstream F: Testing and verification

Tasks:

- add server unit tests
- add server endpoint integration tests
- add trust and config migration tests
- add desktop service-level tests
- add Android repository/view model tests
- define manual LAN verification checklist

Exit criteria:

- refactor can proceed with regression confidence

## 13. Recommended Execution Order

This order matters.

1. Freeze the protocol, config, and trust contracts in docs.
2. Add regression tests around the current server behavior.
3. Refactor the server internally without API changes.
4. Refactor the desktop app around service boundaries without UI replacement.
5. Refactor the Android app around repository/view model boundaries.
6. Clean runtime artifacts, packaging, and docs.
7. Rename modules to cross-platform names.
8. Only after that, implement optional new front ends such as TUI or CLI (Completed for TUI).

## 14. Trade-Offs

### Why not rename modules first

Because directory renames generate high churn and make code review harder while delivering little architectural value by themselves.

### Why not rewrite everything into a shared library immediately

Because the server, desktop, and Android app are in different languages and platforms. Shared concepts matter immediately; shared implementation should be evaluated later and only where it clearly reduces duplication.

### Why not replace the desktop GUI with a TUI during refactor

Because that changes product surface and architecture at the same time. First isolate application logic from the current GUI. After that, adding a TUI becomes a smaller and safer decision.

### Why server first

Because both clients depend on its protocol, auth rules, and trust semantics. Refactoring clients first without a stabilized server contract is higher risk.

## 15. Milestones

### Milestone 1: Architecture freeze

- Docs written
- config drift identified
- compatibility rules agreed

### Milestone 2: Server decomposition

- server internals extracted
- tests added
- no API break

### Milestone 3: Desktop decomposition

- GUI thin over services
- trust/discovery/transfers extracted

### Milestone 4: Android decomposition

- feature/state boundaries established
- `MainActivity` reduced substantially

### Milestone 5: Cross-platform cleanup

- naming updated
- platform adapters explicit
- packaging path clarified for non-Windows targets

## 16. Out Of Scope For This Planning Phase

- protocol-breaking redesign
- replacing Fyne with another desktop framework
- replacing Android UI design
- adding iOS, Linux, or macOS clients immediately
- adding cloud relay or internet sync
- implementing CLI front ends

## 17. First Concrete Next Actions

The first real execution batch should be:

1. Create protocol, config, and trust docs under `docs/`.
2. Remove documentation drift between current server code and checked-in examples.
3. Add regression tests around the current server endpoints and role rules.
4. Define the target package boundaries for the server.
5. Extract the server into internal packages without changing external behavior.

That sequence gives the rest of the refactor a stable base.

## 18. Execution Checklist

This checklist is intended to be updated as work progresses. Items should be marked from `[ ]` to `[x]` only when the task is complete.

### Planning and contracts

- [x] Write `docs/architecture/system-overview.md`
- [x] Write `docs/protocol/http-api.md`
- [x] Write `docs/protocol/websocket-events.md`
- [x] Write `docs/protocol/config-schema.md`
- [x] Write `docs/protocol/trust-model.md`
- [x] Define protocol compatibility policy between server and clients
- [x] Audit and document all current mismatches between code and README/config examples
- [x] Update checked-in examples so they match the current contract

### Server safety net

- [x] Add regression tests for auth and role resolution
- [x] Add regression tests for guest/public visibility rules
- [x] Add regression tests for path validation and traversal protection
- [x] Add regression tests for file listing behavior
- [x] Add regression tests for upload behavior
- [x] Add regression tests for download behavior
- [x] Add regression tests for clipboard text behavior
- [x] Add regression tests for clipboard image behavior
- [x] Add regression tests for history behavior
- [x] Add regression tests for thumbnail behavior

### Server refactor

- [x] Create server package layout under a new internal structure
- [x] Extract config loading and validation from the monolithic entrypoint
- [x] Extract auth and role policy into a dedicated package
- [x] Extract storage and safe path handling into a dedicated package
- [x] Extract clipboard state management into a dedicated package
- [x] Extract clipboard history into a dedicated package
- [x] Extract thumbnail generation and cache management into a dedicated package
- [x] Extract WebSocket hub and typed event publishing into a dedicated package
- [x] Extract HTTP handlers into grouped transport packages
- [x] Extract platform-specific integrations behind platform adapters
- [x] Reduce server bootstrap file to wiring only
- [x] Re-run server regression tests after extraction

### Desktop app safety net

- [x] Add tests for desktop config loading and migration
- [x] Add tests for trust store and known-server behavior
- [x] Add tests for discovery helper behavior
- [x] Add tests for transfer service behavior
- [x] Add tests for session state transitions
- [x] Centralize desktop config mutations behind helper methods

### Desktop app refactor

- [x] Separate persistent settings from live session state
- [x] Extract session management into a dedicated package
- [x] Extract discovery orchestration into a dedicated package
- [x] Extract trust/pinning logic into a dedicated package
- [x] Extract file transfer logic into a dedicated package
- [x] Extract clipboard orchestration into a dedicated package
- [x] Extract history loading and actions into a dedicated package
- [x] Extract thumbnail loading and cache orchestration into a dedicated package
- [x] Introduce presentation/view-model state separate from the UI toolkit
- [x] Reduce GUI layer to presentation and input wiring only
- [x] Verify the desktop app still works against the unchanged server API

### Android app safety net

- [x] Add tests for settings persistence and migration
- [x] Add tests for trust and known-server behavior
- [x] Add tests for discovery decision logic
- [x] Add tests for API client parsing and error mapping
- [x] Add tests for feature state/view model behavior

### Android app refactor

- [x] Introduce feature-oriented package structure
- [x] Move settings persistence behind repository-style access
- [x] Move network operations behind repositories/services
- [x] Extract trust flow out of activities
- [x] Extract discovery state machine out of activities
- [x] Extract websocket lifecycle management out of activities
- [x] Extract clipboard synchronization logic out of activities
- [x] Extract file transfer orchestration out of activities
- [x] Move background sync logic behind dedicated sync components
- [x] Introduce focused view models for main flow
- [x] Introduce focused view models for settings flow
- [x] Introduce focused view models for share flow
- [x] Introduce focused view models for history flow
- [x] Reduce `MainActivity` to a host/orchestration boundary
- [ ] Verify the Android app still works against the unchanged server API

### Cross-platform cleanup

- [x] Define platform adapter interfaces for clipboard, notifications, open-folder, and tray/menu integration
- [x] Define OS-specific config, cache, log, and trust-store locations
- [x] Remove source-tree assumptions that only work on Windows
- [x] Document packaging expectations for Windows, Linux, and macOS server/desktop targets
- [x] Confirm desktop core logic is no longer tied to platform-specific naming or platform-specific behavior

### Repository cleanup

- [x] Remove checked-in runtime-generated logs from source directories
- [x] Remove checked-in generated cert/key artifacts from source directories
- [x] Remove checked-in local config/runtime state from source directories
- [x] Remove checked-in built binaries from source directories
- [x] Standardize build output locations
- [x] Standardize development vs release packaging instructions

### Naming migration

- [x] Rename top-level product folders to `server` and `desktop-app`
- [x] Update import paths, scripts, docs, and build instructions after rename
- [x] Verify no documentation still describes the desktop/server products as platform-specific by default
- [x] Clean up Android package naming and remove generated template tests

### Final validation

- [ ] Run full server test suite
- [ ] Run desktop app verification checklist
- [ ] Run Android app verification checklist
- [ ] Run end-to-end LAN pairing verification
- [ ] Run admin role verification
- [ ] Run guest role verification
- [ ] Run trust-on-first-use verification
- [ ] Run cert-rotation mismatch verification
- [ ] Run file upload/download verification across clients
- [ ] Run clipboard text/image sync verification across clients
- [x] Update final architecture docs to match the refactored codebase
