package com.kush.kshare

import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp

class SettingsActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val settings = SettingsManager(this)
        
        setContent {
            MaterialTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    SettingsScreen(settings) {
                        Toast.makeText(this, "Settings saved", Toast.LENGTH_SHORT).show()
                        finish()
                    }
                }
            }
        }
    }
}

@Composable
fun SettingsScreen(settings: SettingsManager, onSave: () -> Unit) {
    var gistUrl by remember { mutableStateOf(settings.gistUrl) }
    var port by remember { mutableStateOf(settings.serverPort) }
    var jsonKey by remember { mutableStateOf(settings.gistJsonKey) }
    var pairingCode by remember { mutableStateOf(settings.pairingCode) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        Text("Settings", style = MaterialTheme.typography.headlineMedium)
        
        OutlinedTextField(
            value = gistUrl,
            onValueChange = { gistUrl = it },
            label = { Text("Gist Raw URL") },
            modifier = Modifier.fillMaxWidth()
        )
        
        OutlinedTextField(
            value = port,
            onValueChange = { port = it },
            label = { Text("Port (e.g. 26260)") },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.fillMaxWidth()
        )
        
        OutlinedTextField(
            value = jsonKey,
            onValueChange = { jsonKey = it },
            label = { Text("JSON Key (e.g. ip)") },
            modifier = Modifier.fillMaxWidth()
        )
        
        OutlinedTextField(
            value = pairingCode,
            onValueChange = { pairingCode = it },
            label = { Text("Pairing Code") },
            modifier = Modifier.fillMaxWidth()
        )
        
        Button(
            onClick = {
                settings.gistUrl = gistUrl.trim()
                settings.serverPort = port.trim()
                settings.gistJsonKey = jsonKey.trim()
                settings.pairingCode = pairingCode.trim()
                onSave()
            },
            modifier = Modifier.fillMaxWidth()
        ) {
            Text("Save Settings")
        }
    }
}
