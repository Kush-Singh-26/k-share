# Implementation Plan: K-Share Cross-Platform Improvements

[Overview]
Improve cooperation between server, desktop, and Android components while adding high-value features (file search, mDNS discovery) and fixing remaining reliability issues.

The codebase has already undergone significant architectural refactoring per REFACTOR_PLAN.md. The server has clean internal packages, the desktop app uses service-oriented layers, and the Android app has feature-based modules. This plan builds on that foundation to add new capabilities and tighten cross-platform integration.

Key goals:
1. Enable file search across the shared directory
2. Unify discovery so desktop and Android both use mDNS + subnet scan
3. Fix desktop memory leaks and Android error handling
4. Make thumbnails asynchronous to avoid blocking requests

[Types]
Add shared data structures for file search results and unified errors.

```go
// server/internal/domain/search.go
package domain

type SearchResult struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    IsDirectory bool   `json:"isDirectory"`
    Size        int64  `json:"size"`
    ModTime     string `json:"modTime"`
}

type SearchRequest struct {
    Query string `json:"query"`
    Role  string `json:"role"`
}
```

```go
// server/internal/domain/errors.go
package domain

type AppError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

const (
    ErrUnauthorized         = "unauthorized"
    ErrForbidden            = "forbidden"
    ErrTrustRequired        = "trust_required"
    ErrTrustMismatch        = "trust_mismatch"
    ErrServerUnreachable    = "server_unreachable"
    ErrDiscoveryFailed      = "discovery_failed"
    ErrInvalidConfig        = "invalid_config"
    ErrTransferFailed       = "transfer_failed"
    ErrInvalidPath          = "invalid_path"
    ErrRateLimitExceeded    = "rate_limit_exceeded"
    ErrNotFound             = "not_found"
)
```

```kotlin
// android-app/app/src/main/java/com/kshare/android/domain/Errors.kt
package com.kshare.android.domain

sealed class KShareError(val code: String, val message: String) {
    object Unauthorized : KShareError("unauthorized", "Invalid or missing auth code")
    object Forbidden : KShareError("forbidden", "Admin access required")
    object TrustRequired : KShareError("trust_required", "Unknown server certificate")
    object ServerUnreachable : KShareError("server_unreachable", "Cannot connect to server")
    object DiscoveryFailed : KShareError("discovery_failed", "No server found on network")
    object TransferFailed : KShareError("transfer_failed", "File transfer failed")
    object NotFound : KShareError("not_found", "Resource not found")
    data class Unknown(val msg: String) : KShareError("unknown", msg)
}
```

[Files]
Create new files for search, async thumbnails, and mDNS desktop discovery. Modify existing handlers and client code.

New files:
- `server/internal/domain/search.go` - Search types
- `server/internal/domain/errors.go` - Unified error types
- `server/internal/search/index.go` - File indexing and search service
- `server/internal/httpapi/search_handlers.go` - Search HTTP handlers
- `desktop-app/discovery/mdns.go` - mDNS discovery for desktop
- `android-app/app/src/main/java/com/kshare/android/domain/Errors.kt` - Error model
- `android-app/app/src/main/java/com/kshare/android/feature/search/SearchViewModel.kt` - Search UI state
- `android-app/app/src/main/java/com/kshare/android/feature/search/SearchScreen.kt` - Search composable

Modified files:
- `server/internal/bootstrap/bootstrap.go` - Register new routes, inject search service
- `server/internal/httpapi/handlers.go` - Add HandleSearch, return structured errors
- `server/internal/files/files.go` - Add Walk function for indexing
- `server/internal/thumbnail/cache.go` - Add async thumbnail generation queue
- `server/internal/realtime/hub.go` - Add search update events
- `desktop-app/ui/app.go` - Add search UI, fix memory leaks
- `desktop-app/api/client.go` - Add Search, structured error handling
- `desktop-app/discovery/discovery.go` - Integrate mDNS
- `android-app/app/src/main/java/com/kshare/android/MainActivity.kt` - Add search button
- `android-app/app/src/main/java/com/kshare/android/api/ApiClient.kt` - Add search endpoint, return Result types
- `android-app/app/src/main/java/com/kshare/android/feature/main/MainViewModel.kt` - Add search state
- `android-app/app/src/main/java/com/kshare/android/connection/ConnectionCoordinator.kt` - mDNS before subnet scan

Deleted files:
- None

[Functions]
Add functions for file search, async thumbnails, mDNS discovery, and structured error mapping.

New functions:
- `search.NewIndex(rootDir string) *Index` in `server/internal/search/index.go` - Create file index
- `search.(*Index).Build()` in `server/internal/search/index.go` - Index all files
- `search.(*Index).Query(q string, role string) []domain.SearchResult` in `server/internal/search/index.go` - Search files
- `search.(*Index).NotifyUpdate()` in `server/internal/search/index.go` - Trigger re-index
- `httpapi.Handlers.HandleSearch(w, r)` in `server/internal/httpapi/search_handlers.go` - HTTP search handler
- `files.Walk(rootDir, role string, fn)` in `server/internal/files/files.go` - Walk files with role filter
- `thumbnail.(*Store).QueueGeneration(rootDir, name, folder string)` in `server/internal/thumbnail/cache.go` - Async thumbnail gen
- `thumbnail.(*Store).ProcessQueue()` in `server/internal/thumbnail/cache.go` - Worker loop
- `mdns.Discover(service string, timeout time.Duration) ([]ServiceEntry, error)` in `desktop-app/discovery/mdns.go` - mDNS browse
- `api.Client.Search(ctx, query string) ([]FileInfo, error)` in `desktop-app/api/client.go` - Search API call
- `ApiClient.search(...)` in `android-app/.../ApiClient.kt` - Search endpoint

