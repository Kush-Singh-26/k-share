package com.kshare.android.sync

import android.content.Context
import com.kshare.android.NetworkScanner
import com.kshare.android.SettingsManager
import com.kshare.android.api.ApiClient

interface DiscoverySyncSettings {
    var serverIp: String
    val serverPort: String
    val pairingCode: String
    fun getLastServerIp(networkId: String): String?
}

interface DiscoverySyncEnvironment {
    fun hasValidLan(): Boolean
    fun getNetworkId(): String?
    suspend fun ping(serverIp: String, port: Int, pairingCode: String): ApiClient.PingResult
}

sealed class DiscoverySyncResult {
    object Success : DiscoverySyncResult()
    object Retry : DiscoverySyncResult()
}

class AndroidDiscoverySyncEnvironment(private val context: Context) : DiscoverySyncEnvironment {
    override fun hasValidLan(): Boolean = NetworkScanner.hasValidLan(context)
    override fun getNetworkId(): String? = NetworkScanner.getNetworkId(context)
    override suspend fun ping(serverIp: String, port: Int, pairingCode: String): ApiClient.PingResult {
        return ApiClient.ping(serverIp, port, pairingCode)
    }
}

class DiscoverySyncTask(
    private val settings: DiscoverySyncSettings,
    private val environment: DiscoverySyncEnvironment
) {
    suspend fun run(): DiscoverySyncResult {
        if (!environment.hasValidLan()) {
            return DiscoverySyncResult.Success
        }

        val networkId = environment.getNetworkId() ?: return DiscoverySyncResult.Success
        val cachedIp = settings.getLastServerIp(networkId) ?: return DiscoverySyncResult.Success
        val port = settings.serverPort.toIntOrNull() ?: 26260

        return try {
            val isOnline = environment.ping(cachedIp, port, settings.pairingCode).success
            if (isOnline) {
                settings.serverIp = cachedIp
            }
            DiscoverySyncResult.Success
        } catch (e: Exception) {
            DiscoverySyncResult.Retry
        }
    }
}
