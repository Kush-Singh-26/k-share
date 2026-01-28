package com.kush.kshare

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.net.Uri
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Build
import android.os.Bundle
import android.text.util.Linkify
import android.util.Patterns
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.ui.viewinterop.AndroidView

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.*
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import androidx.work.*
import com.kush.kshare.api.ApiClient
import com.kush.kshare.api.HistoryItem
import com.kush.kshare.api.RemoteFile
import kotlinx.coroutines.*
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.util.concurrent.TimeUnit

class MainActivity : ComponentActivity() {

    private lateinit var settings: SettingsManager
    private lateinit var client: OkHttpClient
    private var webSocket: WebSocket? = null
    private var pollingJob: Job? = null
    
    // State
    private var serverIpState = mutableStateOf("")
    private var serverPortState = mutableStateOf("26260")
    private var statusColorState = mutableStateOf(Color.Yellow)
    private var clipboardTextState = mutableStateOf("")
    private var clipboardChannelState = mutableStateOf("") // "" = private, "guest" = guest
    private var isGuestModeState = mutableStateOf(false)
    private var lastClipboardSync = ""
    private var lastUserEditTime = 0L
    private var fileListState = mutableStateOf<List<RemoteFile>>(emptyList())
    private var isRefreshingState = mutableStateOf(false)
    private var showHistoryDialog = mutableStateOf(false)
    private var historyList = mutableStateOf<List<HistoryItem>>(emptyList())
    private var themeModeState = mutableStateOf("system")
    private var discoveryStatusState = mutableStateOf("")
    
    // Trust Dialog State
    private var showTrustDialog = mutableStateOf(false)
    private var pendingTrustIp = mutableStateOf("")
    private var pendingTrustHash = mutableStateOf("")
    private var pendingTrustPort = 26260


