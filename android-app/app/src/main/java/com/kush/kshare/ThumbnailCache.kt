package com.kush.kshare

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
import com.kush.kshare.api.ApiClient

object ThumbnailCache {
    private val memoryCache: LruCache<String, Bitmap>
    private lateinit var cacheDir: File

    init {
        val maxMemory = (Runtime.getRuntime().maxMemory() / 1024).toInt()
        val cacheSize = maxMemory / 8
        memoryCache = object : LruCache<String, Bitmap>(cacheSize) {
            override fun sizeOf(key: String, bitmap: Bitmap): Int {
                return bitmap.byteCount / 1024
            }
        }
    }

    fun init(context: Context) {
        cacheDir = File(context.cacheDir, "thumbs")
        if (!cacheDir.exists()) cacheDir.mkdirs()
    }

    fun getFromMemory(key: String): Bitmap? = memoryCache.get(key)

    suspend fun get(key: String, url: String, pairingCode: String): Bitmap? {
        // 1. Memory
        memoryCache.get(key)?.let { return it }

        // 2. Disk
        val safeKey = key.replace("[^a-zA-Z0-9]".toRegex(), "_")
        val diskFile = File(cacheDir, safeKey)
        if (diskFile.exists()) {
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

        // 3. Network
        // 3. Network
        val client = OkHttpClient.Builder()
            .sslSocketFactory(ApiClient.getSslSocketFactory(), ApiClient.getTrustManager())
            .hostnameVerifier { _, _ -> true }
            .build()
            
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
                            FileOutputStream(diskFile).use { out ->
                                bitmap.compress(Bitmap.CompressFormat.JPEG, 80, out)
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
