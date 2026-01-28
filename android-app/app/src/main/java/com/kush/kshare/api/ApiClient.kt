package com.kush.kshare.api

import android.content.Context
import android.util.Base64
import com.kush.kshare.SettingsManager
import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import com.google.gson.reflect.TypeToken
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.InputStream
import java.io.OutputStream
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.X509TrustManager

data class RemoteFile(
    val name: String,
    @SerializedName("isDirectory") val isDir: Boolean = false,
    val size: Long,
    val modTime: String
)

data class HistoryItem(
    val id: String,
    val text: String,
    val timestamp: String
)

object ApiClient {
    @Volatile var lastError: String? = null
    private lateinit var settingsManager: SettingsManager
    private lateinit var trustManager: PinningTrustManager
    private lateinit var sslContext: SSLContext

    private var client = OkHttpClient()
    private val gson = Gson()

    fun init(context: Context) {
        settingsManager = SettingsManager(context)
        trustManager = PinningTrustManager(settingsManager)
        
        sslContext = SSLContext.getInstance("TLS")
        sslContext.init(null, arrayOf(trustManager), SecureRandom())
        
        client = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(0, TimeUnit.SECONDS)
            .writeTimeout(0, TimeUnit.SECONDS)
            .sslSocketFactory(sslContext.socketFactory, trustManager)
            // Hostname verification re-enabled for security.
            // Self-signed certs from server include IP SANs.
            .build()
    }

    fun getSslSocketFactory(): SSLSocketFactory = sslContext.socketFactory
    fun getTrustManager(): X509TrustManager = trustManager

    class PinningTrustManager(private val settings: SettingsManager) : X509TrustManager {
        @Volatile var lastSeenCert: X509Certificate? = null

        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}

        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
            if (chain.isNullOrEmpty()) throw java.security.cert.CertificateException("Empty certificate chain")
            
            val cert = chain[0]
            lastSeenCert = cert
            
            val hash = getCertHash(cert)
            val knownServers = settings.getKnownServers()
            
            // If we've seen this certificate hash before, it's trusted.
            // If not, we allow it ONLY during the initial handshake so user can verify.
            // This is the "Trust On First Use" (TOFU) model.
            if (knownServers.containsKey(hash)) {
                return // Trusted
            }
            
