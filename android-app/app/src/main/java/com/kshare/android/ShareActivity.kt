package com.kshare.android

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.collectAsState
import androidx.core.content.IntentCompat
import com.kshare.android.feature.share.ShareViewModel

class ShareActivity : ComponentActivity() {
    private val viewModel: ShareViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        viewModel.refresh()

        val sharedText = intent?.getStringExtra(Intent.EXTRA_TEXT)
        val hasFile = IntentCompat.getParcelableExtra(intent, Intent.EXTRA_STREAM, Uri::class.java) != null
            || IntentCompat.getParcelableArrayListExtra(intent, Intent.EXTRA_STREAM, Uri::class.java) != null

        if (intent?.action == Intent.ACTION_SEND && sharedText != null && !hasFile) {
            setContent {
                val state by viewModel.uiState.collectAsState()
                KShareTheme(themeMode = state.darkMode) {
                    val openDialog = remember { mutableStateOf(true) }
                    if (openDialog.value) {
                        val displayPreview = if (sharedText.length > 100) sharedText.take(100) + "..." else sharedText

                        AlertDialog(
                            onDismissRequest = { finish() },
                            title = { Text("K-Share Sync") },
                            text = { Text(displayPreview) },
                            confirmButton = {
                                if (sharedText.startsWith("http") || sharedText.contains("www.")) {
                                    TextButton(onClick = { viewModel.openOnPc(sharedText); openDialog.value = false }) {
                                        Text("Open on PC")
                                    }
                                }
                                TextButton(onClick = { viewModel.pushToClipboard(sharedText); openDialog.value = false }) {
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
                    viewModel.upload(uri)
                }
            }
            Intent.ACTION_SEND_MULTIPLE -> {
                IntentCompat.getParcelableArrayListExtra(intent, Intent.EXTRA_STREAM, Uri::class.java)?.let { uris ->
                    viewModel.upload(uris)
                }
            }
        }
        Toast.makeText(this, "Uploading...", Toast.LENGTH_SHORT).show()
        // Delay finish slightly so Toast has time to render
        window.decorView.postDelayed({ finish() }, 500)
    }
}
