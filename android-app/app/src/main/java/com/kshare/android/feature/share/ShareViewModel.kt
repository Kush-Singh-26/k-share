package com.kshare.android.feature.share

import android.app.Application
import android.net.Uri
import android.widget.Toast
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kshare.android.SettingsManager
import com.kshare.android.api.ApiClient
import com.kshare.android.transfer.TransferLauncher
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

class ShareViewModel(application: Application) : AndroidViewModel(application) {
    private val appContext = application.applicationContext
    private val settings = SettingsManager(appContext)

    private val _uiState = MutableStateFlow(
        ShareUiState(
            serverIp = settings.serverIp,
            serverPort = settings.serverPort.ifEmpty { "26260" },
            pairingCode = settings.pairingCode,
            darkMode = settings.darkMode
        )
    )
    val uiState: StateFlow<ShareUiState> = _uiState.asStateFlow()

    fun refresh() {
        _uiState.update {
            it.copy(
                serverIp = settings.serverIp,
                serverPort = settings.serverPort.ifEmpty { "26260" },
                pairingCode = settings.pairingCode,
                darkMode = settings.darkMode
            )
        }
    }

    fun upload(uri: Uri) {
        TransferLauncher.upload(
            context = appContext,
            serverIp = settings.serverIp,
            serverPort = settings.serverPort.toIntOrNull() ?: 26260,
            pairingCode = settings.pairingCode,
            uri = uri
        )
    }

    fun upload(uris: List<Uri>) {
        TransferLauncher.upload(
            context = appContext,
            serverIp = settings.serverIp,
            serverPort = settings.serverPort.toIntOrNull() ?: 26260,
            pairingCode = settings.pairingCode,
            uris = uris
        )
    }

    fun openOnPc(url: String) {
        val ip = settings.serverIp
        if (ip.isEmpty()) {
            Toast.makeText(appContext, "Server IP not configured", Toast.LENGTH_SHORT).show()
            return
        }

        viewModelScope.launch {
            val success = ApiClient.openOnPc(
                ip,
                settings.serverPort.toIntOrNull() ?: 26260,
                url,
                settings.pairingCode
            )
            Toast.makeText(appContext, if (success) "Opening on PC..." else "Open failed", Toast.LENGTH_SHORT).show()
        }
    }

    fun pushToClipboard(text: String) {
        val ip = settings.serverIp
        if (ip.isEmpty()) {
            Toast.makeText(appContext, "Server IP not configured", Toast.LENGTH_SHORT).show()
            return
        }

        viewModelScope.launch {
            val success = ApiClient.postClipboard(
                ip,
                settings.serverPort.toIntOrNull() ?: 26260,
                text,
                settings.pairingCode,
                append = true
            )
            Toast.makeText(appContext, if (success) "Synced to Laptop" else "Sync failed", Toast.LENGTH_SHORT).show()
        }
    }
}
