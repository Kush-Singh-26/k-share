package com.kshare.android.feature.settings

data class SettingsUiState(
    val pairingCode: String = "",
    val themeMode: String = "system",
    val downloadUri: String = "",
    val savedIps: Map<String, String> = emptyMap()
)
