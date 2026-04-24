package com.kshare.android.sync

import com.kshare.android.api.ApiClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.InputStream
import java.security.MessageDigest
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

interface ClipboardGateway {
    suspend fun getText(serverIp: String, port: Int, pairingCode: String, channel: String = ""): String?
    suspend fun postText(
        serverIp: String,
        port: Int,
        text: String,
        pairingCode: String,
        append: Boolean = false,
        channel: String = ""
    ): Boolean
    suspend fun getImage(serverIp: String, port: Int, pairingCode: String): ByteArray?
    suspend fun postImage(serverIp: String, port: Int, bytes: ByteArray, pairingCode: String): Boolean
}

object ApiClipboardGateway : ClipboardGateway {
    override suspend fun getText(serverIp: String, port: Int, pairingCode: String, channel: String): String? {
        return ApiClient.getClipboard(serverIp, port, pairingCode, channel)
    }

    override suspend fun postText(
        serverIp: String,
        port: Int,
        text: String,
        pairingCode: String,
        append: Boolean,
        channel: String
    ): Boolean {
        return ApiClient.postClipboard(serverIp, port, text, pairingCode, append, channel)
    }

    override suspend fun getImage(serverIp: String, port: Int, pairingCode: String): ByteArray? {
        val request = Request.Builder()
            .url("https://$serverIp:$port/clipboard/image")
            .header("Authorization", "Bearer $pairingCode")
            .build()
        return try {
            withContext(Dispatchers.IO) {
                ApiClient.getSecureClient().newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@use null
                    response.body?.bytes()
                }
            }
        } catch (e: Exception) {
            null
        }
    }

    override suspend fun postImage(serverIp: String, port: Int, bytes: ByteArray, pairingCode: String): Boolean {
        val request = Request.Builder()
            .url("https://$serverIp:$port/clipboard/image")
            .header("Authorization", "Bearer $pairingCode")
            .post(bytes.toRequestBody("image/png".toMediaType()))
            .build()
        return try {
            withContext(Dispatchers.IO) {
                ApiClient.getSecureClient().newCall(request).execute().use { it.isSuccessful }
            }
        } catch (e: Exception) {
            false
        }
    }
}

class ClipboardSyncManager(
    private val gateway: ClipboardGateway = ApiClipboardGateway
) {
    private var lastSentText = ""
    private var lastReceivedText = ""
    private var lastSentImageHash = ""
    private var lastReceivedImageHash = ""

    fun shouldSendText(text: String): Boolean {
        return text.isNotEmpty() && text != lastSentText && text != lastReceivedText
    }

    fun rememberSentText(text: String) {
        lastSentText = text
    }

    fun shouldApplyRemoteText(text: String): Boolean {
        return text.isNotEmpty() && text != lastReceivedText
    }

    fun rememberReceivedText(text: String) {
        lastReceivedText = text
    }

    fun shouldSendImage(bytes: ByteArray): Boolean {
        val hash = hashBytes(bytes)
        return hash != lastSentImageHash && hash != lastReceivedImageHash
    }

    fun rememberSentImage(bytes: ByteArray) {
        lastSentImageHash = hashBytes(bytes)
    }

    fun shouldApplyRemoteImage(bytes: ByteArray): Boolean {
        return hashBytes(bytes) != lastReceivedImageHash
    }

    fun rememberReceivedImage(bytes: ByteArray) {
        lastReceivedImageHash = hashBytes(bytes)
    }

    suspend fun fetchText(serverIp: String, port: Int, pairingCode: String, channel: String = ""): String? {
        return gateway.getText(serverIp, port, pairingCode, channel)
    }

    suspend fun pushText(
        serverIp: String,
        port: Int,
        text: String,
        pairingCode: String,
        append: Boolean = false,
        channel: String = ""
    ): Boolean {
        if (!shouldSendText(text)) return false
        val success = gateway.postText(serverIp, port, text, pairingCode, append, channel)
        if (success) rememberSentText(text)
        return success
    }

    suspend fun fetchImage(serverIp: String, port: Int, pairingCode: String): ByteArray? {
        return gateway.getImage(serverIp, port, pairingCode)
    }

    suspend fun pushImage(serverIp: String, port: Int, bytes: ByteArray, pairingCode: String): Boolean {
        if (!shouldSendImage(bytes)) return false
        val success = gateway.postImage(serverIp, port, bytes, pairingCode)
        if (success) rememberSentImage(bytes)
        return success
    }

    private fun hashBytes(bytes: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        return digest.digest(bytes).joinToString("") { "%02x".format(it) }
    }
}