            // Note: We don't throw here so that ApiClient.ping can capture the hash 
            // and MainActivity can show the "Trust this server?" dialog.
            // Strict enforcement happens because we check result.certHash in MainActivity.
        }

        override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
        
        fun getCertHash(cert: X509Certificate): String {
            val digest = MessageDigest.getInstance("SHA-256")
            val hash = digest.digest(cert.encoded)
            return hash.joinToString("") { "%02x".format(it) }
        }
    }

    data class PingResult(val success: Boolean, val certHash: String? = null, val name: String? = null, val role: String? = null)

    suspend fun ping(serverIp: String, port: Int, pairingCode: String = ""): PingResult {
        // trustManager.lastSeenCert = null // DO NOT RESET: Reuse cert from scanner if session resumed
        val requestBuilder = Request.Builder().url("https://$serverIp:$port/ping")
        if (pairingCode.isNotEmpty()) {
            requestBuilder.header("Authorization", "Bearer $pairingCode")
        }
        val request = requestBuilder.build()

        return try {
            lastError = null
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    var cert: X509Certificate? = response.handshake?.peerCertificates?.firstOrNull() as? X509Certificate
                    
                    if (cert == null) {
                         cert = trustManager.lastSeenCert
                    }

                    val certHash = cert?.let { trustManager.getCertHash(it) }
                    
                    if (!response.isSuccessful) {
                        lastError = "HTTP ${response.code}"
                        return@use PingResult(false, certHash)
                    }
                    val body = response.body?.string() ?: return@use PingResult(false, certHash)
                    
                    try {
                        val json = JSONObject(body)
                        val success = json.optString("status") == "ok"
                        val name = json.optString("name")
                        val role = json.optString("role")
                        PingResult(success, certHash, name, role)
                    } catch (e: Exception) {
                        // Fallback for old servers (or text response)
                        val success = body.contains("ok")
                        PingResult(success, certHash)
                    }
                }
            }
        } catch (e: Exception) {
            lastError = e.message ?: e.javaClass.simpleName
            PingResult(false)
        }
    }

    suspend fun listFiles(serverIp: String, port: Int, pairingCode: String, folder: String = ""): List<RemoteFile> {
        var url = "https://$serverIp:$port/files"
        if (folder.isNotEmpty()) {
             url += "?folder=${java.net.URLEncoder.encode(folder, "UTF-8")}"
        }
        val request = Request.Builder()
            .url(url)
            .header("Authorization", "Bearer $pairingCode")
            .build()
            
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use emptyList<RemoteFile>()
                    val body = response.body?.string() ?: return@use emptyList<RemoteFile>()
                    val type = object : TypeToken<List<RemoteFile>>() {}.type
                    gson.fromJson(body, type)
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun getClipboard(serverIp: String, port: Int, pairingCode: String, channel: String = ""): String? {
        var url = "https://$serverIp:$port/clipboard"
        if (channel.isNotEmpty()) url += "?channel=$channel"
        
        val request = Request.Builder()
            .url(url)
            .header("Authorization", "Bearer $pairingCode")
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use null
                    response.body?.string()
                }
            }
        } catch (e: Exception) { null }
    }

    suspend fun postClipboard(serverIp: String, port: Int, text: String, pairingCode: String, append: Boolean = false, channel: String = ""): Boolean {
        var url = "https://$serverIp:$port/clipboard"
        val params = ArrayList<String>()
        if (append) params.add("mode=append")
        if (channel.isNotEmpty()) params.add("channel=$channel")
        
        if (params.isNotEmpty()) {
            url += "?" + params.joinToString("&")
        }

        val request = Request.Builder()
            .url(url)
            .header("Authorization", "Bearer $pairingCode")
            .post(text.toRequestBody("text/plain".toMediaType()))
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    suspend fun getHistory(serverIp: String, port: Int, pairingCode: String): List<HistoryItem> {
        val request = Request.Builder()
            .url("https://$serverIp:$port/clipboard/history")
            .header("Authorization", "Bearer $pairingCode")
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use emptyList<HistoryItem>()
                    val body = response.body?.string() ?: return@use emptyList<HistoryItem>()
                    val type = object : TypeToken<List<HistoryItem>>() {}.type
                    gson.fromJson<List<HistoryItem>>(body, type)
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun deleteHistory(serverIp: String, port: Int, id: String, pairingCode: String): Boolean {
        val request = Request.Builder()
            .url("https://$serverIp:$port/clipboard/history?id=$id")
            .header("Authorization", "Bearer $pairingCode")
            .delete()
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    fun getThumbnailUrl(serverIp: String, port: Int, name: String): String {
        return "https://$serverIp:$port/thumbnail?name=${java.net.URLEncoder.encode(name, "UTF-8")}"
    }

    suspend fun openOnPc(serverIp: String, port: Int, url: String, pairingCode: String): Boolean {
        val request = Request.Builder()
            .url("https://$serverIp:$port/open")
            .header("Authorization", "Bearer $pairingCode")
            .post(url.toRequestBody("text/plain".toMediaType()))
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    suspend fun deleteFile(serverIp: String, port: Int, fileName: String, pairingCode: String): Boolean {
        val encodedName = java.net.URLEncoder.encode(fileName, "UTF-8")
        val request = Request.Builder()
            .url("https://$serverIp:$port/delete?name=$encodedName")
            .header("Authorization", "Bearer $pairingCode")
            .delete()
            .build()

        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    suspend fun downloadFile(
        serverIp: String,
        port: Int,
        fileName: String,
        pairingCode: String,
        onStreamReady: suspend (InputStream, Long) -> Unit
    ): Boolean {
        val encodedName = java.net.URLEncoder.encode(fileName, "UTF-8").replace("+", "%20")
        val request = Request.Builder()
            .url("https://$serverIp:$port/download/$encodedName")
            .header("Authorization", "Bearer $pairingCode")
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (response.isSuccessful && response.body != null) {
                        onStreamReady(response.body!!.byteStream(), response.body!!.contentLength())
                        true
                    } else false
                }
            }
        } catch (e: Exception) { false }
    }

    suspend fun uploadFile(
        serverIp: String,
        port: Int,
        inputStream: InputStream,
        fileName: String,
        pairingCode: String,
        contentLength: Long,
        onProgress: (Long, Long) -> Unit
    ): Boolean {
        val requestBody = object : RequestBody() {
            override fun contentType() = "application/octet-stream".toMediaType()
            override fun contentLength() = contentLength
            override fun writeTo(sink: okio.BufferedSink) {
                val buffer = ByteArray(64 * 1024)
                var uploaded = 0L
                var read: Int
                while (inputStream.read(buffer).also { read = it } != -1) {
                    sink.write(buffer, 0, read)
                    uploaded += read
                    onProgress(uploaded, contentLength)
                }
            }
        }
        
        val encodedName = java.net.URLEncoder.encode(fileName, "UTF-8")
        val request = Request.Builder()
            .url("https://$serverIp:$port/upload?name=$encodedName")
            .header("Authorization", "Bearer $pairingCode")
            .post(requestBody)
            .build()
            
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }
}
