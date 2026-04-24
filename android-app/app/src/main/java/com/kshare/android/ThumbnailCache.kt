package com.kshare.android

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.LruCache
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.io.FileOutputStream
import java.net.URL
import okhttp3.OkHttpClient
import com.kshare.android.api.ApiClient

object ThumbnailCache {
    private val memoryCache: LruCache<String, Bitmap>
    private var cacheDir: File? = null

    init {
        val maxMemory = (Runtime.getRuntime().maxMemory() / 1024).toInt()
        val cacheSize = maxMemory / 8
        memoryCache = object : LruCache<String, Bitmap>(cacheSize) {
            override fun sizeOf(key: String, bitmap: Bitmap): Int {
                return bitmap.byteCount / 1024
            }
        }
    }

    private const val MAX_DISK_CACHE_SIZE = 50 * 1024 * 1024L // 50 MB

    fun init(context: Context) {
        cacheDir = File(context.cacheDir, "thumbs").also { if (!it.exists()) it.mkdirs() }
        cleanupDiskCache()
    }

    private fun cleanupDiskCache() {
        val dir = cacheDir ?: return
        val files = dir.listFiles() ?: return
        val totalSize = files.sumOf { it.length() }
        if (totalSize > MAX_DISK_CACHE_SIZE) {
            // Sort by last modified (oldest first) and delete oldest files until under limit
            val sorted = files.sortedBy { it.lastModified() }
            var currentSize = totalSize
            for (file in sorted) {
                if (currentSize <= MAX_DISK_CACHE_SIZE * 0.7) break // Target 70% of max
                currentSize -= file.length()
                file.delete()
            }
        }
    }

    fun getFromMemory(key: String): Bitmap? = memoryCache.get(key)

    suspend fun get(key: String, url: String, pairingCode: String): Bitmap? {
        // 1. Memory
        memoryCache.get(key)?.let { return it }

        // 2. Disk
        val safeKey = key.replace("[^a-zA-Z0-9]".toRegex(), "_") + ".png"
        val dir = cacheDir
        val diskFile = if (dir != null) File(dir, safeKey) else null
        if (diskFile?.exists() == true) {
            return withContext(Dispatchers.IO) {
                try {
                    val bitmap = BitmapFactory.decodeFile(diskFile.absolutePath)
                    if (bitmap != null) memoryCache.put(key, bitmap)
                    bitmap
                } catch (e: Exception) {
                    null
                }
            }
        }

        // 3. Network - use secure client that enforces hostname verification/pinning
        val client = ApiClient.getSecureClient()

        return withContext(Dispatchers.IO) {
            try {
                val request = okhttp3.Request.Builder()
                    .url(url)
                    .addHeader("Authorization", "Bearer $pairingCode")
                    .build()
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use null
                    response.body?.byteStream()?.use { stream ->
                        val bitmap = BitmapFactory.decodeStream(stream)
                        if (bitmap != null) {
                            memoryCache.put(key, bitmap)
                            diskFile?.let { df ->
                                FileOutputStream(df).use { out ->
                                    bitmap.compress(Bitmap.CompressFormat.PNG, 100, out)
                                }
                            }
                        }
                        bitmap
                    }
                }
            } catch (e: Exception) {
                null
            }
        }
    }
}
