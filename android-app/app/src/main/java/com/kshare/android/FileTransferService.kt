package com.kshare.android

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
import android.os.PowerManager
import android.provider.OpenableColumns
import androidx.core.content.IntentCompat
import android.webkit.MimeTypeMap
import androidx.core.app.NotificationCompat
import androidx.core.content.FileProvider
import androidx.documentfile.provider.DocumentFile
import com.kshare.android.api.ApiClient
import kotlinx.coroutines.*
import java.io.*
import java.util.*
import java.util.concurrent.atomic.AtomicInteger
import java.util.zip.ZipEntry
import java.util.zip.ZipInputStream

class FileTransferService : Service() {

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var wifiLock: WifiManager.WifiLock? = null
    private var wakeLock: PowerManager.WakeLock? = null

    companion object {
        const val ACTION_UPLOAD = "com.kshare.android.action.UPLOAD"
        const val ACTION_UPLOAD_FOLDER = "com.kshare.android.action.UPLOAD_FOLDER"
        const val ACTION_DOWNLOAD = "com.kshare.android.action.DOWNLOAD"
        const val EXTRA_URI = "uri"
        const val EXTRA_URIS = "uris"
        const val EXTRA_TREE_URI = "treeUri"
        const val EXTRA_SERVER_IP = "serverIp"
        const val EXTRA_SERVER_PORT = "serverPort"
        const val EXTRA_FILE_NAME = "fileName"
        const val EXTRA_IS_DIR = "isDir"
        const val EXTRA_PAIRING_CODE = "pairingCode"
        private val nextNotifId = AtomicInteger(100)
    }

