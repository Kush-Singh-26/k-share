package com.kush.kshare

import android.content.ClipData
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.core.content.IntentCompat
import androidx.lifecycle.lifecycleScope
import com.kush.kshare.api.ApiClient
import kotlinx.coroutines.launch
import java.util.ArrayList

class ShareActivity : ComponentActivity() {
    
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val sharedText = intent?.getStringExtra(Intent.EXTRA_TEXT)
        
        if (intent?.action == Intent.ACTION_SEND && sharedText != null) {
            setContent {
                val openDialog = remember { mutableStateOf(true) }
                if (openDialog.value) {
                    val displayPreview = if (sharedText.length > 100) sharedText.take(100) + "..." else sharedText
                    
                    AlertDialog(
                        onDismissRequest = { finish() },
                        title = { Text("K-Share Sync") },
                        text = { Text(displayPreview) },
                        confirmButton = {
                            if (sharedText.startsWith("http") || sharedText.contains("www.")) {
                                TextButton(onClick = { openOnPc(sharedText); openDialog.value = false }) {
                                    Text("Open on PC")
                                }
                            }
                            TextButton(onClick = { pushToClipboard(sharedText); openDialog.value = false }) {
                                Text("Sync Clipboard")
                            }
                        },
                        dismissButton = {
                            TextButton(onClick = { finish() }) {
                                Text("Cancel")
                            }
                        }
                    )
                }
            }
        } else {
            handleFileShare()
        }
    }

    private fun handleFileShare() {
        val intent = intent ?: return
        when (intent.action) {
            Intent.ACTION_SEND -> {
                IntentCompat.getParcelableExtra(intent, Intent.EXTRA_STREAM, Uri::class.java)?.let { uri ->
                    startTransferService(listOf(uri))
                }
            }
            Intent.ACTION_SEND_MULTIPLE -> {
                IntentCompat.getParcelableArrayListExtra(intent, Intent.EXTRA_STREAM, Uri::class.java)?.let { uris ->
                    startTransferService(uris)
                }
            }
        }
        Toast.makeText(this, "Uploading...", Toast.LENGTH_SHORT).show()
        finish()
    }

    private fun openOnPc(url: String) {
        val settings = SettingsManager(this)
        val ip = settings.serverIp
        val port = settings.serverPort.toIntOrNull() ?: 26260
        val code = settings.pairingCode
        if (ip.isEmpty()) { Toast.makeText(this, "Server IP not configured", Toast.LENGTH_SHORT).show(); finish(); return }
        lifecycleScope.launch {
            val success = ApiClient.openOnPc(ip, port, url, code)
            Toast.makeText(this@ShareActivity, if (success) "Opening on PC..." else "Open failed", Toast.LENGTH_SHORT).show()
            finish()
        }
    }

    private fun pushToClipboard(text: String) {
        val settings = SettingsManager(this)
        val ip = settings.serverIp
        val port = settings.serverPort.toIntOrNull() ?: 26260
        val code = settings.pairingCode
        if (ip.isEmpty()) { Toast.makeText(this, "Server IP not configured", Toast.LENGTH_SHORT).show(); finish(); return }
        lifecycleScope.launch {
            val success = ApiClient.postClipboard(ip, port, text, code, append = true)
            Toast.makeText(this@ShareActivity, if (success) "Synced to Laptop" else "Sync failed", Toast.LENGTH_SHORT).show()
            finish()
        }
    }
    
    private fun startTransferService(uris: List<Uri>) {
        if (uris.isEmpty()) return
        val settings = SettingsManager(this)
        val serviceIntent = Intent(this, FileTransferService::class.java).apply {
            action = FileTransferService.ACTION_UPLOAD
            putExtra(FileTransferService.EXTRA_SERVER_IP, settings.serverIp)
            putExtra(FileTransferService.EXTRA_SERVER_PORT, settings.serverPort.toIntOrNull() ?: 26260)
            putExtra(FileTransferService.EXTRA_PAIRING_CODE, settings.pairingCode)
            if (uris.size == 1) {
                putExtra(FileTransferService.EXTRA_URI, uris[0].toString())
                data = uris[0]
            } else {
                putParcelableArrayListExtra(FileTransferService.EXTRA_URIS, ArrayList(uris))
                val clipData = ClipData.newRawUri("Files", uris[0])
                for (i in 1 until uris.size) clipData.addItem(ClipData.Item(uris[i]))
                setClipData(clipData)
            }
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        startForegroundService(serviceIntent)
    }
}
