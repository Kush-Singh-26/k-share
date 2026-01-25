package com.kush.kshare

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.documentfile.provider.DocumentFile

class SettingsActivity : ComponentActivity() {
    private lateinit var settings: SettingsManager
    
    private val downloadFolderLauncher = registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        uri?.let {
            contentResolver.takePersistableUriPermission(it, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
            settings.downloadUri = it.toString()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settings = SettingsManager(this)
        
        setContent {
            KShareTheme(themeMode = settings.darkMode) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    SettingsScreen(settings, { downloadFolderLauncher.launch(null) }) {
                        Toast.makeText(this, "Settings saved", Toast.LENGTH_SHORT).show()
                        finish()
                    }
                }
            }
        }
    }
}

@Composable
fun SettingsScreen(settings: SettingsManager, onPickFolder: () -> Unit, onSave: () -> Unit) {
    var pairingCode by remember { mutableStateOf(settings.pairingCode) }
    var themeMode by remember { mutableStateOf(settings.darkMode) }
    
    val context = LocalContext.current
    val downloadFolderName = remember(settings.downloadUri) {
        if (settings.downloadUri.isEmpty()) "Default (Downloads)"
        else {
            try {
                val doc = DocumentFile.fromTreeUri(context, Uri.parse(settings.downloadUri))
                doc?.name ?: "Selected Folder"
            } catch (e: Exception) { "Access Lost" }
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp)
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        Text("Settings", style = MaterialTheme.typography.headlineMedium)
        
        Text("Theme", style = MaterialTheme.typography.labelLarge)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("system", "light", "dark").forEach { mode ->
                FilterChip(
                    selected = themeMode == mode,
                    onClick = { themeMode = mode },
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
                    if (settings.downloadUri.isNotEmpty()) {
                        TextButton(onClick = { settings.downloadUri = "" }) { Text("Reset") }
                    }
                }
            }
        }
        
        OutlinedTextField(
            value = pairingCode,
            onValueChange = { pairingCode = it },
            label = { Text("Pairing Code") },
            modifier = Modifier.fillMaxWidth()
        )
        
        // Saved Networks Section
        var savedIps by remember { mutableStateOf(settings.getAllSavedIps()) }
        
        if (savedIps.isNotEmpty()) {
            Text("Saved Networks", style = MaterialTheme.typography.labelLarge)
            OutlinedCard(modifier = Modifier.fillMaxWidth()) {
                Column(modifier = Modifier.padding(12.dp)) {
                    savedIps.forEach { (networkId, ip) ->
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text("$networkId.* → $ip", style = MaterialTheme.typography.bodyMedium)
                            }
                            IconButton(
                                onClick = {
                                    settings.removeServerIp(networkId)
                                    savedIps = settings.getAllSavedIps()
                                }
                            ) {
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
            onClick = {
                settings.pairingCode = pairingCode.trim()
                settings.darkMode = themeMode
                onSave()
            },
            modifier = Modifier.fillMaxWidth()
        ) {
            Text("Save Settings")
        }
    }
}
