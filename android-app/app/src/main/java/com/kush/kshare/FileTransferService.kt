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
import android.webkit.MimeTypeMap
import androidx.core.app.NotificationCompat
import androidx.core.content.FileProvider
import androidx.documentfile.provider.DocumentFile
import com.kush.kshare.api.ApiClient
import kotlinx.coroutines.*
import java.io.*
import java.util.*
import java.util.concurrent.atomic.AtomicInteger
import java.util.zip.ZipEntry
import java.util.zip.ZipInputStream

class FileTransferService : Service() {
    
    private val scope = CoroutineScope(Dispatchers.IO + Job())
    private var wifiLock: WifiManager.WifiLock? = null
    
    companion object {
        const val ACTION_UPLOAD = "com.kush.kshare.action.UPLOAD"
        const val ACTION_UPLOAD_FOLDER = "com.kush.kshare.action.UPLOAD_FOLDER"
        const val ACTION_DOWNLOAD = "com.kush.kshare.action.DOWNLOAD"
        const val EXTRA_URI = "uri"
        const val EXTRA_URIS = "uris"
        const val EXTRA_TREE_URI = "treeUri"
        const val EXTRA_SERVER_IP = "serverIp"
        const val EXTRA_SERVER_PORT = "serverPort"
        const val EXTRA_FILE_NAME = "fileName"
        const val EXTRA_IS_DIR = "isDir"
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

    private fun handleFolderUpload(treeUriString: String, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        startForegroundCompat(1, createNotification("Scanning folder..."))
        scope.launch {
            try {
                val treeUri = Uri.parse(treeUriString)
                val rootDoc = DocumentFile.fromTreeUri(this@FileTransferService, treeUri) ?: throw Exception("Failed to open folder")
                val files = mutableListOf<Pair<DocumentFile, String>>()
                
                fun scan(doc: DocumentFile, path: String) {
                    val name = doc.name ?: return
                    if (doc.isDirectory) {
                        doc.listFiles().forEach { scan(it, "$path$name/") }
                    } else {
                        files.add(doc to "$path$name")
                    }
                }
                
                val rootName = rootDoc.name ?: "Folder"
                rootDoc.listFiles().forEach { scan(it, "$rootName/") }

                if (files.isEmpty()) {
                    withContext(Dispatchers.Main) { showCompletionNotification(System.currentTimeMillis().toInt(), "Folder is empty") }
                    return@launch
                }

                var successCount = 0
                files.forEachIndexed { index, (file, relPath) ->
                    updateNotification(1, "Uploading (${index + 1}/${files.size}): ${file.name}")
                    val inputStream = contentResolver.openInputStream(file.uri) ?: return@forEachIndexed
                    val success = ApiClient.uploadFile(serverIp, serverPort, inputStream, relPath, pairingCode, file.length()) { _, _ -> }
                    if (success) successCount++
                }
                
                withContext(Dispatchers.Main) { 
                    showCompletionNotification(System.currentTimeMillis().toInt(), "Uploaded $successCount/${files.size} files from ${rootDoc.name}") 
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(System.currentTimeMillis().toInt(), "Folder upload error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForeground(STOP_FOREGROUND_DETACH)
                stopSelf(startId)
            }
        }
    }

    private fun handleDownload(fileName: String, isDir: Boolean, serverIp: String, serverPort: Int, pairingCode: String, startId: Int) {
        activeTasks.incrementAndGet()
        val displayTitle = if (isDir) "Downloading folder $fileName..." else "Downloading $fileName..."
        startForegroundCompat(3, createNotification(displayTitle))
        scope.launch {
            try {
                val settings = SettingsManager(this@FileTransferService)
                val customUri = settings.downloadUri
                var startTime = System.currentTimeMillis()
                
                ApiClient.downloadFile(serverIp, serverPort, fileName, pairingCode) { encryptedStream, total ->
                    if (isDir) {
                        val tempZip = File(cacheDir, "download_${System.currentTimeMillis()}.zip")
                        FileOutputStream(tempZip).use { output ->
                            ApiClient.decryptStream(encryptedStream, output, pairingCode) { downloaded ->
                                val currentTime = System.currentTimeMillis()
                                if (currentTime - startTime > 1000) {
                                    val progress = if (total > 0) (downloaded * 100 / total).toInt() else -1
                                    updateNotification(3, "Downloading folder: ${if (progress >= 0) "$progress%" else ""}")
                                    startTime = currentTime
                                }
                            }
                        }
                        
                        if (customUri.isNotEmpty()) {
                            val rootDoc = DocumentFile.fromTreeUri(this@FileTransferService, Uri.parse(customUri))
                            if (rootDoc != null && rootDoc.canWrite()) {
                                var targetDir = rootDoc.findFile(fileName)
                                var count = 1
                                while (targetDir != null && targetDir.exists()) {
                                    targetDir = rootDoc.findFile("$fileName ($count)")
                                    count++
                                }
                                val finalTarget = rootDoc.createDirectory(if (count == 1) fileName else "$fileName (${count-1})") ?: throw Exception("Failed to create folder")
                                unzipToDoc(tempZip, finalTarget)
                                withContext(Dispatchers.Main) { showDownloadCompleteNotification(System.currentTimeMillis().toInt(), finalTarget.name ?: fileName, finalTarget.uri.toString()) }
                            } else throw Exception("Custom folder inaccessible")
                        } else {
                            val downloadsDir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                            var targetDir = File(downloadsDir, fileName)
                            var count = 1
                            while (targetDir.exists()) {
                                targetDir = File(downloadsDir, "$fileName ($count)")
                                count++
                            }
                            targetDir.mkdirs()
                            unzip(tempZip, targetDir)
                            withContext(Dispatchers.Main) { showDownloadCompleteNotification(System.currentTimeMillis().toInt(), targetDir.name) }
                        }
                        tempZip.delete()
                    } else {
                        if (customUri.isNotEmpty()) {
                            val rootDoc = DocumentFile.fromTreeUri(this@FileTransferService, Uri.parse(customUri))
                            if (rootDoc != null && rootDoc.canWrite()) {
                                var name = fileName
                                var count = 1
                                val nameWithoutExt = fileName.substringBeforeLast(".")
                                val ext = fileName.substringAfterLast(".", "")
                                
                                while (rootDoc.findFile(name) != null) {
                                    name = if (ext.isNotEmpty()) "$nameWithoutExt ($count).$ext" else "$nameWithoutExt ($count)"
                                    count++
                                }
                                
                                val mimeType = MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext.lowercase()) ?: "application/octet-stream"
                                val newFile = rootDoc.createFile(mimeType, name) ?: throw Exception("Failed to create file")
                                
                                contentResolver.openOutputStream(newFile.uri).use { output ->
                                    if (output == null) throw Exception("Failed to open output")
                                    ApiClient.decryptStream(encryptedStream, output, pairingCode) { downloaded ->
                                        val currentTime = System.currentTimeMillis()
                                        if (currentTime - startTime > 1000) {
                                            val progress = if (total > 0) (downloaded * 100 / total).toInt() else -1
                                            updateNotification(3, "Downloading: ${if (progress >= 0) "$progress%" else ""}")
                                            startTime = currentTime
                                        }
                                    }
                                }
                                withContext(Dispatchers.Main) { showDownloadCompleteNotification(System.currentTimeMillis().toInt(), newFile.name ?: fileName, rootDoc.uri.toString()) }
                            } else throw Exception("Custom folder inaccessible")
                        } else {
                            val downloadsDir = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS)
                            var outputFile = File(downloadsDir, fileName)
                            var count = 1
                            val nameWithoutExt = outputFile.nameWithoutExtension
                            val ext = outputFile.extension
                            
                            while (outputFile.exists()) {
                                val newName = if (ext.isNotEmpty()) "$nameWithoutExt ($count).$ext" else "$nameWithoutExt ($count)"
                                outputFile = File(downloadsDir, newName)
                                count++
                            }

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
                            withContext(Dispatchers.Main) { showDownloadCompleteNotification(System.currentTimeMillis().toInt(), outputFile.name) }
                        }
                    }
                }
            } catch (e: Exception) {
                withContext(Dispatchers.Main) { showCompletionNotification(System.currentTimeMillis().toInt(), "Download error: ${e.message}") }
            } finally {
                if (activeTasks.decrementAndGet() == 0) stopForeground(STOP_FOREGROUND_DETACH)
                stopSelf(startId)
            }
        }
    }

    private fun unzipToDoc(zipFile: File, targetRoot: DocumentFile) {
        ZipInputStream(FileInputStream(zipFile)).use { zis ->
            var entry: ZipEntry? = zis.nextEntry
            while (entry != null) {
                val pathParts = entry.name.replace("\\", "/").split("/").filter { it.isNotEmpty() }
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
        
        // Fix: Use the notification 'id' as the requestCode and add FLAG_UPDATE_CURRENT 
        // to prevent reusing the same intent from previous notifications.
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
        scope.cancel()
    }
}
