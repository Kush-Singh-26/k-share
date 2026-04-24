package com.kshare.android

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.documentfile.provider.DocumentFile
import com.kshare.android.feature.settings.SettingsUiState
import com.kshare.android.feature.settings.SettingsViewModel

class SettingsActivity : ComponentActivity() {
    private val viewModel: SettingsViewModel by viewModels()

    private val downloadFolderLauncher = registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        uri?.let {
            contentResolver.takePersistableUriPermission(
                it,
                Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION
            )
            viewModel.setDownloadUri(it.toString())
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            val state by viewModel.uiState.collectAsState()
            KShareTheme(themeMode = state.themeMode) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    SettingsScreen(
                        state = state,
                        onPickFolder = { downloadFolderLauncher.launch(null) },
                        onPairingCodeChange = {
                            viewModel.setPairingCode(it)
                            viewModel.save()
                        },
                        onThemeModeChange = {
                            viewModel.setThemeMode(it)
                            viewModel.save()
                        },
                        onResetFolder = viewModel::clearDownloadUri,
                        onDeleteSavedIp = viewModel::removeSavedIp,
                        onSave = {
                            viewModel.save()
                            finish()
                        }
                    )
                }
            }
        }
    }
}

@Composable
fun SettingsScreen(
    state: SettingsUiState,
    onPickFolder: () -> Unit,
    onPairingCodeChange: (String) -> Unit,
    onThemeModeChange: (String) -> Unit,
    onResetFolder: () -> Unit,
    onDeleteSavedIp: (String) -> Unit,
    onSave: () -> Unit
) {
    val context = LocalContext.current
    val downloadFolderName = remember(state.downloadUri) {
        if (state.downloadUri.isEmpty()) {
            "Default (Downloads)"
        } else {
            try {
                val doc = DocumentFile.fromTreeUri(context, Uri.parse(state.downloadUri))
                doc?.name ?: "Selected Folder"
            } catch (e: Exception) {
                "Access Lost"
            }
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .systemBarsPadding()
            .padding(16.dp)
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        Text("Settings", style = MaterialTheme.typography.headlineMedium)

        Text("Theme", style = MaterialTheme.typography.labelLarge)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("system", "light", "dark").forEach { mode ->
                FilterChip(
                    selected = state.themeMode == mode,
                    onClick = { onThemeModeChange(mode) },
                    label = { Text(mode.replaceFirstChar { it.uppercase() }) }
                )
            }
        }

        Text("Download Location", style = MaterialTheme.typography.labelLarge)
        OutlinedCard(modifier = Modifier.fillMaxWidth()) {
            Column(modifier = Modifier.padding(12.dp)) {
                Text(downloadFolderName, style = MaterialTheme.typography.bodyLarge)
                Spacer(Modifier.height(8.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = onPickFolder) { Text("Change") }
                    if (state.downloadUri.isNotEmpty()) {
                        TextButton(onClick = onResetFolder) { Text("Reset") }
                    }
                }
            }
        }

        OutlinedTextField(
            value = state.pairingCode,
            onValueChange = onPairingCodeChange,
            label = { Text("Pairing Code") },
            modifier = Modifier.fillMaxWidth()
        )

        if (state.savedIps.isNotEmpty()) {
            Text("Saved Networks", style = MaterialTheme.typography.labelLarge)
            OutlinedCard(modifier = Modifier.fillMaxWidth()) {
                Column(modifier = Modifier.padding(12.dp)) {
                    state.savedIps.forEach { (networkId, ip) ->
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text("$networkId.* → $ip", style = MaterialTheme.typography.bodyMedium)
                            }
                            IconButton(onClick = { onDeleteSavedIp(networkId) }) {
                                Icon(
                                    Icons.Default.Delete,
                                    contentDescription = "Delete",
                                    tint = MaterialTheme.colorScheme.error
                                )
                            }
                        }
                    }
                }
            }
        }

        Button(
            onClick = onSave,
            modifier = Modifier.fillMaxWidth()
        ) {
            Text("Save Settings")
        }
    }
}
