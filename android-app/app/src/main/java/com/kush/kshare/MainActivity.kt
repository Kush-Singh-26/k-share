package com.kush.kshare

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
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
    private val client = OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).build()
    private var webSocket: WebSocket? = null
    private var pollingJob: Job? = null
    
    // State
    private var serverIpState = mutableStateOf("")
    private var serverPortState = mutableStateOf("26260")
    private var statusColorState = mutableStateOf(Color.Yellow)
    private var clipboardTextState = mutableStateOf("")
    private var fileListState = mutableStateOf<List<RemoteFile>>(emptyList())
    private var isRefreshingState = mutableStateOf(false)
    private var showHistoryDialog = mutableStateOf(false)
    private var historyList = mutableStateOf<List<HistoryItem>>(emptyList())

    private var lastClipboardSync = ""
    private var lastUserEditTime = 0L

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settings = SettingsManager(this)
        ThumbnailCache.init(this)

        serverIpState.value = settings.serverIp
        serverPortState.value = settings.serverPort.ifEmpty { "26260" }

        setContent {
            MaterialTheme {
                Surface(color = Color(0xFFF8F9FA), modifier = Modifier.fillMaxSize()) {
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
                                if (ApiClient.deleteHistory(ip, port, item.id)) {
                                    historyList.value = historyList.value.filter { it.id != item.id }
                                }
                            }
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
        discoverServer()
        scheduleBackgroundSync()
    }

    override fun onResume() {
        super.onResume()
        serverIpState.value = settings.serverIp
        serverPortState.value = settings.serverPort.ifEmpty { "26260" }
        connectWebSocket()
        startPolling()
    }

    override fun onPause() {
        super.onPause()
        closeWebSocket()
        stopPolling()
    }

    @Composable
    fun MainScreen() {
        Column(modifier = Modifier.padding(16.dp)) {
            // Header
            Text("K-Share", fontSize = 32.sp, fontWeight = FontWeight.Bold, color = Color(0xFF3F51B5))
            Text("Encrypted Local Sharing", fontSize = 12.sp, color = Color.Gray, modifier = Modifier.padding(bottom = 8.dp))

            // Settings Card
            Card(
                colors = CardDefaults.cardColors(containerColor = Color.White),
                elevation = CardDefaults.cardElevation(2.dp),
                modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        OutlinedTextField(
                            value = serverIpState.value,
                            onValueChange = { 
                                serverIpState.value = it
                                settings.serverIp = it
                                connectWebSocket()
                            },
                            label = { Text("Server IP") },
                            modifier = Modifier.weight(1f).padding(end = 8.dp),
                            singleLine = true
                        )
                        OutlinedTextField(
                            value = serverPortState.value,
                            onValueChange = { 
                                serverPortState.value = it
                                settings.serverPort = it
                                connectWebSocket()
                            },
                            label = { Text("Port") },
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                            modifier = Modifier.width(100.dp),
                            singleLine = true
                        )
                    }
                    Row(
                        modifier = Modifier.padding(top = 8.dp).fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.End
                    ) {
                        Box(
                            modifier = Modifier
                                .size(12.dp)
                                .clip(CircleShape)
                                .background(statusColorState.value)
                        )
                        Spacer(Modifier.width(16.dp))
                        TextButton(onClick = { discoverServer() }) { Text("Re-Discover") }
                        TextButton(onClick = { startActivity(Intent(this@MainActivity, SettingsActivity::class.java)) }) { Text("Config") }
                    }
                }
            }

            // Clipboard Card
            Card(
                colors = CardDefaults.cardColors(containerColor = Color.White),
                elevation = CardDefaults.cardElevation(2.dp),
                modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text("SHARED CLIPBOARD", fontSize = 11.sp, color = Color.Gray, letterSpacing = 1.sp)
                    
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(220.dp)
                            .padding(vertical = 8.dp)
                            .border(1.dp, Color.LightGray, RoundedCornerShape(4.dp))
                            .padding(8.dp)
                    ) {
                        AndroidView(
                            factory = { ctx ->
                                EditText(ctx).apply {
                                    background = null
                                    textSize = 16f
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
                            },
                            modifier = Modifier.fillMaxSize()
                        )
                    }

                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Button(onClick = { pushClipboard() }) { Text("Push") }
                        TextButton(onClick = { fetchClipboard() }) { Text("Fetch") }
                        TextButton(onClick = { loadHistory() }) { Text("History") }
                    }
                }
            }

            // File List Card
            Card(
                colors = CardDefaults.cardColors(containerColor = Color.White),
                elevation = CardDefaults.cardElevation(2.dp),
                modifier = Modifier.weight(1f).fillMaxWidth().padding(vertical = 8.dp)
            ) {
                Column {
                    Row(
                        modifier = Modifier.padding(16.dp).fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Files", fontWeight = FontWeight.Bold)
                        IconButton(onClick = { refreshFileList() }) {
                            Icon(Icons.Default.Refresh, "Refresh")
                        }
                    }
                    if (isRefreshingState.value) {
                        LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                    }
                    LazyColumn {
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
            if (file.name.lowercase().let { it.endsWith(".jpg") || it.endsWith(".png") }) {
                val url = ApiClient.getThumbnailUrl(serverIpState.value, serverPortState.value.toIntOrNull()?:0, "tophone", file.name)
                val key = "${serverIpState.value}_${file.name}"
                val cached = ThumbnailCache.getFromMemory(key)
                if (cached != null) thumbnail = cached
                else withContext(Dispatchers.IO) {
                    thumbnail = ThumbnailCache.get(key, url)
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
            if (thumbnail != null) {
                Image(
                    bitmap = thumbnail!!.asImageBitmap(),
                    contentDescription = null,
                    modifier = Modifier.size(48.dp).background(Color.LightGray)
                )
            } else {
                Icon(
                    painter = painterResource(android.R.drawable.ic_menu_save),
                    contentDescription = null,
                    modifier = Modifier.size(48.dp).padding(8.dp)
                )
            }
            Column(modifier = Modifier.padding(start = 12.dp).weight(1f)) {
                Text(file.name, fontWeight = FontWeight.Bold, maxLines = 1)
                Text("${file.size / 1024} KB • ${file.modTime}", fontSize = 12.sp, color = Color.Gray)
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
        Dialog(onDismissRequest = onDismiss) {
            Card(
                colors = CardDefaults.cardColors(containerColor = Color.White),
                modifier = Modifier.fillMaxWidth(0.95f).heightIn(max = 500.dp)
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text("History", style = MaterialTheme.typography.titleLarge)
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
                                            setTextColor(android.graphics.Color.BLACK)
                                            autoLinkMask = Linkify.WEB_URLS
                                            linksClickable = true
                                            movementMethod = android.text.method.LinkMovementMethod.getInstance()
                                        }
                                    },
                                    update = { view ->
                                        view.text = item.text
                                    },
                                    modifier = Modifier.weight(1f)
                                )
                                IconButton(onClick = { onDelete(item) }) {
                                    Icon(Icons.Default.Delete, "Delete")
                                }
                            }
                            HorizontalDivider()
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
        val request = Request.Builder().url("ws://$ip:$port/ws").build()
        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                statusColorState.value = Color(0xFF4CAF50)
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                val type = JSONObject(text).optString("type")
                lifecycleScope.launch(Dispatchers.Main) {
                    when (type) {
                        "clip" -> fetchClipboard()
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
                if (isOnWifi()) {
                    val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
                    if (ip.isNotEmpty()) {
                        val online = ApiClient.ping(ip, port, settings.pairingCode)
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
    
    private fun isOnWifi(): Boolean {
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val caps = cm.getNetworkCapabilities(cm.activeNetwork) ?: return false
        return caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)
    }

    private fun scheduleBackgroundSync() {
        val constraints = Constraints.Builder().setRequiredNetworkType(NetworkType.UNMETERED).build()
        val request = PeriodicWorkRequestBuilder<SyncWorker>(15, TimeUnit.MINUTES).setConstraints(constraints).build()
        WorkManager.getInstance(this).enqueueUniquePeriodicWork("DiscoverySync", ExistingPeriodicWorkPolicy.KEEP, request)
    }

    private fun discoverServer() {
        val url = settings.gistUrl; val key = settings.gistJsonKey
        if (url.isEmpty()) return
        ApiClient.fetchIpFromGist(url, key) { ip ->
            runOnUiThread {
                if (ip != null) {
                    serverIpState.value = ip
                    settings.serverIp = ip
                    refreshFileList()
                    connectWebSocket()
                } else statusColorState.value = Color.Red
            }
        }
    }

    private fun fetchClipboard() {
        val ip = serverIpState.value; val port = serverPortState.value.toIntOrNull() ?: 26260
        if (ip.isEmpty()) return
        lifecycleScope.launch {
            val text = ApiClient.getClipboard(ip, port, settings.pairingCode)
            if (text != null) {
                if (clipboardTextState.value != text) clipboardTextState.value = text
                lastClipboardSync = text
            }
        }
    }

    private fun fetchClipboardSilently(ip: String, port: Int) {
        lifecycleScope.launch {
            val text = ApiClient.getClipboard(ip, port, settings.pairingCode)
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
            if (ApiClient.postClipboard(ip, port, text, settings.pairingCode)) {
                lastClipboardSync = text
                Toast.makeText(this@MainActivity, "Pushed", Toast.LENGTH_SHORT).show()
            }
        }
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
            putExtra(FileTransferService.EXTRA_SERVER_IP, serverIpState.value)
            putExtra(FileTransferService.EXTRA_SERVER_PORT, serverPortState.value.toIntOrNull() ?: 26260)
            putExtra(FileTransferService.EXTRA_PAIRING_CODE, settings.pairingCode)
        }
        startForegroundService(intent)
    }
}
