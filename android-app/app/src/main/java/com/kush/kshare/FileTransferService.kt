package com.kush.kshare

import android.app.*
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.media.MediaScannerConnection
import android.net.Uri
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Environment
import android.os.IBinder
import android.provider.OpenableColumns
import androidx.core.content.IntentCompat
import androidx.core.app.NotificationCompat
import androidx.core.content.FileProvider
import com.kush.kshare.api.ApiClient
import kotlinx.coroutines.*
import java.io.File
import java.io.FileOutputStream
import java.io.InputStream
import java.util.*
import java.util.concurrent.atomic.AtomicInteger

class FileTransferService : Service() {
    
    private val scope = CoroutineScope(Dispatchers.IO + Job())
    private var wifiLock: WifiManager.WifiLock? = null
    
    companion object {
        const val ACTION_UPLOAD = "com.kush.kshare.action.UPLOAD"
        const val ACTION_DOWNLOAD = "com.kush.kshare.action.DOWNLOAD"
        const val EXTRA_URI = "uri"
        const val EXTRA_URIS = "uris"
        const val EXTRA_SERVER_IP = "serverIp"
        const val EXTRA_SERVER_PORT = "serverPort"
        const val EXTRA_FILE_NAME = "fileName"
        const val EXTRA_PAIRING_CODE = "pairingCode"
    }

    private val activeTasks = AtomicInteger(0)
    