    private val folderPickerLauncher = registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        uri?.let { 
            try {
                contentResolver.takePersistableUriPermission(it, Intent.FLAG_GRANT_READ_URI_PERMISSION)
            } catch (e: Exception) { e.printStackTrace() }
            uploadFolder(it) 
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settings = SettingsManager(this)
        ApiClient.init(this) // Initialize API Client with Context
        client = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .sslSocketFactory(ApiClient.getSslSocketFactory(), ApiClient.getTrustManager())
            .build()
        ThumbnailCache.init(this)

        serverIpState.value = settings.serverIp
        serverPortState.value = settings.serverPort.ifEmpty { "26260" }
        themeModeState.value = settings.darkMode

        setContent {
            KShareTheme(themeMode = themeModeState.value) {
                Surface(color = MaterialTheme.colorScheme.background, modifier = Modifier.fillMaxSize()) {
                    MainScreen()
                }
                if (showHistoryDialog.value) {
                    HistoryDialog(
                        items = historyList.value,
                        onDismiss = { showHistoryDialog.value = false },
                        onSelect = { 
                            clipboardTextState.value = it.text
                            pushClipboard()
                            showHistoryDialog.value = false
                        },
                        onDelete = { item ->
                            lifecycleScope.launch {
                                val ip = serverIpState.value
                                val port = serverPortState.value.toIntOrNull() ?: 26260
                                if (ApiClient.deleteHistory(ip, port, item.id, settings.pairingCode)) {
                                    historyList.value = historyList.value.filter { it.id != item.id }
                                }
                            }
                        }
                    )
                }
                
                if (showTrustDialog.value) {
                    AlertDialog(
                        onDismissRequest = { 
                            showTrustDialog.value = false 
                            discoveryStatusState.value = "Connection cancelled"
                        },
                        title = { Text("New Server Found") },
                        text = { 
                            Column {
                                Text("Do you trust this server?")
                                Spacer(Modifier.height(8.dp))
                                Text("IP: ${pendingTrustIp.value}", fontWeight = FontWeight.Bold)
                                Text("Hash: ${pendingTrustHash.value.take(16)}...", fontSize = 12.sp, color = Color.Gray)
                            }
                        },
                        confirmButton = {
                            Button(onClick = {
                                // Save as known server
                                settings.saveServer(pendingTrustHash.value, pendingTrustIp.value, pendingTrustPort, settings.pairingCode)
                                showTrustDialog.value = false
                                connectToPendingServer()
                            }) { Text("Trust & Connect") }
                        },
                        dismissButton = {
                            TextButton(onClick = { 
                                showTrustDialog.value = false
                                discoveryStatusState.value = "Connection cancelled"
                            }) { Text("Cancel") }
                        }
                    )
                }
            }
        }

        if (Build.VERSION.SDK_INT >= 33) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.POST_NOTIFICATIONS), 101)
            }
        }
        scheduleBackgroundSync()
    }

    override fun onResume() {
        super.onResume()
        serverIpState.value = settings.serverIp
        serverPortState.value = settings.serverPort.ifEmpty { "26260" }
        themeModeState.value = settings.darkMode
        
        // Context-Aware Auto-Connect
        tryAutoConnect()
    }

    override fun onPause() {
        super.onPause()
        closeWebSocket()
        stopPolling()
    }

    // Discovery status text
    private var fileToDeleteState = mutableStateOf<RemoteFile?>(null)

    @Composable
    fun MainScreen() {
        val isDark = when (themeModeState.value) {
            "light" -> false
            "dark" -> true
            else -> isSystemInDarkTheme()
        }
        
        if (fileToDeleteState.value != null) {
             AlertDialog(
                 onDismissRequest = { fileToDeleteState.value = null },
                 title = { Text("Delete File?") },
                 text = { Text("Are you sure you want to delete '${fileToDeleteState.value?.name}'?\nIt will be moved to the trash folder.") },
                 confirmButton = {
                     Button(
                         onClick = {
                             val file = fileToDeleteState.value
                             if (file != null) {
                                 lifecycleScope.launch {
                                     val ip = serverIpState.value
                                     val port = serverPortState.value.toIntOrNull() ?: 26260
                                     if (ApiClient.deleteFile(ip, port, file.name, settings.pairingCode)) {
                                         Toast.makeText(this@MainActivity, "Deleted", Toast.LENGTH_SHORT).show()
                                         refreshFileList()
                                     } else {
                                         Toast.makeText(this@MainActivity, "Delete failed", Toast.LENGTH_SHORT).show()
                                     }
                                 }
                             }
                             fileToDeleteState.value = null
                         },
                         colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error)
                     ) { Text("Delete") }
                 },
                 dismissButton = {
                     TextButton(onClick = { fileToDeleteState.value = null }) { Text("Cancel") }
                 }
             )
        }

        Column(modifier = Modifier.padding(16.dp)) {
            // Settings Card
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
                                    .background(statusColorState.value)
                            )
                            Spacer(Modifier.width(12.dp))
                            IconButton(onClick = { discoverServer() }, modifier = Modifier.size(24.dp)) {
                                Icon(Icons.Default.Refresh, "Re-Discover", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                            Spacer(Modifier.width(12.dp))
                            IconButton(onClick = { startActivity(Intent(this@MainActivity, SettingsActivity::class.java)) }, modifier = Modifier.size(24.dp)) {
                                Icon(Icons.Default.Menu, "Config", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                    
                    // Discovery status text
                    if (discoveryStatusState.value.isNotEmpty()) {
                        Text(
                            text = discoveryStatusState.value,
                            fontSize = 11.sp,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(top = 4.dp)
                        )
                    }
                    
                    Spacer(modifier = Modifier.height(8.dp))

                    Row(verticalAlignment = Alignment.CenterVertically) {
                        OutlinedTextField(
                            value = serverIpState.value,
                            onValueChange = { 
                                serverIpState.value = it
                                settings.serverIp = it
                            },
                            label = { Text("Server IP", fontSize = 12.sp) },
                            modifier = Modifier.weight(1f).padding(end = 8.dp),
                            singleLine = true,
                            textStyle = MaterialTheme.typography.bodyMedium,
                            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                            keyboardActions = KeyboardActions(
                                onDone = { verifyManualIp() }
                            )
                        )
                        OutlinedTextField(
                            value = serverPortState.value,
                            onValueChange = { 
                                serverPortState.value = it
                                settings.serverPort = it
                                connectWebSocket()
                            },
                            label = { Text("Port", fontSize = 12.sp) },
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                            modifier = Modifier.width(80.dp),
                            singleLine = true,
                            textStyle = MaterialTheme.typography.bodyMedium
                        )
                    }
                }
            }

            // Clipboard Card
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
                        // Channel Toggle (Only visible if NOT guest mode)
                        if (!isGuestModeState.value) {
                            Row(
                                horizontalArrangement = Arrangement.spacedBy(8.dp),
                                modifier = Modifier.padding(end = 8.dp)
                            ) {
                                 val isGuest = clipboardChannelState.value == "guest"
                                 Text(
                                     "Private", 
                                     fontSize = 12.sp, 
                                     fontWeight = if(!isGuest) FontWeight.Bold else FontWeight.Normal,
                                     color = if(!isGuest) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                                     modifier = Modifier.clickable { 
                                         clipboardChannelState.value = "" 
                                         fetchClipboard()
                                     }
                                 )
                                 Text("|", fontSize = 12.sp, color = MaterialTheme.colorScheme.outline)
                                 Text(
                                     "Guest", 
                                     fontSize = 12.sp, 
                                     fontWeight = if(isGuest) FontWeight.Bold else FontWeight.Normal,
                                     color = if(isGuest) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                                     modifier = Modifier.clickable { 
                                         clipboardChannelState.value = "guest" 
                                         fetchClipboard()
                                     }
                                 )
                            }
                        } else {
                            Text("Guest Mode", fontSize = 12.sp, color = MaterialTheme.colorScheme.tertiary)
                        }
                    }

                    Row(
                        modifier = Modifier.fillMaxWidth().padding(top=4.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                            TextButton(onClick = { pushClipboard() }, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("Push", fontSize = 12.sp) }
                            TextButton(onClick = { fetchClipboard() }, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("Fetch", fontSize = 12.sp) }
                            TextButton(onClick = { copyToPhoneClipboard() }, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("Copy", fontSize = 12.sp) }
                            TextButton(onClick = { loadHistory() }, modifier = Modifier.height(30.dp), contentPadding = PaddingValues(horizontal = 8.dp)) { Text("History", fontSize = 12.sp) }
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
                                            clipboardTextState.value = s.toString()
                                            lastUserEditTime = System.currentTimeMillis()
                                        }
                                        override fun afterTextChanged(s: android.text.Editable?) {}
                                    })
                                }
                            },
                            update = { view ->
                                if (view.text.toString() != clipboardTextState.value) {
                                    view.setText(clipboardTextState.value)
                                }
                                view.setTextColor(if (isDark) android.graphics.Color.WHITE else android.graphics.Color.BLACK)
                            },
                            modifier = Modifier.fillMaxSize()
                        )
                    }
                }
            }

            // File List Card
            Card(
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                elevation = CardDefaults.cardElevation(2.dp),
                modifier = Modifier.weight(1f).fillMaxWidth().padding(vertical = 4.dp)
            ) {
                Column {
                    Row(
                        modifier = Modifier.padding(12.dp).fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Files", fontWeight = FontWeight.Bold, fontSize = 16.sp)
                        Row {
                            IconButton(onClick = { folderPickerLauncher.launch(null) }, modifier = Modifier.size(24.dp)) {
                                Icon(Icons.Default.Add, "Upload Folder", tint = MaterialTheme.colorScheme.primary)
                            }
                            Spacer(Modifier.width(16.dp))
                            IconButton(onClick = { refreshFileList() }, modifier = Modifier.size(24.dp)) {
                                Icon(Icons.Default.Refresh, "Refresh")
                            }
                        }
                    }
                    if (isRefreshingState.value) {
                        LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                    }
                    LazyColumn(modifier = Modifier.padding(horizontal = 4.dp)) {
                        items(fileListState.value) { file ->
                            FileItem(file)
                        }
                    }
                }
            }
        }
    }

    @Composable
    fun FileItem(file: RemoteFile) {
        var thumbnail by remember { mutableStateOf<Bitmap?>(null) }
        
        LaunchedEffect(file.name) {
            if (!file.isDir && file.name.lowercase().let { it.endsWith(".jpg") || it.endsWith(".png") }) {
                val url = ApiClient.getThumbnailUrl(serverIpState.value, serverPortState.value.toIntOrNull()?:0, file.name)
                val key = "${serverIpState.value}_${file.name}"
                val cached = ThumbnailCache.getFromMemory(key)
                if (cached != null) thumbnail = cached
                else withContext(Dispatchers.IO) {
                    thumbnail = ThumbnailCache.get(key, url, settings.pairingCode)
                }
            }
        }

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable { downloadFile(file) }
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
                // Parse Name for Guest Label
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
                Text(if (file.isDir) "Folder • ${file.modTime}" else "${file.size / 1024} KB • ${file.modTime}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            
            // Delete Button (if not guest)
            if (!isGuestModeState.value) {
                IconButton(onClick = { fileToDeleteState.value = file }) {
                    Icon(Icons.Default.Delete, "Delete", tint = MaterialTheme.colorScheme.error)
                }
            }
        }
    }

    @Composable
    fun HistoryDialog(
        items: List<HistoryItem>,
        onDismiss: () -> Unit,
        onSelect: (HistoryItem) -> Unit,
        onDelete: (HistoryItem) -> Unit
    ) {
        val isDark = when (themeModeState.value) {
            "light" -> false
            "dark" -> true
            else -> isSystemInDarkTheme()
        }
        Dialog(onDismissRequest = onDismiss) {
            Card(
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                modifier = Modifier.fillMaxWidth(0.95f).heightIn(max = 500.dp)
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text("History", style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.onSurface)
                    Spacer(Modifier.height(16.dp))
                    LazyColumn {
                        items(items) { item ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable { onSelect(item) }
                                    .padding(vertical = 8.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                AndroidView(
                                    factory = { ctx ->
                                        TextView(ctx).apply {
                                            textSize = 16f
                                            setTextColor(if (isDark) android.graphics.Color.WHITE else android.graphics.Color.BLACK)
                                            autoLinkMask = Linkify.WEB_URLS
                                            linksClickable = true
                                            movementMethod = android.text.method.LinkMovementMethod.getInstance()
                                        }
                                    },
                                    update = { view ->
                                        view.text = item.text
                                        view.setTextColor(if (isDark) android.graphics.Color.WHITE else android.graphics.Color.BLACK)
                                    },
                                    modifier = Modifier.weight(1f)
                                )
                                IconButton(onClick = { onDelete(item) }) {
                                    Icon(Icons.Default.Delete, "Delete", tint = MaterialTheme.colorScheme.error)
                                }
                            }
                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                        }
                    }
                    Button(onClick = onDismiss, modifier = Modifier.fillMaxWidth().padding(top = 16.dp)) {
                        Text("Close")
                    }
                }
            }
        }
    }

    // Logic Methods
    private fun connectWebSocket() {
        closeWebSocket()
        val ip = serverIpState.value; val port = serverPortState.value
        if (ip.isEmpty()) return
        val request = Request.Builder().url("wss://$ip:$port/ws").build()
        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                statusColorState.value = Color(0xFF4CAF50)
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                val type = JSONObject(text).optString("type")
                lifecycleScope.launch(Dispatchers.Main) {
                    when (type) {
                        "clip" -> {
                            // Only fetch if user hasn't edited locally in the last 5 seconds
                            if (System.currentTimeMillis() - lastUserEditTime > 5000) {
                                fetchClipboard()
                            }
                        }
                        "files" -> refreshFileList()
                    }
                }
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                statusColorState.value = Color(0xFFF44336)
            }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                statusColorState.value = Color(0xFFF44336)
                lifecycleScope.launch { delay(5000); if (webSocket == this@MainActivity.webSocket) connectWebSocket() }
            }
        })
    }

    private fun closeWebSocket() { webSocket?.close(1000, null); webSocket = null }

    private fun startPolling() {
        stopPolling()
        pollingJob = lifecycleScope.launch {
            while (isActive) {
                if (NetworkScanner.hasValidLan(this@MainActivity)) {
                    val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
                    if (ip.isNotEmpty()) {
                        val online = ApiClient.ping(ip, port).success
                        statusColorState.value = if (online) Color(0xFF4CAF50) else Color(0xFFF44336)
                        
                        if (!online && ApiClient.lastError != null) {
                           runOnUiThread { Toast.makeText(this@MainActivity, "Error: ${ApiClient.lastError}", Toast.LENGTH_SHORT).show() }
                        }

                        if (online && System.currentTimeMillis() - lastUserEditTime > 5000) fetchClipboardSilently(ip, port)
                    }
                } else statusColorState.value = Color.Gray
                delay(15000)
            }
        }
    }

    private fun stopPolling() { pollingJob?.cancel(); pollingJob = null }

    private fun scheduleBackgroundSync() {
        val constraints = Constraints.Builder().setRequiredNetworkType(NetworkType.UNMETERED).build()
        val request = PeriodicWorkRequestBuilder<SyncWorker>(15, TimeUnit.MINUTES).setConstraints(constraints).build()
        WorkManager.getInstance(this).enqueueUniquePeriodicWork("DiscoverySync", ExistingPeriodicWorkPolicy.KEEP, request)
    }

    private fun tryAutoConnect() {
        lifecycleScope.launch {
            if (!NetworkScanner.hasValidLan(this@MainActivity)) {
                statusColorState.value = Color.Gray
                discoveryStatusState.value = "No LAN"
                return@launch
            }
            
            val port = serverPortState.value.toIntOrNull() ?: 26260
            val networkId = NetworkScanner.getNetworkId(this@MainActivity)
            
            if (networkId != null) {
                val cachedIp = settings.getLastServerIp(networkId)
                if (cachedIp != null) {
                    discoveryStatusState.value = "Connecting..."
                    verifyAndConnect(cachedIp, port, true) // silent mode
                    return@launch
                }
            }
            
            if (serverIpState.value.isNotEmpty()) {
                verifyAndConnect(serverIpState.value, port, true)
            }
        }
    }

    private fun discoverServer() {
        lifecycleScope.launch {
            if (!NetworkScanner.hasValidLan(this@MainActivity)) {
                statusColorState.value = Color.Gray
                discoveryStatusState.value = "No LAN"
                return@launch
            }
            
            val port = serverPortState.value.toIntOrNull() ?: 26260
            statusColorState.value = Color.Yellow

            // Check cached
            val networkId = NetworkScanner.getNetworkId(this@MainActivity)
            if (networkId != null) {
                val cachedIp = settings.getLastServerIp(networkId)
                if (cachedIp != null && verifyAndConnect(cachedIp, port, true)) {
                    return@launch
                }
            }
            
            // Scan
            var foundIp: String? = null
            try {
                foundIp = NetworkScanner.findServer(
                    context = this@MainActivity,
                    port = port,
                    pairingCode = settings.pairingCode // unused in scanning now
                ) { status ->
                    runOnUiThread { discoveryStatusState.value = status }
                }
            } catch (e: Exception) {
                statusColorState.value = Color(0xFFF44336)
                discoveryStatusState.value = "Discovery Error: ${e.message}"
                return@launch
            }
            
            if (foundIp != null) {
                discoveryStatusState.value = "Found: $foundIp"
                verifyAndConnect(foundIp, port, false)
            } else {
                statusColorState.value = Color(0xFFF44336)
                discoveryStatusState.value = "Server not found"
            }
        }
    }

    private fun verifyManualIp() {
        val ip = serverIpState.value.trim()
        if (ip.isEmpty()) return
        val port = serverPortState.value.toIntOrNull() ?: 26260
        
        lifecycleScope.launch {
            statusColorState.value = Color.Yellow
            verifyAndConnect(ip, port, false)
        }
    }

    // Returns true if connected or prompt shown, false if failed
    private suspend fun verifyAndConnect(ip: String, port: Int, silent: Boolean): Boolean {
        // Pass pairing code to check role
        val result = ApiClient.ping(ip, port, settings.pairingCode)
        if (result.success && result.certHash != null) {
            
            // Apply Role Settings
            runOnUiThread {
                if (result.role == "guest") {
                    isGuestModeState.value = true
                    clipboardChannelState.value = "guest"
                } else {
                     isGuestModeState.value = false
                     // Keep existing channel selection or default to private
                }
            }

            val known = settings.getKnownServers()[result.certHash]
            if (known != null) {
                // Trusted Server
                // Update IP if changed (roaming)
                if (known.ip != ip) {
                    settings.saveServer(result.certHash, ip, port, known.authCode)
                }
                
                runOnUiThread {
                    serverIpState.value = ip
                    settings.serverIp = ip
                    statusColorState.value = Color(0xFF4CAF50)
                    discoveryStatusState.value = ""
                    connectWebSocket()
                    refreshFileList()
                    startPolling()
                }
                
                // Cache for auto-connect
                NetworkScanner.getNetworkId(this@MainActivity)?.let {
                    settings.setLastServerIp(it, ip)
                }
                return true
            } else {
                // New Server -> Prompt Trust
                if (!silent) {
                    runOnUiThread {
                        pendingTrustIp.value = ip
                        pendingTrustHash.value = result.certHash
                        pendingTrustPort = port
                        showTrustDialog.value = true
                    }
                    return true
                }
            }
        } else {
             if (!silent) {
                 statusColorState.value = Color(0xFFF44336)
                 discoveryStatusState.value = "Connection failed"
             }
        }
        return false
    }

    private fun connectToPendingServer() {
        val ip = pendingTrustIp.value
        val port = pendingTrustPort
        
        // After trusting, we just connect. 
        // Note: The code in settings.pairingCode might be wrong for this server if it uses a different code.
        // But for now we assume global pairing code or the user will update it in Settings.
        // Ideally TrustDialog should ask for Code too?
        // Plan says: "If Yes: Prompt for Code (or try existing code?)".
        // Let's stick to existing code for simplicity or open Settings?
        // Current implementation: uses global settings.pairingCode.
        
        serverIpState.value = ip
        settings.serverIp = ip
        statusColorState.value = Color(0xFF4CAF50)
        discoveryStatusState.value = ""
        connectWebSocket()
        refreshFileList()
        startPolling()
        
        // Cache
        NetworkScanner.getNetworkId(this@MainActivity)?.let {
            settings.setLastServerIp(it, ip)
        }
    }

    private fun fetchClipboard() {
        val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
        if (ip.isEmpty()) return
        lifecycleScope.launch {
            val text = ApiClient.getClipboard(ip, port, settings.pairingCode, clipboardChannelState.value)
            if (text != null) {
                if (clipboardTextState.value != text) clipboardTextState.value = text
                lastClipboardSync = text
            }
        }
    }

    private fun fetchClipboardSilently(ip: String, port: Int) {
        lifecycleScope.launch {
            val text = ApiClient.getClipboard(ip, port, settings.pairingCode, clipboardChannelState.value)
            if (text != null && text != lastClipboardSync) {
                clipboardTextState.value = text
                lastClipboardSync = text
            }
        }
    }

    private fun pushClipboard() {
        val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
        if (ip.isEmpty()) return
        val text = clipboardTextState.value
        lifecycleScope.launch {
            if (ApiClient.postClipboard(ip, port, text, settings.pairingCode, false, clipboardChannelState.value)) {
                lastClipboardSync = text
                Toast.makeText(this@MainActivity, "Pushed", Toast.LENGTH_SHORT).show()
            }
        }
    }

    private fun copyToPhoneClipboard() {
        val text = clipboardTextState.value
        if (text.isEmpty()) {
            Toast.makeText(this, "Nothing to copy", Toast.LENGTH_SHORT).show()
            return
        }
        val clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        val clip = android.content.ClipData.newPlainText("K-Share", text)
        clipboardManager.setPrimaryClip(clip)
        Toast.makeText(this, "Copied to clipboard", Toast.LENGTH_SHORT).show()
    }

    private fun loadHistory() {
        val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
        if (ip.isEmpty()) return
        lifecycleScope.launch {
            val history = ApiClient.getHistory(ip, port, settings.pairingCode)
            if (history.isEmpty()) Toast.makeText(this@MainActivity, "No history", Toast.LENGTH_SHORT).show()
            else {
                historyList.value = history
                showHistoryDialog.value = true
            }
        }
    }

    private fun refreshFileList() {
        val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
        if (ip.isEmpty()) return
        isRefreshingState.value = true
        lifecycleScope.launch {
            fileListState.value = ApiClient.listFiles(ip, port, settings.pairingCode)
            isRefreshingState.value = false
        }
    }

    private fun downloadFile(file: RemoteFile) {
        val intent = Intent(this, FileTransferService::class.java).apply {
            action = FileTransferService.ACTION_DOWNLOAD
            putExtra(FileTransferService.EXTRA_FILE_NAME, file.name)
            putExtra(FileTransferService.EXTRA_IS_DIR, file.isDir)
            putExtra(FileTransferService.EXTRA_SERVER_IP, serverIpState.value)
            putExtra(FileTransferService.EXTRA_SERVER_PORT, serverPortState.value.toIntOrNull() ?: 26260)
            putExtra(FileTransferService.EXTRA_PAIRING_CODE, settings.pairingCode)
        }
        startForegroundService(intent)
    }

    private fun uploadFolder(treeUri: Uri) {
        val intent = Intent(this, FileTransferService::class.java).apply {
            action = FileTransferService.ACTION_UPLOAD_FOLDER
            putExtra(FileTransferService.EXTRA_TREE_URI, treeUri.toString())
            putExtra(FileTransferService.EXTRA_SERVER_IP, serverIpState.value)
            putExtra(FileTransferService.EXTRA_SERVER_PORT, serverPortState.value.toIntOrNull() ?: 26260)
            putExtra(FileTransferService.EXTRA_PAIRING_CODE, settings.pairingCode)
        }
        startForegroundService(intent)
    }
}
