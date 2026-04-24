package com.kshare.android.feature.main

import com.kshare.android.api.RemoteFile

interface MainSettingsSource {
    val serverIp: String
    val serverPort: String
    val darkMode: String
}

object MainStateReducer {
    fun initialState(settings: MainSettingsSource): MainUiState {
        return MainUiState(
            serverIp = settings.serverIp,
            serverPort = settings.serverPort.ifEmpty { "26260" },
            themeMode = settings.darkMode
        )
    }

    fun withServerIp(state: MainUiState, ip: String): MainUiState = state.copy(serverIp = ip)

    fun withServerPort(state: MainUiState, port: String): MainUiState = state.copy(serverPort = port)

    fun withFiles(state: MainUiState, files: List<RemoteFile>): MainUiState = state.copy(fileList = files)

    fun setRefreshing(state: MainUiState, refreshing: Boolean): MainUiState = state.copy(isRefreshing = refreshing)
}
