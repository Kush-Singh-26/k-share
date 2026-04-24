package com.kshare.android.feature.main

import android.graphics.Bitmap
import android.text.util.Linkify
import android.widget.EditText
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import com.kshare.android.ThumbnailCache
import com.kshare.android.api.ApiClient
import com.kshare.android.api.RemoteFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

@Composable
fun MainScreen(
    state: MainUiState,
    isDark: Boolean,
    thumbnailPairingCode: String,
    showTrustDialog: Boolean,
    pendingTrustIp: String,
    pendingTrustHash: String,
    onDiscoverServer: () -> Unit,
    onOpenSettings: () -> Unit,
    onServerIpChange: (String) -> Unit,
    onServerPortChange: (String) -> Unit,
    onClipboardChannelChange: (String) -> Unit,
    onClipboardTextChange: (String) -> Unit,
    onPushClipboard: () -> Unit,
    onFetchClipboard: () -> Unit,
    onCopyClipboard: () -> Unit,
    onLoadHistory: () -> Unit,
    onUploadFolder: () -> Unit,
    onRefreshFiles: () -> Unit,
    onSearchQueryChange: (String) -> Unit,
    onVerifyManualIp: () -> Unit,
    onDownloadFile: (RemoteFile) -> Unit,
    onRequestDeleteFile: (RemoteFile) -> Unit,
    onConfirmDeleteFile: () -> Unit,
    onDismissDeleteRequest: () -> Unit,
    onAcceptTrust: () -> Unit,
    onDismissTrust: () -> Unit,
) {
    if (showTrustDialog) {
        AlertDialog(
            onDismissRequest = onDismissTrust,
            title = { Text("New Server Found") },
            text = {
                Column {
                    Text("Do you trust this server?")
                    Spacer(Modifier.height(8.dp))
                    Text("IP: $pendingTrustIp", fontWeight = FontWeight.Bold)
                    Text("Hash: ${pendingTrustHash.take(16)}...", fontSize = 12.sp, color = Color.Gray)
                }
            },
            confirmButton = {
                Button(onClick = onAcceptTrust) {
                    Text("Trust & Connect")
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissTrust) {
                    Text("Cancel")
                }
            }
        )
    }

    if (state.fileToDelete != null) {
        AlertDialog(
            onDismissRequest = onDismissDeleteRequest,
            title = { Text("Delete File?") },
            text = { Text("Are you sure you want to delete '${state.fileToDelete?.name}'?\nIt will be moved to the trash folder.") },
            confirmButton = {
                Button(
                    onClick = onConfirmDeleteFile,
                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error)
                ) { Text("Delete") }
            },
            dismissButton = {
                TextButton(onClick = onDismissDeleteRequest) { Text("Cancel") }
            }
        )
    }

    Column(modifier = Modifier.systemBarsPadding().padding(16.dp)) {
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            elevation = CardDefaults.cardElevation(2.dp),
            modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp)
        ) {
            Column(modifier = Modifier.padding(12.dp)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text("K-Share", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.primary)
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            modifier = Modifier
                                .size(8.dp)
                                .clip(CircleShape)
                                .background(Color(state.statusColor))
                        )
                        Spacer(Modifier.width(12.dp))
                        IconButton(onClick = onDiscoverServer, modifier = Modifier.size(24.dp)) {
                            Icon(Icons.Default.Refresh, "Re-Discover", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Spacer(Modifier.width(12.dp))
                        IconButton(onClick = onOpenSettings, modifier = Modifier.size(24.dp)) {
                            Icon(Icons.Default.Menu, "Config", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }

                if (state.discoveryStatus.isNotEmpty()) {
                    Text(
                        text = state.discoveryStatus,
                        fontSize = 11.sp,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 4.dp)
                    )
                }

                Spacer(modifier = Modifier.height(8.dp))

                Row(verticalAlignment = Alignment.CenterVertically) {
                    OutlinedTextField(
                        value = state.serverIp,
                        onValueChange = onServerIpChange,
                        label = { Text("Server IP", fontSize = 12.sp) },
                        modifier = Modifier.weight(1f).padding(end = 8.dp),
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodyMedium,
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = { onVerifyManualIp() })
                    )
                    OutlinedTextField(
                        value = state.serverPort,
                        onValueChange = onServerPortChange,
                        label = { Text("Port", fontSize = 12.sp) },
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = { onVerifyManualIp() }),
                        modifier = Modifier.width(80.dp),
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodyMedium
                    )
                    Spacer(Modifier.width(8.dp))
                    IconButton(
                        onClick = onVerifyManualIp,
                        modifier = Modifier
                            .size(40.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(MaterialTheme.colorScheme.primaryContainer)
                    ) {
                        Icon(
                            painter = painterResource(android.R.drawable.ic_menu_send),
                            contentDescription = "Connect",
                            tint = MaterialTheme.colorScheme.onPrimaryContainer,
                            modifier = Modifier.size(20.dp)
                        )
                    }
                }
            }
        }

        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            elevation = CardDefaults.cardElevation(2.dp),
            modifier = Modifier.weight(1f).fillMaxWidth().padding(vertical = 4.dp)
        ) {
            Column(modifier = Modifier.padding(12.dp)) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text("SHARED CLIPBOARD", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, letterSpacing = 1.sp)
                    if (!state.isGuestMode) {
                        Row(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            modifier = Modifier.padding(end = 8.dp)
                        ) {
                            val isGuest = state.clipboardChannel == "guest"
                            Text(
                                "Private",
                                fontSize = 12.sp,
                                fontWeight = if (!isGuest) FontWeight.Bold else FontWeight.Normal,
                                color = if (!isGuest) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.clickable { onClipboardChannelChange("") }
                            )
                            Text("|", fontSize = 12.sp, color = MaterialTheme.colorScheme.outline)
                            Text(
                                "Guest",
                                fontSize = 12.sp,
                                fontWeight = if (isGuest) FontWeight.Bold else FontWeight.Normal,
                                color = if (isGuest) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.clickable { onClipboardChannelChange("guest") }
                            )
                        }
                    } else {
                        Text("Guest Mode", fontSize = 12.sp, color = MaterialTheme.colorScheme.tertiary)
                    }
                }

                Row(
                    modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        TextButton(onClick = onPushClipboard, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("Push", fontSize = 12.sp) }
                        TextButton(onClick = onFetchClipboard, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("Fetch", fontSize = 12.sp) }
                        TextButton(onClick = onCopyClipboard, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("Copy", fontSize = 12.sp) }
                        TextButton(onClick = onLoadHistory, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("History", fontSize = 12.sp) }
                    }
                }

                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(top = 4.dp)
                        .border(1.dp, MaterialTheme.colorScheme.outline, RoundedCornerShape(4.dp))
                        .padding(8.dp)
                ) {
                    AndroidView(
                        factory = { ctx ->
                            EditText(ctx).apply {
                                background = null
                                textSize = 14f
                                setTextColor(if (isDark) android.graphics.Color.WHITE else android.graphics.Color.BLACK)
                                gravity = android.view.Gravity.TOP or android.view.Gravity.START
                                inputType = android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE
                                autoLinkMask = Linkify.WEB_URLS
                                linksClickable = true
                                movementMethod = android.text.method.LinkMovementMethod.getInstance()
                                addTextChangedListener(object : android.text.TextWatcher {
                                    override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
                                    override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                                        onClipboardTextChange(s.toString())
                                    }
                                    override fun afterTextChanged(s: android.text.Editable?) {}
                                })
                            }
                        },
                        update = { view ->
                            if (view.text.toString() != state.clipboardText) {
                                view.setText(state.clipboardText)
                            }
                            view.setTextColor(if (isDark) android.graphics.Color.WHITE else android.graphics.Color.BLACK)
                        },
                        modifier = Modifier.fillMaxSize()
                    )
                }
            }
        }

        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            elevation = CardDefaults.cardElevation(2.dp),
            modifier = Modifier.weight(1f).fillMaxWidth().padding(vertical = 4.dp)
        ) {
            Column {
                Column(modifier = Modifier.padding(12.dp).fillMaxWidth()) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Files", fontWeight = FontWeight.Bold, fontSize = 16.sp)
                        Row {
                            IconButton(onClick = onUploadFolder, modifier = Modifier.size(24.dp)) {
                                Icon(Icons.Default.Add, "Upload Folder", tint = MaterialTheme.colorScheme.primary)
                            }
                            Spacer(Modifier.width(16.dp))
                            IconButton(onClick = onRefreshFiles, modifier = Modifier.size(24.dp)) {
                                Icon(Icons.Default.Refresh, "Refresh")
                            }
                        }
                    }
                    Spacer(modifier = Modifier.height(8.dp))
                    OutlinedTextField(
                        value = state.searchQuery,
                        onValueChange = onSearchQueryChange,
                        label = { Text("Search files...", fontSize = 12.sp) },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodyMedium,
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search)
                    )
                }
                if (state.isRefreshing) {
                    LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                }
                LazyColumn(modifier = Modifier.padding(horizontal = 4.dp)) {
                    items(state.fileList) { file ->
                        FileItem(
                            file = file,
                            serverIp = state.serverIp,
                            serverPort = state.serverPort,
                            pairingCode = thumbnailPairingCode,
                            isGuestMode = state.isGuestMode,
                            onDownloadFile = onDownloadFile,
                            onRequestDeleteFile = onRequestDeleteFile
                        )
                    }
                }
            }
        }
    }
}
private fun formatSize(bytes: Long): String = when {
    bytes < 1024L -> "$bytes B"
    bytes < 1024L * 1024L -> "${bytes / 1024} KB"
    else -> "${"%.1f".format(bytes / (1024.0 * 1024.0))} MB"
}

