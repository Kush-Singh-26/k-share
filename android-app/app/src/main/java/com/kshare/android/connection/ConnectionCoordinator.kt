package com.kshare.android.connection

import com.kshare.android.NetworkScanner
import com.kshare.android.SettingsManager
import com.kshare.android.api.ApiClient

interface ConnectionSettings {
    var pairingCode: String
    var serverIp: String
    var serverPort: String
    fun getKnownServers(): Map<String, com.kshare.android.data.ServerInfo>
    fun saveServer(certHash: String, ip: String, port: Int, authCode: String)
    fun getLastServerIp(networkId: String): String?
    fun setLastServerIp(networkId: String, ip: String)
}

sealed class ConnectionOutcome {
    data class Connected(
        val ip: String,
        val port: Int,
        val certHash: String,
        val role: String?,
        val isGuest: Boolean
    ) : ConnectionOutcome()

    data class TrustRequired(
        val ip: String,
        val port: Int,
        val certHash: String
    ) : ConnectionOutcome()

    data class Failed(val message: String) : ConnectionOutcome()

    object NoLan : ConnectionOutcome()
    object NotFound : ConnectionOutcome()
}

interface PingGateway {
    suspend fun ping(serverIp: String, port: Int, pairingCode: String): ApiClient.PingResult
}

interface DiscoveryGateway {
    fun hasValidLan(): Boolean
    fun getNetworkId(): String?
    suspend fun findServer(
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): String?
}

object RealPingGateway : PingGateway {
    override suspend fun ping(serverIp: String, port: Int, pairingCode: String): ApiClient.PingResult {
        return ApiClient.ping(serverIp, port, pairingCode)
    }
}



class AndroidDiscoveryGateway(private val context: android.content.Context) : DiscoveryGateway {
    override fun hasValidLan(): Boolean = NetworkScanner.hasValidLan(context)
    override fun getNetworkId(): String? = NetworkScanner.getNetworkId(context)
    override suspend fun findServer(
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): String? = NetworkScanner.findServer(context, port, pairingCode, onStatus)
}

class ConnectionCoordinator(
    private val settings: ConnectionSettings,
    private val pingGateway: PingGateway = RealPingGateway,
    private val discoveryGateway: DiscoveryGateway
) {
    suspend fun verifyConnection(
        ip: String,
        port: Int,
        silent: Boolean
    ): ConnectionOutcome {
        val result = pingGateway.ping(ip, port, settings.pairingCode)
        val certHash = result.certHash
            ?: return ConnectionOutcome.Failed(ApiClient.lastError ?: "No certificate found")

        if (!result.success && result.role == null) {
            return ConnectionOutcome.Failed(ApiClient.lastError ?: "Handshake failed")
        }

        val isGuest = result.role == "guest"
        val known = settings.getKnownServers()[certHash]

        if (known != null) {
            if (known.ip != ip) {
                settings.saveServer(certHash, ip, port, known.authCode)
            }
            settings.serverIp = ip
            discoveryGateway.getNetworkId()?.let {
                settings.setLastServerIp(it, ip)
            }
            return ConnectionOutcome.Connected(ip, port, certHash, result.role, isGuest)
        }

        return if (silent) {
            ConnectionOutcome.Failed("Server not trusted")
        } else {
            ConnectionOutcome.TrustRequired(ip, port, certHash)
        }
    }

    suspend fun discoverConnection(
        port: Int,
        silent: Boolean,
        onStatus: (String) -> Unit
    ): ConnectionOutcome {
        if (!discoveryGateway.hasValidLan()) {
            return ConnectionOutcome.NoLan
        }

        // 1. Try currently entered IP first (Manual Override/Fast Refresh)
        val currentIp = settings.serverIp.trim()
        if (currentIp.isNotEmpty()) {
            onStatus("Checking $currentIp...")
            when (val current = verifyConnection(currentIp, port, silent)) {
                is ConnectionOutcome.Connected -> return current
                is ConnectionOutcome.TrustRequired -> if (!silent) return current
                is ConnectionOutcome.Failed -> {
                    // Log but continue if it was just a manual check that failed
                }
                else -> Unit
            }
        }

        // 2. Try cached IP for this network
        val networkId = discoveryGateway.getNetworkId()
        if (networkId != null) {
            val cachedIp = settings.getLastServerIp(networkId)
            if (cachedIp != null && cachedIp != currentIp) {
                onStatus("Checking cached server...")
                when (val cached = verifyConnection(cachedIp, port, true)) {
                    is ConnectionOutcome.Connected -> return cached
                    else -> Unit
                }
            }
        }

        // 3. Fallback to full discovery
        val foundIp = try {
            discoveryGateway.findServer(port, settings.pairingCode, onStatus)
        } catch (e: Exception) {
            return ConnectionOutcome.Failed(e.message ?: e.javaClass.simpleName)
        } ?: return ConnectionOutcome.NotFound

        return verifyConnection(foundIp, port, silent)
    }

    fun trustPendingServer(
        ip: String,
        port: Int,
        certHash: String
    ): ConnectionOutcome {
        val known = settings.getKnownServers()[certHash]
        val authCode = known?.authCode ?: settings.pairingCode
        settings.saveServer(certHash, ip, port, authCode)
        settings.serverIp = ip
        discoveryGateway.getNetworkId()?.let {
            settings.setLastServerIp(it, ip)
        }
        return ConnectionOutcome.Connected(ip, port, certHash, null, false)
    }
}
