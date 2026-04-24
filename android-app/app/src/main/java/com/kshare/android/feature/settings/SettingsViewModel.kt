package com.kshare.android.feature.settings

import android.app.Application
import android.widget.Toast
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kshare.android.SettingsManager
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

class SettingsViewModel(application: Application) : AndroidViewModel(application) {
    private val settings = SettingsManager(application.applicationContext)

    private val _uiState = MutableStateFlow(
        SettingsUiState(
            pairingCode = settings.pairingCode,
            themeMode = settings.darkMode,
            downloadUri = settings.downloadUri,
            savedIps = settings.getAllSavedIps()
        )
    )
    val uiState: StateFlow<SettingsUiState> = _uiState.asStateFlow()

    fun setPairingCode(value: String) {
        updateState { copy(pairingCode = value) }
    }

    fun setThemeMode(value: String) {
        settings.darkMode = value
        updateState { copy(themeMode = value) }
    }

    fun setDownloadUri(value: String) {
        settings.downloadUri = value
        updateState { copy(downloadUri = value) }
    }

    fun clearDownloadUri() {
        setDownloadUri("")
    }

    fun removeSavedIp(networkId: String) {
        settings.removeServerIp(networkId)
        updateState { copy(savedIps = settings.getAllSavedIps()) }
    }

    fun save() {
        settings.pairingCode = uiState.value.pairingCode.trim()
        settings.darkMode = uiState.value.themeMode
        settings.downloadUri = uiState.value.downloadUri
        Toast.makeText(getApplication(), "Settings saved", Toast.LENGTH_SHORT).show()
    }

    private fun updateState(block: SettingsUiState.() -> SettingsUiState) {
        _uiState.update { it.block() }
    }
}
