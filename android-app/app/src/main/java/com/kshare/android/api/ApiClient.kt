package com.kshare.android.api

import android.content.Context
import android.util.Log
import com.kshare.android.SettingsManager
import com.google.gson.annotations.SerializedName
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.InputStream
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.*
import java.net.Socket
import java.util.concurrent.ConcurrentHashMap

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
    private const val TAG = "ApiClient"
    @Volatile var lastError: String? = null
    private lateinit var settingsManager: SettingsManager
    private lateinit var trustManager: PinningTrustManager
    private lateinit var sslContext: SSLContext

    private lateinit var insecureClient: OkHttpClient
    private lateinit var secureClient: OkHttpClient

    @Volatile
    private var isInitialized = false

    fun init(context: Context) {
        if (isInitialized) return
        synchronized(this) {
            if (isInitialized) return
            settingsManager = SettingsManager(context)
            trustManager = PinningTrustManager(settingsManager)

            sslContext = SSLContext.getInstance("TLS")
            sslContext.init(null, arrayOf(trustManager), SecureRandom())

            insecureClient = OkHttpClient.Builder()
                .connectionPool(ConnectionPool(0, 1, TimeUnit.MILLISECONDS))
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(10, TimeUnit.SECONDS)
                .writeTimeout(10, TimeUnit.SECONDS)
                .sslSocketFactory(sslContext.socketFactory, trustManager)
                .hostnameVerifier { _, _ -> true }
                .build()

            val strictHostnameVerifier = HostnameVerifier { hostname: String?, session: SSLSession? ->
                try {
                    val defaultVerifier = HttpsURLConnection.getDefaultHostnameVerifier()
                    if (defaultVerifier.verify(hostname, session)) return@HostnameVerifier true
                } catch (e: Exception) { }

                try {
                    if (session != null) {
                        val peerCerts = session.peerCertificates
                        if (peerCerts.isNotEmpty()) {
                            val cert = peerCerts[0] as? X509Certificate ?: return@HostnameVerifier false
                            val hash = trustManager.getCertHash(cert)
                            return@HostnameVerifier settingsManager.getKnownServers().containsKey(hash)
                        }
                    }
                } catch (e: Exception) { }
                false
            }

            secureClient = OkHttpClient.Builder()
                .connectTimeout(15, TimeUnit.SECONDS)
                .readTimeout(0, TimeUnit.SECONDS)
                .writeTimeout(0, TimeUnit.SECONDS)
                .sslSocketFactory(sslContext.socketFactory, trustManager)
                .hostnameVerifier(strictHostnameVerifier)
                .build()

            isInitialized = true
        }
    }

    private fun ensureInitialized() {
        if (!isInitialized) {
            throw IllegalStateException("ApiClient not initialized. Call ApiClient.init(context) in Application.onCreate()")
        }
    }

    fun getSslSocketFactory(): SSLSocketFactory {
        ensureInitialized()
        return sslContext.socketFactory
    }
    fun getTrustManager(): X509TrustManager {
        ensureInitialized()
        return trustManager
    }
    fun getSecureClient(): OkHttpClient {
        ensureInitialized()
        return secureClient
    }

    private val insecureClientCache = ConcurrentHashMap<Long, OkHttpClient>()

    fun getInsecureClient(timeoutMs: Long = 15000L): OkHttpClient {
        ensureInitialized()
        return insecureClientCache.getOrPut(timeoutMs) {
            OkHttpClient.Builder()
                .connectionPool(ConnectionPool(10, 30, TimeUnit.SECONDS))
                .connectTimeout(timeoutMs, TimeUnit.MILLISECONDS)
                .readTimeout(timeoutMs, TimeUnit.MILLISECONDS)
                .writeTimeout(timeoutMs, TimeUnit.MILLISECONDS)
                .sslSocketFactory(sslContext.socketFactory, trustManager)
                .hostnameVerifier { _, _ -> true }
                .build()
        }
    }

    class PinningTrustManager(private val settings: SettingsManager) : X509ExtendedTrustManager() {
        private val recentCerts = ConcurrentHashMap<String, X509Certificate>()
        @Volatile var lastSeenCert: X509Certificate? = null

        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?, socket: Socket?) {}
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?, engine: SSLEngine?) {}

        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
            if (chain.isNullOrEmpty()) return
            val cert = chain[0]
            lastSeenCert = cert
            Log.d(TAG, "Captured cert from chain: ${getCertHash(cert).take(8)}")
        }

        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?, socket: Socket?) {
            if (!chain.isNullOrEmpty()) {
                val cert = chain[0]
                lastSeenCert = cert
                val host = (socket as? SSLSocket)?.inetAddress?.hostAddress
                if (host != null) {
                    recentCerts[host] = cert
                    Log.d(TAG, "Captured cert for host (socket): $host")
                }
            }
            checkServerTrusted(chain, authType)
        }

        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?, engine: SSLEngine?) {
            if (!chain.isNullOrEmpty()) {
                val cert = chain[0]
                lastSeenCert = cert
                val host = engine?.peerHost
                if (host != null) {
                    recentCerts[host] = cert
                    Log.d(TAG, "Captured cert for host (engine): $host")
                }
            }
            checkServerTrusted(chain, authType)
        }

        fun getRecentCert(host: String): X509Certificate? {
            return recentCerts[host] ?: lastSeenCert
        }

        override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()

        fun getCertHash(cert: X509Certificate): String {
            val digest = MessageDigest.getInstance("SHA-256")
            val hash = digest.digest(cert.encoded)
            return hash.joinToString("") { "%02x".format(it) }
        }
    }

    data class PingResult(val success: Boolean, val certHash: String? = null, val name: String? = null, val role: String? = null)

    private suspend fun getCertDirectly(host: String, port: Int): X509Certificate? = withContext(Dispatchers.IO) {
        var captured: X509Certificate? = null
        try {
            val tm = object : X509TrustManager {
                override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
                override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
                    captured = chain?.firstOrNull()
                }
                override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
            }
            val ctx = SSLContext.getInstance("TLS")
            ctx.init(null, arrayOf(tm), SecureRandom())

            val factory = ctx.socketFactory
            val socket = factory.createSocket() as SSLSocket
            socket.soTimeout = 5000
            socket.connect(java.net.InetSocketAddress(host, port), 5000)
            socket.startHandshake()
            val cert = socket.session.peerCertificates.firstOrNull() as? X509Certificate
            socket.close()
            cert ?: captured
        } catch (e: Exception) {
            Log.e(TAG, "getCertDirectly failed for $host:$port: ${e.message}")
            captured
        }
    }

    suspend fun ping(serverIp: String, port: Int, pairingCode: String = ""): PingResult {
        ensureInitialized()
        val url = "https://$serverIp:$port/ping"
        val request = Request.Builder()
            .url(url)
            .apply { if (pairingCode.isNotEmpty()) header("Authorization", "Bearer $pairingCode") }
            .header("Connection", "close")
            .build()

        return try {
            lastError = null
            withContext(Dispatchers.IO) {
                try {
                    insecureClient.newCall(request).execute().use { response ->
                        var cert: X509Certificate? = response.handshake?.peerCertificates?.firstOrNull() as? X509Certificate
                        if (cert == null) cert = trustManager.getRecentCert(serverIp)
                        if (cert == null) cert = getCertDirectly(serverIp, port)

                        val certHash = cert?.let { trustManager.getCertHash(it) }

                        if (!response.isSuccessful) {
                            lastError = "Server Error: ${response.code}"
                            return@use PingResult(false, certHash)
                        }

                        if (certHash == null) {
                             lastError = "No certificate found for $serverIp"
                             return@use PingResult(false)
                        }

                        val body = response.body?.string() ?: return@use PingResult(false, certHash)
                        ApiResponseParser.parsePingResponse(body, certHash)
                    }
                } catch (e: Exception) {
                    Log.e(TAG, "Ping failed for $serverIp: ${e.message}")
                    val cert = getCertDirectly(serverIp, port)
                    val certHash = cert?.let { trustManager.getCertHash(it) }
                    lastError = e.message ?: e.javaClass.simpleName
                    PingResult(false, certHash)
                }
            }
        } catch (e: Exception) {
            lastError = e.message ?: e.javaClass.simpleName
            PingResult(false)
        }
    }

    suspend fun listFiles(serverIp: String, port: Int, pairingCode: String, folder: String = ""): List<RemoteFile> {
        ensureInitialized()
        var url = "https://$serverIp:$port/files"
        if (folder.isNotEmpty()) url += "?folder=${java.net.URLEncoder.encode(folder, "UTF-8")}"
        val request = Request.Builder().url(url).header("Authorization", "Bearer $pairingCode").build()
        return try {
            withContext(Dispatchers.IO) {
                secureClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use emptyList()
                    ApiResponseParser.parseFiles(response.body?.string() ?: "")
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun searchFiles(serverIp: String, port: Int, pairingCode: String, query: String): List<RemoteFile> {
        ensureInitialized()
        val url = "https://$serverIp:$port/search"
        val requestBody = """{"query":"$query"}""".toRequestBody("application/json".toMediaType())
        val request = Request.Builder()
            .url(url)
            .header("Authorization", "Bearer $pairingCode")
            .post(requestBody)
            .build()
            
        return try {
            withContext(Dispatchers.IO) {
                secureClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use emptyList()
                    ApiResponseParser.parseFiles(response.body?.string() ?: "")
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun getClipboard(serverIp: String, port: Int, pairingCode: String, channel: String = ""): String? {
        ensureInitialized()
        var url = "https://$serverIp:$port/clipboard"
        if (channel.isNotEmpty()) url += "?channel=$channel"
        val request = Request.Builder().url(url).header("Authorization", "Bearer $pairingCode").build()
        return try {
            withContext(Dispatchers.IO) {
                secureClient.newCall(request).execute().use { response ->
                    if (response.isSuccessful) response.body?.string() else null
                }
            }
        } catch (e: Exception) { null }
    }

    suspend fun postClipboard(serverIp: String, port: Int, text: String, pairingCode: String, append: Boolean = false, channel: String = ""): Boolean {
        ensureInitialized()
        var url = "https://$serverIp:$port/clipboard"
        val params = ArrayList<String>()
        if (append) params.add("mode=append")
        if (channel.isNotEmpty()) params.add("channel=$channel")
        if (params.isNotEmpty()) url += "?" + params.joinToString("&")
        val request = Request.Builder().url(url).header("Authorization", "Bearer $pairingCode")
            .post(text.toRequestBody("text/plain".toMediaType())).build()
        return try {
            withContext(Dispatchers.IO) { secureClient.newCall(request).execute().use { it.isSuccessful } }
        } catch (e: Exception) { false }
    }

    suspend fun getHistory(serverIp: String, port: Int, pairingCode: String): List<HistoryItem> {
        ensureInitialized()
        val request = Request.Builder().url("https://$serverIp:$port/clipboard/history").header("Authorization", "Bearer $pairingCode").build()
        return try {
            withContext(Dispatchers.IO) {
                secureClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use emptyList()
                    ApiResponseParser.parseHistory(response.body?.string() ?: "")
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun deleteHistory(serverIp: String, port: Int, id: String, pairingCode: String): Boolean {
        ensureInitialized()
        val request = Request.Builder().url("https://$serverIp:$port/clipboard/history?id=$id").header("Authorization", "Bearer $pairingCode").delete().build()
        return try {
            withContext(Dispatchers.IO) { secureClient.newCall(request).execute().use { it.isSuccessful } }
        } catch (e: Exception) { false }
    }

    fun getThumbnailUrl(serverIp: String, port: Int, name: String): String {
        return "https://$serverIp:$port/thumbnail?name=${java.net.URLEncoder.encode(name, "UTF-8")}"
    }

    suspend fun openOnPc(serverIp: String, port: Int, url: String, pairingCode: String): Boolean {
        ensureInitialized()
        val request = Request.Builder().url("https://$serverIp:$port/open").header("Authorization", "Bearer $pairingCode")
            .post(url.toRequestBody("text/plain".toMediaType())).build()
        return try {
            withContext(Dispatchers.IO) { secureClient.newCall(request).execute().use { it.isSuccessful } }
        } catch (e: Exception) { false }
    }

    suspend fun deleteFile(serverIp: String, port: Int, fileName: String, pairingCode: String): Boolean {
        ensureInitialized()
        val request = Request.Builder().url("https://$serverIp:$port/delete?name=${java.net.URLEncoder.encode(fileName, "UTF-8")}")
            .header("Authorization", "Bearer $pairingCode").delete().build()
        return try {
            withContext(Dispatchers.IO) { secureClient.newCall(request).execute().use { it.isSuccessful } }
        } catch (e: Exception) { false }
    }

    suspend fun downloadFile(serverIp: String, port: Int, fileName: String, pairingCode: String, startOffset: Long = 0, onStreamReady: suspend (InputStream, Long) -> Unit): Boolean {
        ensureInitialized()
        val encodedName = java.net.URLEncoder.encode(fileName, "UTF-8").replace("+", "%20")
        val request = Request.Builder().url("https://$serverIp:$port/download/$encodedName").header("Authorization", "Bearer $pairingCode")
            .apply { if (startOffset > 0) header("Range", "bytes=$startOffset-") }.build()
        return try {
            withContext(Dispatchers.IO) {
                secureClient.newCall(request).execute().use { response ->
                    if (response.isSuccessful && response.body != null) {
                        onStreamReady(response.body!!.byteStream(), response.body!!.contentLength())
                        true
                    } else false
                }
            }
        } catch (e: Exception) { false }
    }

    suspend fun uploadFile(serverIp: String, port: Int, inputStream: InputStream, fileName: String, pairingCode: String, contentLength: Long, onProgress: (Long, Long) -> Unit): Boolean {
        ensureInitialized()
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
        val request = Request.Builder().url("https://$serverIp:$port/upload?name=${java.net.URLEncoder.encode(fileName, "UTF-8")}")
            .header("Authorization", "Bearer $pairingCode").post(requestBody).build()
        return try {
            withContext(Dispatchers.IO) {
                try { secureClient.newCall(request).execute().use { it.isSuccessful } }
                finally { try { inputStream.close() } catch (e: Exception) {} }
            }
        } catch (e: Exception) { false }
    }

}
