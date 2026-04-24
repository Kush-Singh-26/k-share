package com.kshare.android.feature.share

data class ShareUiState(
    val serverIp: String = "",
    val serverPort: String = "26260",
    val pairingCode: String = "",
    val darkMode: String = "system"
)