@Composable
private fun FileItem(
    file: RemoteFile,
    serverIp: String,
    serverPort: String,
    pairingCode: String,
    isGuestMode: Boolean,
    onDownloadFile: (RemoteFile) -> Unit,
    onRequestDeleteFile: (RemoteFile) -> Unit
) {
    var thumbnail by remember { mutableStateOf<Bitmap?>(null) }

    LaunchedEffect(file.name, serverIp) {
        if (!file.isDir && file.name.lowercase().let { it.endsWith(".jpg") || it.endsWith(".png") }) {
            val url = ApiClient.getThumbnailUrl(
                serverIp,
                serverPort.toIntOrNull() ?: 0,
                file.name
            )
            val key = "${serverIp}_${file.name}"
            thumbnail = withContext(Dispatchers.IO) {
                ThumbnailCache.get(key, url, pairingCode)
            }
        }
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onDownloadFile(file) }
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        if (file.isDir) {
            Icon(
                painter = painterResource(android.R.drawable.ic_menu_directions),
                contentDescription = null,
                modifier = Modifier.size(48.dp).padding(8.dp),
                tint = Color(0xFFFFC107)
            )
        } else if (thumbnail != null) {
            Image(
                bitmap = thumbnail!!.asImageBitmap(),
                contentDescription = null,
                modifier = Modifier.size(48.dp).background(MaterialTheme.colorScheme.surfaceVariant)
            )
        } else {
            Icon(
                painter = painterResource(android.R.drawable.ic_menu_save),
                contentDescription = null,
                modifier = Modifier.size(48.dp).padding(8.dp),
                tint = MaterialTheme.colorScheme.primary
            )
        }
        Column(modifier = Modifier.padding(start = 12.dp).weight(1f)) {
            var displayName = file.name
            val isGuest = displayName.startsWith("Public/")
            if (isGuest) displayName = displayName.removePrefix("Public/")

            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(displayName, fontWeight = FontWeight.Bold, maxLines = 1, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.weight(1f, false))
                if (isGuest) {
                    Spacer(Modifier.width(8.dp))
                    Text(
                        "Guest",
                        fontSize = 10.sp,
                        color = MaterialTheme.colorScheme.tertiary,
                        modifier = Modifier
                            .border(1.dp, MaterialTheme.colorScheme.tertiary, RoundedCornerShape(4.dp))
                            .padding(horizontal = 4.dp, vertical = 1.dp)
                    )
                }
            }
            Text(
                if (file.isDir) "Folder • ${file.modTime}" else "${formatSize(file.size)} • ${file.modTime}",
                fontSize = 12.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }

        if (!isGuestMode) {
            IconButton(onClick = { onRequestDeleteFile(file) }) {
                Icon(Icons.Default.Delete, "Delete", tint = MaterialTheme.colorScheme.error)
            }
        }
    }
}
