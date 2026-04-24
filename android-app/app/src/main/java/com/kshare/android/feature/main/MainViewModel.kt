package com.kshare.android.feature.main

import android.app.Application
import android.content.Context
import android.net.Uri
import android.widget.Toast
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kshare.android.NetworkScanner
import com.kshare.android.SettingsManager
import com.kshare.android.ThumbnailCache
import com.kshare.android.api.ApiClient
import com.kshare.android.api.RemoteFile
import com.kshare.android.connection.AndroidDiscoveryGateway
import com.kshare.android.connection.ConnectionCoordinator
import com.kshare.android.connection.ConnectionOutcome
import com.kshare.android.sync.ClipboardSyncManager
import com.kshare.android.sync.OkHttpWebSocketEngine
import com.kshare.android.sync.WebSocketSessionManager
import com.kshare.android.transfer.TransferLauncher
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.concurrent.TimeUnit

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val appContext = application.applicationContext
    private val settings = SettingsManager(appContext)
    private val connectionCoordinator = ConnectionCoordinator(
        settings = settings,
        discoveryGateway = AndroidDiscoveryGateway(appContext)
    )
    private val client by lazy {
        ApiClient.getSecureClient().newBuilder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .build()
    }
    private val webSocketManager by lazy {
        WebSocketSessionManager(OkHttpWebSocketEngine(client), viewModelScope)
    }
    private val clipboardSyncManager = ClipboardSyncManager()

    private var pollingJob: Job? = null
    private var lastUserEditTime = 0L

    private val _uiState = MutableStateFlow(MainStateReducer.initialState(settings))
    val uiState: StateFlow<MainUiState> = _uiState.asStateFlow()

    override fun onCleared() {
        closeWebSocket()
        stopPolling()
        super.onCleared()
    }

    fun onResume() {
        syncSettings()
        tryAutoConnect()
    }

    fun onPause() {
        closeWebSocket()
        stopPolling()
    }

    fun setServerIp(ip: String) {
        settings.serverIp = ip
        updateState { copy(serverIp = ip) }
    }

    fun setServerPort(port: String) {
        settings.serverPort = port
        updateState { copy(serverPort = port) }
        connectWebSocket()
    }

    fun setClipboardText(text: String) {
        lastUserEditTime = System.currentTimeMillis()
        updateState { copy(clipboardText = text) }
    }

    fun setClipboardChannel(channel: String) {
        updateState { copy(clipboardChannel = channel) }
        fetchClipboard()
    }

    fun requestDeleteFile(file: RemoteFile) {
        updateState { copy(fileToDelete = file) }
    }

    fun clearDeleteRequest() {
        updateState { copy(fileToDelete = null) }
    }

    fun dismissTrustDialog() {
        updateState {
            copy(
                showTrustDialog = false,
                discoveryStatus = "Connection cancelled"
            )
        }
    }

    fun refreshFiles() {
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) return

        updateState { copy(isRefreshing = true) }
        viewModelScope.launch {
            val files = ApiClient.listFiles(ip, port, settings.pairingCode)
            updateState {
                copy(
                    fileList = files,
                    isRefreshing = false,
                    searchQuery = ""
                )
            }
        }
    }
    
    fun setSearchQuery(query: String) {
        updateState { copy(searchQuery = query) }
        
        if (query.length >= 2) {
            val ip = uiState.value.serverIp
            val port = currentPort()
            if (ip.isEmpty()) return

            viewModelScope.launch {
                val results = ApiClient.searchFiles(ip, port, settings.pairingCode, query)
                updateState {
                    copy(
                        fileList = results,
                        isRefreshing = false
                    )
                }
            }
        } else if (query.isEmpty()) {
            refreshFiles()
        }
    }


    fun pushClipboard() {
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) return

        val text = uiState.value.clipboardText
        viewModelScope.launch {
            if (clipboardSyncManager.pushText(
                    serverIp = ip,
                    port = port,
                    text = text,
                    pairingCode = settings.pairingCode,
                    append = false,
                    channel = uiState.value.clipboardChannel
                )
            ) {
                Toast.makeText(appContext, "Pushed", Toast.LENGTH_SHORT).show()
            }
        }
    }

    fun fetchClipboard() {
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) return

        viewModelScope.launch {
            val text = clipboardSyncManager.fetchText(
                serverIp = ip,
                port = port,
                pairingCode = settings.pairingCode,
                channel = uiState.value.clipboardChannel
            )
            if (text != null && clipboardSyncManager.shouldApplyRemoteText(text)) {
                clipboardSyncManager.rememberReceivedText(text)
                applyRemoteClipboardText(text)
            }
        }
    }

    fun copyToPhoneClipboard() {
        val text = uiState.value.clipboardText
        if (text.isEmpty()) {
            Toast.makeText(appContext, "Nothing to copy", Toast.LENGTH_SHORT).show()
            return
        }
        val clipboardManager = appContext.getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        val clip = android.content.ClipData.newPlainText("K-Share", text)
        clipboardManager.setPrimaryClip(clip)
        Toast.makeText(appContext, "Copied to clipboard", Toast.LENGTH_SHORT).show()
    }

    fun confirmDeleteFile() {
        val file = uiState.value.fileToDelete ?: return
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) {
            clearDeleteRequest()
            return
        }

        viewModelScope.launch {
            val success = ApiClient.deleteFile(ip, port, file.name, settings.pairingCode)
            if (success) {
                Toast.makeText(appContext, "Deleted", Toast.LENGTH_SHORT).show()
                refreshFiles()
            } else {
                Toast.makeText(appContext, "Delete failed", Toast.LENGTH_SHORT).show()
            }
            clearDeleteRequest()
        }
    }

    fun downloadFile(file: RemoteFile) {
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) return
        TransferLauncher.download(
            context = appContext,
            serverIp = ip,
            serverPort = port,
            pairingCode = settings.pairingCode,
            fileName = file.name,
            isDir = file.isDir
        )
    }

    fun uploadFolder(treeUri: Uri) {
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) return
        TransferLauncher.uploadFolder(
            context = appContext,
            serverIp = ip,
            serverPort = port,
            pairingCode = settings.pairingCode,
            treeUri = treeUri
        )
    }

    fun discoverServer() {
        viewModelScope.launch {
            updateState { copy(statusColor = YELLOW) }
            when (val outcome = connectionCoordinator.discoverConnection(currentPort(), false) { status ->
                updateState { copy(discoveryStatus = status) }
            }) {
                is ConnectionOutcome.Connected -> applyConnectedOutcome(outcome)
                is ConnectionOutcome.TrustRequired -> updateTrustRequest(outcome)
                is ConnectionOutcome.NoLan -> updateNoLan()
                is ConnectionOutcome.NotFound -> updateNotFound()
                is ConnectionOutcome.Failed -> updateDiscoveryFailure(outcome.message)
            }
        }
    }

    fun verifyManualIp() {
        val ip = uiState.value.serverIp.trim()
        val port = currentPort()
        if (ip.isEmpty()) return

        viewModelScope.launch {
            updateState { copy(statusColor = YELLOW, discoveryStatus = "Connecting...") }
            when (val outcome = connectionCoordinator.verifyConnection(ip, port, false)) {
                is ConnectionOutcome.Connected -> applyConnectedOutcome(outcome)
                is ConnectionOutcome.TrustRequired -> updateTrustRequest(outcome)
                is ConnectionOutcome.Failed -> updateDiscoveryFailure(outcome.message)
                else -> Unit
            }
        }
    }

    fun acceptPendingTrust() {
        val state = uiState.value
        when (val outcome = connectionCoordinator.trustPendingServer(
            state.pendingTrustIp,
            state.pendingTrustPort,
            state.pendingTrustHash
        )) {
            is ConnectionOutcome.Connected -> applyConnectedOutcome(outcome)
            else -> Unit
        }
    }

    fun getPairingCode(): String = settings.pairingCode

    fun changeThemeMode(themeMode: String) {
        settings.darkMode = themeMode
        updateState { copy(themeMode = themeMode) }
    }

    private fun syncSettings() {
        updateState {
            copy(
                serverIp = settings.serverIp,
                serverPort = settings.serverPort.ifEmpty { "26260" },
                themeMode = settings.darkMode
            )
        }
    }

    private fun tryAutoConnect() {
        viewModelScope.launch {
            if (!NetworkScanner.hasValidLan(appContext)) {
                updateState {
                    copy(
                        statusColor = GRAY,
                        discoveryStatus = "No LAN"
                    )
                }
                return@launch
            }

            val port = currentPort()
            val networkId = NetworkScanner.getNetworkId(appContext)
            if (networkId != null) {
                val cachedIp = settings.getLastServerIp(networkId)
                if (cachedIp != null) {
                    updateState { copy(discoveryStatus = "Connecting...") }
                    when (val outcome = connectionCoordinator.verifyConnection(cachedIp, port, true)) {
                        is ConnectionOutcome.Connected -> applyConnectedOutcome(outcome)
                        else -> Unit
                    }
                    return@launch
                }
            }

            val savedIp = uiState.value.serverIp
            if (savedIp.isNotEmpty()) {
                when (val outcome = connectionCoordinator.verifyConnection(savedIp, port, true)) {
                    is ConnectionOutcome.Connected -> applyConnectedOutcome(outcome)
                    else -> Unit
                }
            }
        }
    }

    private fun connectWebSocket() {
        val ip = uiState.value.serverIp
        val port = currentPort()
        if (ip.isEmpty()) return

        webSocketManager.connect(ip, port, listener = object : WebSocketSessionManager.Listener {
            override fun onOpen() {
                updateState { copy(statusColor = GREEN) }
            }

            override fun onMessage(text: String) {
                val type = runCatching { org.json.JSONObject(text).optString("type") }.getOrDefault("")
                when (type) {
                    "clip" -> {
                        if (System.currentTimeMillis() - lastUserEditTime > 5_000) {
                            fetchClipboard()
                        }
                    }
                    "files" -> refreshFiles()
                }
            }

            override fun onClosed(code: Int, reason: String) {
                updateState { copy(statusColor = RED) }
            }

            override fun onFailure(t: Throwable) {
                updateState { copy(statusColor = RED) }
            }
        })
    }

    private fun closeWebSocket() {
        webSocketManager.close()
    }

    private fun startPolling() {
        stopPolling()
        pollingJob = viewModelScope.launch {
            while (isActive) {
                if (NetworkScanner.hasValidLan(appContext)) {
                    val ip = uiState.value.serverIp
                    val port = currentPort()
                    if (ip.isNotEmpty()) {
                        val online = ApiClient.ping(ip, port).success
                        updateState {
                            copy(statusColor = if (online) GREEN else RED)
                        }
                        if (online && System.currentTimeMillis() - lastUserEditTime > 5_000) {
                            fetchClipboardSilently(ip, port)
                        }
                    }
                } else {
                    updateState { copy(statusColor = GRAY) }
                }
                delay(15_000)
            }
        }
    }

    private fun stopPolling() {
        pollingJob?.cancel()
        pollingJob = null
    }

    private fun fetchClipboardSilently(ip: String, port: Int) {
        viewModelScope.launch {
            val text = clipboardSyncManager.fetchText(ip, port, settings.pairingCode, uiState.value.clipboardChannel)
            if (text != null && clipboardSyncManager.shouldApplyRemoteText(text)) {
                clipboardSyncManager.rememberReceivedText(text)
                applyRemoteClipboardText(text)
            }
        }
    }

    private fun applyRemoteClipboardText(text: String) {
        updateState { copy(clipboardText = text) }
    }

    private fun applyConnectedOutcome(outcome: ConnectionOutcome.Connected) {
        settings.serverIp = outcome.ip
        settings.serverPort = outcome.port.toString()
        updateState {
            copy(
                serverIp = outcome.ip,
                serverPort = outcome.port.toString(),
                statusColor = GREEN,
                discoveryStatus = "",
                isGuestMode = outcome.isGuest,
                clipboardChannel = if (outcome.isGuest) "guest" else "",
                showTrustDialog = false,
                pendingTrustIp = "",
                pendingTrustHash = "",
                pendingTrustPort = outcome.port
            )
        }
        connectWebSocket()
        refreshFiles()
        startPolling()
    }

    private fun updateTrustRequest(outcome: ConnectionOutcome.TrustRequired) {
        updateState {
            copy(
                showTrustDialog = true,
                pendingTrustIp = outcome.ip,
                pendingTrustHash = outcome.certHash,
                pendingTrustPort = outcome.port
            )
        }
    }

    private fun updateNoLan() {
        updateState {
            copy(
                statusColor = GRAY,
                discoveryStatus = "No LAN"
            )
        }
    }

    private fun updateNotFound() {
        updateState {
            copy(
                statusColor = RED,
                discoveryStatus = "Server not found"
            )
        }
    }

    private fun updateDiscoveryFailure(message: String) {
        updateState {
            copy(
                statusColor = RED,
                discoveryStatus = "Discovery Error: $message"
            )
        }
    }

    private fun currentPort(): Int {
        return uiState.value.serverPort.toIntOrNull() ?: 26260
    }

    private fun updateState(block: MainUiState.() -> MainUiState) {
        _uiState.update { it.block() }
    }

    private companion object {
        const val GREEN = 0xFF4CAF50
        const val RED = 0xFFF44336
        const val YELLOW = 0xFFFFFF00
        const val GRAY = 0xFF888888
    }
}
