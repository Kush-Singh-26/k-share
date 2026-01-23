package com.kush.kshare.api

import android.util.Base64
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.InputStream
import java.io.OutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.concurrent.TimeUnit
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

data class RemoteFile(
    val name: String,
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

    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.SECONDS)
        .writeTimeout(0, TimeUnit.SECONDS)
        .build()

    private val gson = Gson()
    private val secureRandom = SecureRandom()

    private fun getEncryptionKey(pairingCode: String): SecretKeySpec {
        val digest = MessageDigest.getInstance("SHA-256")
        val keyBytes = digest.digest(pairingCode.toByteArray())
        return SecretKeySpec(keyBytes, "AES")
    }

    private fun encryptData(data: ByteArray, pairingCode: String): String {
        val key = getEncryptionKey(pairingCode)
        val nonce = ByteArray(12)
        secureRandom.nextBytes(nonce)

        val timestamp = System.currentTimeMillis() / 1000
        val payload = JSONObject().apply {
            put("t", timestamp)
            put("d", Base64.encodeToString(data, Base64.NO_WRAP))
        }

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, nonce))
        val ciphertext = cipher.doFinal(payload.toString().toByteArray())

        val combined = ByteArray(nonce.size + ciphertext.size)
        System.arraycopy(nonce, 0, combined, 0, nonce.size)
        System.arraycopy(ciphertext, 0, combined, nonce.size, ciphertext.size)

        return Base64.encodeToString(combined, Base64.NO_WRAP)
    }

    private fun decryptData(encryptedStr: String, pairingCode: String): ByteArray? {
        return try {
            val combined = Base64.decode(encryptedStr, Base64.DEFAULT)
            val key = getEncryptionKey(pairingCode)
            val nonce = combined.copyOfRange(0, 12)
            val ciphertext = combined.copyOfRange(12, combined.size)

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, nonce))
            val plaintext = cipher.doFinal(ciphertext)

            val payload = JSONObject(String(plaintext))
            val t = payload.getLong("t")
            if (Math.abs((System.currentTimeMillis() / 1000) - t) > 600) return null

            Base64.decode(payload.getString("d"), Base64.DEFAULT)
        } catch (e: Exception) {
            null
        }
    }

    suspend fun decryptStream(input: InputStream, output: OutputStream, pairingCode: String, onProgress: (Long) -> Unit) {
        val key = getEncryptionKey(pairingCode)
        val nonce = ByteArray(12)
        if (input.read(nonce) != 12) throw Exception("Invalid stream")

        var chunkIndex = 0L
        val sizeBuf = ByteArray(4)
        var totalDecrypted = 0L

        while (true) {
            val read = input.read(sizeBuf)
            if (read == -1) break
            if (read != 4) throw Exception("Invalid chunk size")

            val length = ByteBuffer.wrap(sizeBuf).order(ByteOrder.LITTLE_ENDIAN).int
            val encryptedChunk = ByteArray(length)
            var offset = 0
            while (offset < length) {
                val r = input.read(encryptedChunk, offset, length - offset)
                if (r == -1) break
                offset += r
            }

            val currentNonce = nonce.clone()
            for (i in 0 until 8) {
                currentNonce[i] = (currentNonce[i].toInt() xor (chunkIndex ushr (i * 8)).toInt()).toByte()
            }

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, currentNonce))
            val plaintext = cipher.doFinal(encryptedChunk)
            output.write(plaintext)
            
            totalDecrypted += plaintext.size
            onProgress(totalDecrypted)
            chunkIndex++
        }
    }

    fun fetchIpFromGist(gistUrl: String, jsonKey: String, onResult: (String?) -> Unit) {
        if (gistUrl.isEmpty()) { onResult(null); return }
        val request = Request.Builder().url("$gistUrl?t=${System.currentTimeMillis()}").build()
        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: java.io.IOException) { onResult(null) }
            override fun onResponse(call: Call, response: Response) {
                val body = response.body?.string()
                if (response.isSuccessful && body != null) {
                    try {
                        val map = gson.fromJson<Map<String, String>>(body, object : TypeToken<Map<String, String>>() {}.type)
                        onResult(map[jsonKey])
                    } catch (e: Exception) { onResult(null) }
                } else { onResult(null) }
            }
        })
    }

    suspend fun ping(serverIp: String, port: Int, pairingCode: String): Boolean {
        val request = Request.Builder().url("http://$serverIp:$port/ping").build()
        return try {
            lastError = null
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) {
                        lastError = "HTTP ${response.code}"
                        return@withContext false
                    }
                    val body = response.body?.string() ?: return@withContext false
                    val decrypted = decryptData(body, pairingCode)
                    if (decrypted == null) {
                        lastError = "Decryption failed"
                        return@withContext false
                    }
                    String(decrypted).contains("ok")
                }
            }
        } catch (e: Exception) {
            lastError = e.message ?: e.javaClass.simpleName
            false
        }
    }

    suspend fun listFiles(serverIp: String, port: Int, pairingCode: String, folder: String = "tophone"): List<RemoteFile> {
        val endpoint = if (folder == "fromphone") "fromphone" else "tophone"
        val request = Request.Builder().url("http://$serverIp:$port/files/$endpoint").build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@withContext emptyList<RemoteFile>()
                    val body = response.body?.string() ?: return@withContext emptyList<RemoteFile>()
                    val decrypted = decryptData(body, pairingCode) ?: return@withContext emptyList<RemoteFile>()
                    val type = object : TypeToken<List<RemoteFile>>() {}.type
                    gson.fromJson(String(decrypted), type)
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun getClipboard(serverIp: String, port: Int, pairingCode: String): String? {
        val request = Request.Builder().url("http://$serverIp:$port/clipboard").build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@withContext null
                    val body = response.body?.string() ?: return@withContext null
                    val decrypted = decryptData(body, pairingCode) ?: return@withContext null
                    String(decrypted)
                }
            }
        } catch (e: Exception) { null }
    }

    suspend fun postClipboard(serverIp: String, port: Int, text: String, pairingCode: String, append: Boolean = false): Boolean {
        val encrypted = encryptData(text.toByteArray(), pairingCode)
        val url = "http://$serverIp:$port/clipboard" + (if (append) "?mode=append" else "")
        val request = Request.Builder().url(url).post(encrypted.toRequestBody("text/plain".toMediaType())).build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    suspend fun getHistory(serverIp: String, port: Int, pairingCode: String): List<HistoryItem> {
        val request = Request.Builder().url("http://$serverIp:$port/clipboard/history").build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@withContext emptyList<HistoryItem>()
                    val body = response.body?.string() ?: return@withContext emptyList<HistoryItem>()
                    val decrypted = decryptData(body, pairingCode) ?: return@withContext emptyList<HistoryItem>()
                    val type = object : TypeToken<List<HistoryItem>>() {}.type
                    gson.fromJson<List<HistoryItem>>(String(decrypted), type)
                }
            }
        } catch (e: Exception) { emptyList() }
    }

    suspend fun deleteHistory(serverIp: String, port: Int, id: String): Boolean {
        val request = Request.Builder()
            .url("http://$serverIp:$port/clipboard/history?id=$id")
            .delete()
            .build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    fun getThumbnailUrl(serverIp: String, port: Int, folder: String, name: String): String {
        return "http://$serverIp:$port/thumbnail?folder=$folder&name=${java.net.URLEncoder.encode(name, "UTF-8")}"
    }

    suspend fun openOnPc(serverIp: String, port: Int, url: String, pairingCode: String): Boolean {
        val encrypted = encryptData(url.toByteArray(), pairingCode)
        val request = Request.Builder().url("http://$serverIp:$port/open").post(encrypted.toRequestBody("text/plain".toMediaType())).build()
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
        val request = Request.Builder().url("http://$serverIp:$port/download/$fileName").build()
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
            override fun writeTo(sink: okio.BufferedSink) {
                encryptStreamBlocking(inputStream, sink.outputStream(), pairingCode, contentLength) { sent ->
                    onProgress(sent, contentLength)
                }
            }
        }
        val request = Request.Builder().url("http://$serverIp:$port/upload?name=$fileName").post(requestBody).build()
        return try {
            withContext(Dispatchers.IO) {
                client.newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) { false }
    }

    private fun encryptStreamBlocking(input: InputStream, output: OutputStream, pairingCode: String, totalPlainSize: Long, onProgress: (Long) -> Unit) {
        val key = getEncryptionKey(pairingCode)
        val nonce = ByteArray(12)
        secureRandom.nextBytes(nonce)
        output.write(nonce)

        val buffer = ByteArray(64 * 1024)
        var chunkIndex = 0L
        var totalRead = 0L

        while (true) {
            val n = input.read(buffer)
            if (n == -1) break

            val currentNonce = nonce.clone()
            for (i in 0 until 8) {
                currentNonce[i] = (currentNonce[i].toInt() xor (chunkIndex ushr (i * 8)).toInt()).toByte()
            }

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, currentNonce))
            val encryptedChunk = cipher.doFinal(buffer, 0, n)

            val sizeBuf = ByteBuffer.allocate(4).order(ByteOrder.LITTLE_ENDIAN).putInt(encryptedChunk.size).array()
            output.write(sizeBuf)
            output.write(encryptedChunk)

            totalRead += n
            onProgress(totalRead)
            chunkIndex++
        }
    }
}
