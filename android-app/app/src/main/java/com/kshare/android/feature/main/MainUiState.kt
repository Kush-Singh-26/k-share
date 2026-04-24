package com.kshare.android.feature.main

import com.kshare.android.api.RemoteFile

data class MainUiState(
    val serverIp: String = "",
    val serverPort: String = "26260",
    val statusColor: Long = 0xFFFFFF00,
    val clipboardText: String = "",
    val clipboardChannel: String = "",
    val isGuestMode: Boolean = false,
    val fileList: List<RemoteFile> = emptyList(),
    val isRefreshing: Boolean = false,
    val discoveryStatus: String = "",
    val fileToDelete: RemoteFile? = null,
    val themeMode: String = "system",
    val showTrustDialog: Boolean = false,
    val pendingTrustIp: String = "",
    val pendingTrustHash: String = "",
    val pendingTrustPort: Int = 26260,
    val searchQuery: String = ""
)
