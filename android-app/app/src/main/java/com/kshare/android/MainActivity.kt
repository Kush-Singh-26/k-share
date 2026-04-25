package com.kshare.android

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import com.kshare.android.feature.history.HistoryDialog
import com.kshare.android.feature.history.HistoryViewModel
import com.kshare.android.feature.main.MainScreen
import com.kshare.android.feature.main.MainViewModel
import com.kshare.android.sync.DiscoverySyncScheduler

class MainActivity : ComponentActivity() {
    private val viewModel: MainViewModel by viewModels()
    private val historyViewModel: HistoryViewModel by viewModels()

    private val folderPickerLauncher =
        registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
            uri?.let {
                try {
                    contentResolver.takePersistableUriPermission(
                        it,
                        Intent.FLAG_GRANT_READ_URI_PERMISSION
                    )
                } catch (e: Exception) {
                    e.printStackTrace()
                }
                viewModel.uploadFolder(it)
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        if (Build.VERSION.SDK_INT >= 33) {
            val permissions = mutableListOf(Manifest.permission.POST_NOTIFICATIONS)
            if (Build.VERSION.SDK_INT >= 33) {
                permissions.add(Manifest.permission.READ_MEDIA_AUDIO)
                permissions.add(Manifest.permission.READ_MEDIA_IMAGES)
                permissions.add(Manifest.permission.READ_MEDIA_VIDEO)
            }
            val missing = permissions.filter { ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED }
            if (missing.isNotEmpty()) {
                ActivityCompat.requestPermissions(this, missing.toTypedArray(), Constants.PERMISSION_REQUEST_CODE)
            }
        }

        DiscoverySyncScheduler.schedule(this)

        setContent {
            val state by viewModel.uiState.collectAsState()
            val historyState by historyViewModel.uiState.collectAsState()
            val isDark = when (state.themeMode) {
                "light" -> false
                "dark" -> true
                else -> androidx.compose.foundation.isSystemInDarkTheme()
            }

            KShareTheme(themeMode = state.themeMode) {
                Surface(color = MaterialTheme.colorScheme.background, modifier = Modifier.fillMaxSize()) {
                    MainScreen(
                        state = state,
                        isDark = isDark,
                        thumbnailPairingCode = viewModel.getPairingCode(),
                        showTrustDialog = state.showTrustDialog,
                        pendingTrustIp = state.pendingTrustIp,
                        pendingTrustHash = state.pendingTrustHash,
                        onDiscoverServer = viewModel::discoverServer,
                        onOpenSettings = { startActivity(Intent(this@MainActivity, SettingsActivity::class.java)) },
                        onServerIpChange = viewModel::setServerIp,
                        onServerPortChange = viewModel::setServerPort,
                        onClipboardChannelChange = viewModel::setClipboardChannel,
                        onClipboardTextChange = viewModel::setClipboardText,
                        onPushClipboard = viewModel::pushClipboard,
                        onFetchClipboard = viewModel::fetchClipboard,
                        onCopyClipboard = viewModel::copyToPhoneClipboard,
                        onLoadHistory = {
                            historyViewModel.loadHistory(
                                serverIp = state.serverIp,
                                port = state.serverPort.toIntOrNull() ?: 26260,
                                pairingCode = viewModel.getPairingCode()
                            )
                        },
                        onUploadFolder = { folderPickerLauncher.launch(null) },
                        onRefreshFiles = viewModel::refreshFiles,
                        onSearchQueryChange = viewModel::setSearchQuery,
                        onVerifyManualIp = viewModel::verifyManualIp,
                        onDownloadFile = viewModel::downloadFile,
                        onRequestDeleteFile = viewModel::requestDeleteFile,
                        onConfirmDeleteFile = viewModel::confirmDeleteFile,
                        onDismissDeleteRequest = viewModel::clearDeleteRequest,
                        onAcceptTrust = viewModel::acceptPendingTrust,
                        onDismissTrust = viewModel::dismissTrustDialog,
                        onNavigateToFolder = viewModel::navigateToFolder,
                        onNavigateToPath = viewModel::navigateToPath,
                        onNavigateUp = viewModel::navigateUp
                    )
                }

                if (historyState.isVisible) {
                    HistoryDialog(
                        state = historyState,
                        isDark = isDark,
                        onDismiss = { historyViewModel.dismiss() },
                        onSelect = {
                            historyViewModel.dismiss()
                            viewModel.setClipboardText(it.text)
                            viewModel.pushClipboard()
                        },
                        onDelete = {
                            historyViewModel.deleteHistory(
                                serverIp = state.serverIp,
                                port = state.serverPort.toIntOrNull() ?: 26260,
                                pairingCode = viewModel.getPairingCode(),
                                item = it
                            )
                        }
                    )
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        viewModel.onResume()
    }

    override fun onPause() {
        super.onPause()
        viewModel.onPause()
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == Constants.PERMISSION_REQUEST_CODE) {
            val denied = permissions.zip(grantResults.toList()).filter { it.second != PackageManager.PERMISSION_GRANTED }
            if (denied.isNotEmpty()) {
                android.widget.Toast.makeText(this, "Some permissions were denied. File sharing may not work correctly.", android.widget.Toast.LENGTH_LONG).show()
            }
        }
    }
}
