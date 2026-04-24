package com.kshare.android

import kotlinx.coroutines.flow.MutableStateFlow

data class AppState(
    val serverIp: String = "",
    val serverPort: Int = 26260,
    val isConnected: Boolean = false,
    val clipboardText: String = "",
    val isGuestMode: Boolean = false,
    val discoveryStatus: String = "",
    val files: List<com.kshare.android.api.RemoteFile> = emptyList(),
    val isRefreshing: Boolean = false
)

object SyncManager {
    val state = MutableStateFlow(AppState())

    fun updateState(block: (AppState) -> AppState) {
        state.value = block(state.value)
    }
}
