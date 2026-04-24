package com.kshare.android

object Constants {
    const val DEFAULT_PORT = 26260
    const val DEFAULT_SERVER_PORT_STRING = "26260"

    // Notification IDs
    const val SYNC_NOTIFICATION_ID = 101
    const val TRANSFER_NOTIFICATION_BASE_ID = 100

    // Permission request codes
    const val PERMISSION_REQUEST_CODE = 101

    // Channel IDs
    const val SYNC_CHANNEL_ID = "sync_channel"
    const val TRANSFER_CHANNEL_ID = "transfer"

    // WebSocket
    const val WS_PATH = "/ws"
    const val WS_RECONNECT_DELAY_MS = 5000L

    // Polling
    const val POLLING_INTERVAL_MS = 15000L
    const val CLIPBOARD_EDIT_COOLDOWN_MS = 5000L

    // Network Scanner
    const val SCANNER_WORKER_COUNT = 60
    const val SCANNER_CONNECT_TIMEOUT_MS = 500
    const val MDNS_TIMEOUT_MS = 2500L

    // Thumbnail Cache
    const val MAX_DISK_CACHE_SIZE = 50 * 1024 * 1024L // 50 MB

    // Transfer
    const val TRANSFER_BUFFER_SIZE = 64 * 1024
    const val PROGRESS_UPDATE_INTERVAL_MS = 1000L
    const val WAKE_LOCK_TIMEOUT_MS = 10 * 60 * 1000L // 10 minutes
}