    override fun onBind(intent: Intent?): IBinder? = null
    
    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        acquireWifiLock()
    }

    private fun startForegroundCompat(id: Int, notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(id, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(id, notification)
        }
    }
    
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val serverIp = intent?.getStringExtra(EXTRA_SERVER_IP) ?: run {
            stopSelf(startId)
            return START_NOT_STICKY
        }
        val serverPort = intent.getIntExtra(EXTRA_SERVER_PORT, 26260)
        val pairingCode = intent.getStringExtra(EXTRA_PAIRING_CODE) ?: ""
        
        when (intent.action) {
            ACTION_UPLOAD -> {
                val uriString = intent.getStringExtra(EXTRA_URI)
                val uris = IntentCompat.getParcelableArrayListExtra(intent, EXTRA_URIS, Uri::class.java)
                if (uriString != null) {
                    handleUpload(uriString, serverIp, serverPort, pairingCode, startId)
                } else if (uris != null) {
                    handleMultipleUploads(uris, serverIp, serverPort, pairingCode, startId)
                } else {
                    stopSelf(startId)
                }
            }
            ACTION_DOWNLOAD -> {
                val fileName = intent.getStringExtra(EXTRA_FILE_NAME) ?: run {
                    stopSelf(startId)
                    return START_NOT_STICKY
                }
                handleDownload(fileName, serverIp, serverPort, pairingCode, startId)
            }
            else -> stopSelf(startId)
        }
        return START_STICKY
    }

    private fun handleUpload(uriString: String, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        startForegroundCompat(1, createNotification("Uploading..."))
        scope.launch {
            try {
                val uri = Uri.parse(uriString)
                val fileName = getFileName(uri) ?: "unknown"
                val length = getFileLength(uri)
                val inputStream = contentResolver.openInputStream(uri) ?: throw Exception("Failed to open stream")
                
                val success = ApiClient.uploadFile(serverIp, serverPort, inputStream, fileName, pairingCode, length) { sent, total ->
                    val progress = if (total > 0) (sent * 100 / total).toInt() else 0
                    updateNotification(1, "Uploading: $progress%")
                }
                
                withContext(Dispatchers.Main) {
                    showCompletionNotification(System.currentTimeMillis().toInt(), if (success) "Upload complete: $fileName" else "Upload failed: $fileName")
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(System.currentTimeMillis().toInt(), "Upload error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForeground(STOP_FOREGROUND_DETACH)
                stopSelf(startId)
            }
        }
    }

    private fun handleMultipleUploads(uris: List<Uri>, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        startForegroundCompat(1, createNotification("Preparing ${uris.size} files..."))
        scope.launch {
            var successCount = 0
            uris.forEachIndexed { index, uri ->
                try {
                    val fileName = getFileName(uri) ?: "file_$index"
                    val length = getFileLength(uri)
                    val inputStream = contentResolver.openInputStream(uri) ?: return@forEachIndexed
                    
                    updateNotification(1, "Uploading (${index + 1}/${uris.size}): $fileName")
                    
                    val success = ApiClient.uploadFile(serverIp, serverPort, inputStream, fileName, pairingCode, length) { _, _ -> }
                    if (success) successCount++
                } catch (e: Exception) { e.printStackTrace() }
            }
            withContext(Dispatchers.Main) { showCompletionNotification(System.currentTimeMillis().toInt(), "Uploaded $successCount/${uris.size} files") }
            if (activeTasks.decrementAndGet() == 0) stopForeground(STOP_FOREGROUND_DETACH)
            stopSelf(startId)
        }
    }

    private fun handleDownload(fileName: String, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        startForegroundCompat(3, createNotification("Downloading $fileName..."))
        scope.launch {
            try {
                var startTime = System.currentTimeMillis()
                ApiClient.downloadFile(serverIp, serverPort, fileName, pairingCode) { encryptedStream, total ->
                    val outputFile = File(Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS), fileName)
                    FileOutputStream(outputFile).use { output ->
                        ApiClient.decryptStream(encryptedStream, output, pairingCode) { downloaded ->
                            val currentTime = System.currentTimeMillis()
                            if (currentTime - startTime > 1000) {
                                val progress = if (total > 0) (downloaded * 100 / total).toInt() else -1
                                updateNotification(3, "Downloading: ${if (progress >= 0) "$progress%" else ""}")
                                startTime = currentTime
                            }
                        }
                    }
                    MediaScannerConnection.scanFile(this@FileTransferService, arrayOf(outputFile.absolutePath), null, null)
                    withContext(Dispatchers.Main) { showDownloadCompleteNotification(System.currentTimeMillis().toInt(), fileName, outputFile) }
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(System.currentTimeMillis().toInt(), "Download error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForeground(STOP_FOREGROUND_DETACH)
                stopSelf(startId)
            }
        }
    }
    
    private fun acquireWifiLock() {
        val wifiManager = getSystemService(WIFI_SERVICE) as WifiManager
        wifiLock = wifiManager.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "Kshare:Transfer").apply { acquire() }
    }

    private fun getFileName(uri: Uri): String? {
        var name: String? = null
        if (uri.scheme == "content") {
            contentResolver.query(uri, null, null, null, null)?.use {
                if (it.moveToFirst()) {
                    val index = it.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (index != -1) name = it.getString(index)
                }
            }
        }
        return name ?: uri.path?.substringAfterLast('/')
    }

    private fun getFileLength(uri: Uri): Long {
        return try { contentResolver.openAssetFileDescriptor(uri, "r")?.use { it.length } ?: -1L } catch (e: Exception) { -1L }
    }
    
    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel("transfer", "File Transfer", NotificationManager.IMPORTANCE_LOW)
            getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
        }
    }
    
    private fun createNotification(text: String): Notification {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
        }
        val pendingIntent = PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE)

        return NotificationCompat.Builder(this, "transfer")
            .setContentTitle("K-Share")
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_notification)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setContentIntent(pendingIntent)
            .build()
    }
    
    private fun updateNotification(id: Int, text: String) {
        val notification = createNotification(text)
        getSystemService(NotificationManager::class.java).notify(id, notification)
    }
    
    private fun showCompletionNotification(id: Int, text: String) {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
        }
        val pendingIntent = PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE)

        val notification = NotificationCompat.Builder(this, "transfer")
            .setContentTitle("K-Share")
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_notification)
            .setAutoCancel(true)
            .setContentIntent(pendingIntent)
            .build()
        getSystemService(NotificationManager::class.java).notify(id, notification)
    }

    private fun showDownloadCompleteNotification(id: Int, fileName: String, file: File) {
        val intent = Intent(Intent.ACTION_VIEW)
        val uri = FileProvider.getUriForFile(this, "$packageName.provider", file)
        intent.setDataAndType(uri, contentResolver.getType(uri) ?: "*/*")
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        val pendingIntent = PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE)
        val notification = NotificationCompat.Builder(this, "transfer")
            .setContentTitle("Download Complete")
            .setContentText(fileName)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .addAction(android.R.drawable.ic_menu_view, "Open", pendingIntent)
            .build()
        getSystemService(NotificationManager::class.java).notify(id, notification)
    }
    
    override fun onDestroy() {
        super.onDestroy()
        wifiLock?.release()
        scope.cancel()
    }
}