    private val activeTasks = AtomicInteger(0)

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        acquireWifiLock()
        acquireWakeLock()
    }

    private fun startForegroundCompat(id: Int, notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(id, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(id, notification)
        }
    }

    private fun stopForegroundCompat() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            stopForeground(STOP_FOREGROUND_DETACH)
        } else {
            @Suppress("DEPRECATION")
            stopForeground(false)
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
            ACTION_UPLOAD_FOLDER -> {
                val treeUriString = intent.getStringExtra(EXTRA_TREE_URI)
                if (treeUriString != null) {
                    handleFolderUpload(treeUriString, serverIp, serverPort, pairingCode, startId)
                } else {
                    stopSelf(startId)
                }
            }
            ACTION_DOWNLOAD -> {
                val fileName = intent.getStringExtra(EXTRA_FILE_NAME) ?: run {
                    stopSelf(startId)
                    return START_NOT_STICKY
                }
                val isDir = intent.getBooleanExtra(EXTRA_IS_DIR, false)
                handleDownload(fileName, isDir, serverIp, serverPort, pairingCode, startId)
            }
            else -> stopSelf(startId)
        }
        return START_STICKY
    }

    private fun handleUpload(uriString: String, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        val notifId = nextNotifId.getAndIncrement()
        startForegroundCompat(notifId, createNotification("Preparing upload...", -1))
        scope.launch {
            try {
                if (!isActive) return@launch
                val uri = Uri.parse(uriString)
                val fileName = getFileName(uri) ?: "unknown"
                val length = getFileLength(uri)
                val inputStream = contentResolver.openInputStream(uri) ?: throw Exception("Failed to open stream")

                var lastUpdateTime = System.currentTimeMillis()
                val success = ApiClient.uploadFile(serverIp, serverPort, inputStream, fileName, pairingCode, length) { sent, total ->
                    if (!isActive) return@uploadFile false
                    val currentTime = System.currentTimeMillis()
                    val progress = if (total > 0) (sent * 100 / total).toInt() else -1
                    if (currentTime - lastUpdateTime > 1000 || sent == total) {
                        updateNotification(notifId, "Uploading $fileName", progress)
                        lastUpdateTime = currentTime
                    }
                    true
                }

                withContext(Dispatchers.Main) {
                    showCompletionNotification(notifId, if (success) "Upload complete: $fileName" else "Upload failed: $fileName")
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(notifId, "Upload error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForegroundCompat()
                stopSelf(startId)
            }
        }
    }

    private fun handleMultipleUploads(uris: List<Uri>, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        val notifId = nextNotifId.getAndIncrement()
        startForegroundCompat(notifId, createNotification("Preparing ${uris.size} files...", -1))
        scope.launch {
            var successCount = 0
            uris.forEachIndexed { index, uri ->
                if (!isActive) return@forEachIndexed
                try {
                    val fileName = getFileName(uri) ?: "file_$index"
                    val length = getFileLength(uri)
                    val inputStream = contentResolver.openInputStream(uri) ?: return@forEachIndexed

                    var lastUpdateTime = System.currentTimeMillis()
                    val success = ApiClient.uploadFile(serverIp, serverPort, inputStream, fileName, pairingCode, length) { sent, total ->
                        if (!isActive) return@uploadFile false
                        val currentTime = System.currentTimeMillis()
                        if (currentTime - lastUpdateTime > 1000 || sent == total) {
                            val progress = if (total > 0) (sent * 100 / total).toInt() else -1
                            updateNotification(notifId, "Uploading (${index + 1}/${uris.size}): $fileName", progress)
                            lastUpdateTime = currentTime
                        }
                        true
                    }
                    if (success) successCount++
                } catch (e: Exception) { e.printStackTrace() }
            }
            withContext(Dispatchers.Main) { showCompletionNotification(notifId, "Uploaded $successCount/${uris.size} files") }
            if (activeTasks.decrementAndGet() == 0) stopForegroundCompat()
            stopSelf(startId)
        }
    }

    private fun handleFolderUpload(treeUriString: String, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        val notifId = nextNotifId.getAndIncrement()
        startForegroundCompat(notifId, createNotification("Scanning folder...", -1))
        scope.launch {
            try {
                val treeUri = Uri.parse(treeUriString)
                val rootDoc = DocumentFile.fromTreeUri(this@FileTransferService, treeUri) ?: throw Exception("Failed to open folder")

                var totalFiles = 0
                var successCount = 0
                var currentFileIndex = 0

                // Phase 1: Count files (optional, but good for progress)
                var lastCountTime = System.currentTimeMillis()
                suspend fun count(doc: DocumentFile) {
                    if (!isActive) return
                    if (doc.isDirectory) {
                        for (file in doc.listFiles()) {
                            count(file)
                        }
                    } else {
                        totalFiles++
                        val now = System.currentTimeMillis()
                        if (now - lastCountTime > 1000) {
                             updateNotification(notifId, "Counting files: $totalFiles", -1)
                             lastCountTime = now
                        }
                    }
                }

                updateNotification(notifId, "Counting files...", -1)
                count(rootDoc)

                if (totalFiles == 0) {
                    withContext(Dispatchers.Main) { showCompletionNotification(notifId, "Folder is empty") }
                    return@launch
                }

                // Phase 2: Immediate Upload
                suspend fun scanAndUpload(doc: DocumentFile, path: String) {
                    if (!isActive) return
                    val name = doc.name ?: return
                    if (doc.isDirectory) {
                        for (file in doc.listFiles()) {
                            scanAndUpload(file, "$path$name/")
                        }
                    } else {
                        currentFileIndex++
                        var lastUpdateTime = System.currentTimeMillis()
                        try {
                            val inputStream = contentResolver.openInputStream(doc.uri) ?: return
                            val success = ApiClient.uploadFile(serverIp, serverPort, inputStream, "$path$name", pairingCode, doc.length()) { sent, total ->
                                if (!isActive) return@uploadFile false
                                val currentTime = System.currentTimeMillis()
                                if (currentTime - lastUpdateTime > 1000 || sent == total) {
                                    val progress = if (total > 0) (sent * 100 / total).toInt() else -1
                                    updateNotification(notifId, "Uploading ($currentFileIndex/$totalFiles): $name", progress)
                                    lastUpdateTime = currentTime
                                }
                                true
                            }
                            if (success) successCount++
                        } catch (e: Exception) { e.printStackTrace() }
                    }
                }

                val rootName = rootDoc.name ?: "Folder"
                // Start scan from children of rootDoc to match previous logic
                for (file in rootDoc.listFiles()) {
                    scanAndUpload(file, "$rootName/")
                }

                withContext(Dispatchers.Main) {
                    showCompletionNotification(notifId, "Uploaded $successCount/$totalFiles files from $rootName")
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(notifId, "Folder upload error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForegroundCompat()
                stopSelf(startId)
            }
        }
    }

    private fun handleDownload(fileName: String, isDir: Boolean, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        val notifId = nextNotifId.getAndIncrement()
        val displayTitle = if (isDir) "Downloading folder $fileName..." else "Downloading $fileName..."
        startForegroundCompat(notifId, createNotification(displayTitle, -1))
        scope.launch {
            try {
                val settings = SettingsManager(this@FileTransferService)
                val customUri = settings.downloadUri
                var startTime = System.currentTimeMillis()

                ApiClient.downloadFile(serverIp, serverPort, fileName, pairingCode, 0) { encryptedStream, total ->
                    if (!isActive) return@downloadFile
                    if (isDir) {
                        val tempZip = File(cacheDir, "download_${System.currentTimeMillis()}.zip")
                        FileOutputStream(tempZip).use { output ->
                            val input = encryptedStream
                            val buffer = ByteArray(64 * 1024)
                            var totalRead = 0L
                            var read = 0
                            while (isActive && input.read(buffer).also { read = it } != -1) {
                                output.write(buffer, 0, read)
                                totalRead += read
                                val currentTime = System.currentTimeMillis()
                                if (currentTime - startTime > 1000) {
                                    val progress = if (total > 0) (totalRead * 100 / total).toInt() else -1
                                    updateNotification(notifId, "Downloading folder $fileName", progress)
                                    startTime = currentTime
                                }
                            }
                        }

                        if (customUri.isNotEmpty()) {
                            val rootDoc = DocumentFile.fromTreeUri(this@FileTransferService, Uri.parse(customUri))
                            if (rootDoc != null && rootDoc.canWrite()) {
                                var count = 0
                                while (rootDoc.findFile(if (count == 0) fileName else "$fileName ($count)") != null) {
                                    count++
                                }
                                val finalName = if (count == 0) fileName else "$fileName ($count)"
                                val finalTarget = rootDoc.createDirectory(finalName) ?: throw Exception("Failed to create folder")
                                unzipToDoc(tempZip, finalTarget)
                                withContext(Dispatchers.Main) { showDownloadCompleteNotification(notifId, finalTarget.name ?: fileName, finalTarget.uri.toString()) }
                            } else throw Exception("Custom folder inaccessible")
                        } else {
                            val downloadsDir = getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS) ?: Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                            var targetDir = File(downloadsDir, fileName)
                            var count = 0
                            while (targetDir.exists()) {
                                count++
                                targetDir = File(downloadsDir, "$fileName ($count)")
                            }
                            targetDir.mkdirs()
                            unzip(tempZip, targetDir)
                            withContext(Dispatchers.Main) { showDownloadCompleteNotification(notifId, targetDir.name) }
                        }
                        tempZip.delete()
                    } else {
                        if (customUri.isNotEmpty()) {
                            val rootDoc = DocumentFile.fromTreeUri(this@FileTransferService, Uri.parse(customUri))
                            if (rootDoc != null && rootDoc.canWrite()) {
                                var count = 0
                                var finalName = fileName
                                while (rootDoc.findFile(finalName) != null) {
                                    count++
                                    val nameWithoutExt = File(fileName).nameWithoutExtension
                                    val ext = File(fileName).extension
                                    finalName = if (ext.isNotEmpty()) "$nameWithoutExt ($count).$ext" else "$fileName ($count)"
                                }
                                val outputFile = rootDoc.createFile("application/octet-stream", finalName)
                                    ?: throw Exception("Failed to create file")
                                val pfd = contentResolver.openFileDescriptor(outputFile.uri, "w")
                                    ?: throw Exception("Failed to open file descriptor")
                                FileOutputStream(pfd.fileDescriptor).use { output ->
                                    val input = encryptedStream
                                    val buffer = ByteArray(64 * 1024)
                                    var totalRead = 0L
                                    var read = 0
                                    while (isActive && input.read(buffer).also { read = it } != -1) {
                                        output.write(buffer, 0, read)
                                        totalRead += read
                                        val currentTime = System.currentTimeMillis()
                                        if (currentTime - startTime > 1000) {
                                            val progress = if (total > 0) (totalRead * 100 / total).toInt() else -1
                                            updateNotification(notifId, "Downloading $fileName", progress)
                                            startTime = currentTime
                                        }
                                    }
                                }
                                pfd.close()
                                withContext(Dispatchers.Main) { showDownloadCompleteNotification(notifId, finalName, outputFile.uri.toString()) }
                            } else throw Exception("Custom folder inaccessible")
                        } else {
                            val downloadsDir = getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS) ?: Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                            var outputFile = File(downloadsDir, fileName)
                            var count = 1
                            val nameWithoutExt = outputFile.nameWithoutExtension
                            val ext = outputFile.extension

                            while (outputFile.exists()) {
                                val newName = if (ext.isNotEmpty()) "$nameWithoutExt ($count).$ext" else "$nameWithoutExt ($count)"
                                outputFile = File(downloadsDir, newName)
                                count++
                            }

                            val tempFile = File(cacheDir, "resumable_${fileName.hashCode()}.tmp")
                            val startOffset = if (tempFile.exists()) tempFile.length() else 0L

                            FileOutputStream(tempFile, startOffset > 0).use { output ->
                                val input = encryptedStream
                                val buffer = ByteArray(64 * 1024)
                                var totalRead = startOffset
                                val trueTotal = total + startOffset
                                var read = 0
                                while (isActive && input.read(buffer).also { read = it } != -1) {
                                    output.write(buffer, 0, read)
                                    totalRead += read
                                    val currentTime = System.currentTimeMillis()
                                    if (currentTime - startTime > 1000) {
                                        val progress = if (trueTotal > 0) (totalRead * 100 / trueTotal).toInt() else -1
                                        updateNotification(notifId, "Downloading $fileName", progress)
                                        startTime = currentTime
                                    }
                                }
                            }
                            tempFile.renameTo(outputFile)
                            MediaScannerConnection.scanFile(this@FileTransferService, arrayOf(outputFile.absolutePath), null, null)
                            withContext(Dispatchers.Main) { showDownloadCompleteNotification(notifId, outputFile.name) }
                        }
                    }
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(notifId, "Download error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForegroundCompat()
                stopSelf(startId)
            }
        }
    }

    private fun unzipToDoc(zipFile: File, targetRoot: DocumentFile) {
        ZipInputStream(FileInputStream(zipFile)).use { zis ->
            var entry: ZipEntry? = zis.nextEntry
            while (entry != null) {
                val pathParts = entry.name.replace("\\", "/").split("/").filter { it.isNotEmpty() && it != ".." && it != "." }
                if (pathParts.isEmpty()) {
                    zis.closeEntry()
                    entry = zis.nextEntry
                    continue
                }

                var currentDir = targetRoot
                for (i in 0 until pathParts.size - 1) {
                    val part = pathParts[i]
                    currentDir = currentDir.findFile(part) ?: currentDir.createDirectory(part) ?: currentDir
                }

                if (!entry.isDirectory) {
                    val fileName = pathParts.last()
                    val ext = fileName.substringAfterLast(".", "")
                    val mimeType = MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext.lowercase()) ?: "application/octet-stream"

                    val newFile = currentDir.findFile(fileName) ?: currentDir.createFile(mimeType, fileName)
                    if (newFile != null) {
                        contentResolver.openOutputStream(newFile.uri).use { fos ->
                            if (fos != null) zis.copyTo(fos)
                        }
                    }
                }
                zis.closeEntry()
                entry = zis.nextEntry
            }
        }
    }

    private fun unzip(zipFile: File, targetDir: File) {
        ZipInputStream(FileInputStream(zipFile)).use { zis ->
            var entry: ZipEntry? = zis.nextEntry
            while (entry != null) {
                val entryName = entry.name.replace("\\", "/")
                val newFile = File(targetDir, entryName)

                if (!newFile.canonicalPath.startsWith(targetDir.canonicalPath)) {
                    zis.closeEntry()
                    entry = zis.nextEntry
                    continue
                }

                if (entry.isDirectory) {
                    newFile.mkdirs()
                } else {
                    newFile.parentFile?.mkdirs()
                    FileOutputStream(newFile).use { fos ->
                        zis.copyTo(fos)
                    }
                    MediaScannerConnection.scanFile(this, arrayOf(newFile.absolutePath), null, null)
                }
                zis.closeEntry()
                entry = zis.nextEntry
            }
        }
    }

    private fun acquireWifiLock() {
        val wifiManager = getSystemService(WIFI_SERVICE) as WifiManager
        wifiLock = wifiManager.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "Kshare:Transfer").apply { acquire() }
    }

    private fun acquireWakeLock() {
        val powerManager = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "KShare:TransferWakeLock").apply {
            setReferenceCounted(false)
            acquire(60 * 60 * 1000L) // 1 hour max
        }
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

    private fun createNotification(text: String, progress: Int? = null, maxProgress: Int = 100): Notification {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
        }
        val pendingIntent = PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE)

        val builder = NotificationCompat.Builder(this, "transfer")
            .setContentTitle("K-Share")
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_notification)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setContentIntent(pendingIntent)

        if (progress != null) {
            builder.setProgress(maxProgress, progress, progress < 0)
        }

        return builder.build()
    }

    private fun updateNotification(id: Int, text: String, progress: Int? = null, maxProgress: Int = 100) {
        val notification = createNotification(text, progress, maxProgress)
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

    private fun showDownloadCompleteNotification(id: Int, fileName: String, customUri: String = "") {
        val intent = if (customUri.isNotEmpty()) {
            Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(Uri.parse(customUri), "vnd.android.document/directory")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
        } else {
            Intent(DownloadManager.ACTION_VIEW_DOWNLOADS)
        }
        intent.flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK

        val pendingIntent = PendingIntent.getActivity(this, id, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)

        val notification = NotificationCompat.Builder(this, "transfer")
            .setContentTitle("Download Complete")
            .setContentText(fileName)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .addAction(android.R.drawable.ic_menu_view, if (customUri.isNotEmpty()) "Open Folder" else "View Downloads", pendingIntent)
            .build()
        getSystemService(NotificationManager::class.java).notify(id, notification)
    }

    override fun onDestroy() {
        super.onDestroy()
        wifiLock?.release()
        wakeLock?.release()
        scope.cancel()
    }
}