Modified functions:
- `bootstrap.(*App).Run()` in `server/internal/bootstrap/bootstrap.go` - Add `/search` route, start async thumbnail worker
- `httpapi.Handlers.HandleUpload()` in `server/internal/httpapi/handlers.go` - Return structured errors, trigger search re-index
- `httpapi.Handlers.HandleDelete()` in `server/internal/httpapi/handlers.go` - Trigger search re-index
- `httpapi.writeJSONError(w, code, message, status)` in `server/internal/httpapi/handlers.go` - New helper for structured errors
- `ui.(*App).showHistoryPopup()` in `desktop-app/ui/app.go` - Fix memory leak by reusing window
- `ui.(*App).rebuildHistoryUI()` in `desktop-app/ui/app.go` - Clear old widgets properly
- `ui.(*App).Run()` in `desktop-app/ui/app.go` - Cancel clipboard watcher on exit
- `ConnectionCoordinator.discoverConnection()` in `android-app/.../ConnectionCoordinator.kt` - Try mDNS before subnet scan
- `ApiClient.ping()` in `android-app/.../ApiClient.kt` - Return Result type instead of PingResult

Removed functions:
- None

[Classes]
Add classes for search indexing and structured error management.

New classes:
- `search.Index` in `server/internal/search/index.go` - File search index with in-memory map
- `search.Worker` in `server/internal/search/index.go` - Background re-index worker
- `mdns.Discoverer` in `desktop-app/discovery/mdns.go` - Desktop mDNS browser
- `domain.AppError` in `server/internal/domain/errors.go` - Structured error type
- `KShareError` sealed class in `android-app/.../domain/Errors.kt` - Android error model
- `SearchViewModel` in `android-app/.../feature/search/SearchViewModel.kt` - Search UI state holder

Modified classes:
- `bootstrap.App` in `server/internal/bootstrap/bootstrap.go` - Add SearchIndex field
- `httpapi.Handlers` in `server/internal/httpapi/handlers.go` - Add Search field
- `api.Client` in `desktop-app/api/client.go` - Add search support
- `ui.App` in `desktop-app/ui/app.go` - Add search state, fix clipboard watcher lifecycle
- `MainViewModel` in `android-app/.../MainViewModel.kt` - Add search query state

Removed classes:
- None

[Dependencies]
Add mDNS client library for desktop. No major version conflicts expected.

Server:
- `github.com/grandcat/zeroconf` (already used server-side) - mDNS is already a dependency

Desktop app:
- `github.com/grandcat/zeroconf` v1.0.0 - mDNS client discovery (same library as server)

Android app:
- `androidx.compose.material3:material3` (already present) - Search bar composable

No removals. All additions are lightweight and well-maintained.

[Testing]
Add unit tests for search indexing, error mapping, and mDNS discovery.

New tests:
- `server/internal/search/index_test.go` - Test index build, query, role filtering
- `server/internal/httpapi/search_handlers_test.go` - Test search endpoint auth, results
- `desktop-app/discovery/mdns_test.go` - Test mDNS parsing (mocked)
- `android-app/.../search/SearchViewModelTest.kt` - Test search state management
- `android-app/.../connection/ConnectionCoordinatorTest.kt` - Test mDNS discovery priority

Existing test modifications:
- `server/internal/files/files_test.go` - Add Walk tests
- `server/internal/httpapi/handlers_test.go` (if exists) - Update for structured errors

Validation strategies:
- Server search returns correct results for admin vs guest roles
- mDNS discovery finds server within 5 seconds on LAN
- Desktop clipboard watcher stops cleanly on app exit
- Android returns proper KShareError codes instead of silent failures

[Implementation Order]
Implement in phases to minimize conflicts and ensure each component works before adding the next.

1. **Server foundation**
   - Add domain error types (`server/internal/domain/errors.go`)
   - Add search types and index service (`server/internal/search/index.go`)
   - Add search HTTP handler (`server/internal/httpapi/search_handlers.go`)
   - Modify bootstrap to register new routes and start search indexer
   - Add async thumbnail queue to thumbnail.Store
   - Update files package with Walk function
   - Add server tests for search and errors

2. **Desktop app improvements**
   - Add mDNS discovery package (`desktop-app/discovery/mdns.go`)
   - Add search API method (`desktop-app/api/client.go`)
   - Fix memory leaks in history popup and clipboard watcher lifecycle
   - Add search UI to desktop app (`desktop-app/ui/app.go`)
   - Add desktop tests for mDNS and search

3. **Android app improvements**
   - Add unified error model (`android-app/.../domain/Errors.kt`)
   - Add search API method and Result return types
   - Add search feature (ViewModel + Screen)
   - Modify MainActivity to add search button
   - Update ConnectionCoordinator to try mDNS before subnet scan
   - Add Android tests for search and mDNS discovery

4. **Integration and polish**
   - End-to-end test: File search from desktop and Android
   - Verify mDNS discovery works on desktop
   - Verify async thumbnails don't block uploads
   - Performance test: search on directories with 1000+ files
   - Update README with new features
