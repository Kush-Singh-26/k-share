package com.kush.kshare

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import com.kush.kshare.api.ApiClient
import kotlinx.coroutines.*
import kotlinx.coroutines.channels.Channel
import okhttp3.OkHttpClient
import okhttp3.Request
import java.net.Inet4Address
import java.net.InetSocketAddress
import java.net.NetworkInterface
import java.net.Socket
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

data class LinkAddressInfo(val ip: String, val prefixLength: Int)

object NetworkScanner {

    private const val WORKER_COUNT = 60
    private const val CONNECT_TIMEOUT_MS = 150

    // ==================== 1. CONNECTIVITY GATEKEEPER ====================

    fun hasValidLan(context: Context): Boolean {
        val cm = context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        
        val activeNetwork = cm.activeNetwork
        val caps = activeNetwork?.let { cm.getNetworkCapabilities(it) }
        if (caps != null) {
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) ||
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) {
                return true
            }
        }

        return try {
            NetworkInterface.getNetworkInterfaces()?.asSequence()?.any { iface ->
                val name = iface.name.lowercase()
                val isLanInterface = name.startsWith("wlan") || 
                                     name.startsWith("ap") || 
                                     name.startsWith("rndis") ||
                                     name.startsWith("eth")
                
                if (isLanInterface && iface.isUp) {
                    iface.inetAddresses.asSequence().any { addr ->
                        addr is Inet4Address && !addr.isLoopbackAddress
                    }
                } else false
            } ?: false
        } catch (e: Exception) {
            false
        }
    }

    // ==================== 2. NETWORK IDENTIFICATION ====================

    fun getNetworkId(context: Context): String? {
        val info = getLinkAddressInfo(context) ?: return null
        val parts = info.ip.split(".")
        return if (parts.size == 4) "${parts[0]}.${parts[1]}.${parts[2]}" else null
    }

    fun getLinkAddressInfo(context: Context): LinkAddressInfo? {
        // PRIORITY 1: Check for hotspot/tethering interfaces first
        // These have names like "ap0", "wlan1", "rndis0", "swlan0"
        try {
            val hotspotInfo = NetworkInterface.getNetworkInterfaces()?.asSequence()?.mapNotNull { iface ->
                val name = iface.name.lowercase()
                // Hotspot interface names vary by manufacturer
                val isHotspotInterface = name.startsWith("ap") || 
                                         name.startsWith("swlan") ||
                                         name.startsWith("rndis") ||
                                         (name.startsWith("wlan") && name != "wlan0") // wlan1, wlan2 are often hotspot
                
                if (isHotspotInterface && iface.isUp) {
                    iface.interfaceAddresses.firstOrNull { 
                        it.address is Inet4Address && !it.address.isLoopbackAddress 
                    }?.let { 
                        LinkAddressInfo(it.address.hostAddress ?: return@let null, it.networkPrefixLength.toInt()) 
                    }
                } else null
            }?.firstOrNull()
            
            if (hotspotInfo != null) return hotspotInfo
        } catch (e: Exception) { /* continue to fallback */ }

        // PRIORITY 2: Check wlan0 (standard WiFi) 
        try {
            val wlanInfo = NetworkInterface.getNetworkInterfaces()?.asSequence()?.mapNotNull { iface ->
                val name = iface.name.lowercase()
                if (name == "wlan0" && iface.isUp) {
                    iface.interfaceAddresses.firstOrNull { 
                        it.address is Inet4Address && !it.address.isLoopbackAddress 
                    }?.let { 
                        LinkAddressInfo(it.address.hostAddress ?: return@let null, it.networkPrefixLength.toInt()) 
                    }
                } else null
            }?.firstOrNull()
            
            if (wlanInfo != null) return wlanInfo
        } catch (e: Exception) { /* continue to fallback */ }

        // PRIORITY 3: ConnectivityManager (may return mobile data in hotspot mode)
        val cm = context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val activeNetwork = cm.activeNetwork
        val linkProps = activeNetwork?.let { cm.getLinkProperties(it) }
        
        if (linkProps != null) {
            for (linkAddr in linkProps.linkAddresses) {
                val addr = linkAddr.address
                if (addr is Inet4Address && !addr.isLoopbackAddress) {
                    // Skip carrier-grade NAT IPs (100.64.0.0/10)
                    val firstOctet = addr.address[0].toInt() and 0xFF
                    val secondOctet = addr.address[1].toInt() and 0xFF
                    if (firstOctet == 100 && secondOctet in 64..127) continue
                    
                    return LinkAddressInfo(addr.hostAddress ?: continue, linkAddr.prefixLength)
                }
            }
        }

        return null
    }

    // ==================== 3. QUICK PING ====================

    private val quickPingClient = OkHttpClient.Builder()
        .connectTimeout(200, TimeUnit.MILLISECONDS)
        .readTimeout(200, TimeUnit.MILLISECONDS)
        .build()

    suspend fun quickPing(ip: String, port: Int, pairingCode: String): Boolean {
        return withContext(Dispatchers.IO) {
            try {
                val request = Request.Builder().url("http://$ip:$port/ping").build()
                quickPingClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) return@withContext false
                    val body = response.body?.string() ?: return@withContext false
                    ApiClient.verifyPingResponse(body, pairingCode)
                }
            } catch (e: Exception) {
                false
            }
        }
    }

    // ==================== 4. PRIORITY ZONE SCANNING ====================

    /**
     * Scans network in priority zones:
     * Zone 1: Own /24 block (~254 IPs)
     * Zone 2: ±2 blocks (~1270 IPs) 
     * Zone 3: ±10 blocks (~5334 IPs) - auto-triggers if Zone 2 fails
     */
    /**
     * Scans network in priority zones:
     * Zone 1: Local /24
     * Zone 2: Neighbors (±2)
     * Zone 3: Deep (±10)
     * Zone 4: Wide (±30) 
     * Zone 5: common roots (0,1,2)
     * Zone 6: Full subnet
     */
    suspend fun findServer(
        context: Context,
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): String? = withContext(Dispatchers.IO) {
        
        val linkInfo = getLinkAddressInfo(context)
        if (linkInfo == null) {
            onStatus("No network")
            return@withContext null
        }

        val parts = linkInfo.ip.split(".").map { it.toInt() }
        if (parts.size != 4) {
            onStatus("Invalid IP")
            return@withContext null
        }

        val myThirdOctet = parts[2]
        val basePrefix = "${parts[0]}.${parts[1]}"
        val scannedBlocks = mutableSetOf<Int>()

        // Helper to scan a set of blocks
        suspend fun scanTargetBlocks(blocks: Iterable<Int>, zoneLabel: String): String? {
            // Strict safety: 0..255 check and deduplication
            val targets = blocks.filter { it in 0..255 && !scannedBlocks.contains(it) }.distinct()
            if (targets.isEmpty()) return null
            
            scannedBlocks.addAll(targets)
            
            // UX: Show range being scanned
            val min = targets.minOrNull() ?: 0
            val max = targets.maxOrNull() ?: 0
            val rangeStr = if (min == max) "$min" else "$min-$max"
            onStatus("$zoneLabel: $basePrefix.[$rangeStr].*")

            val ips = targets.flatMap { block ->
                (1..254).map { host -> "$basePrefix.$block.$host" }
            }.shuffled()

            return scanZone(ips, port, pairingCode)
        }

        // Zone 1: Own /24 block (Exact Match)
        var result = scanTargetBlocks(listOf(myThirdOctet), "Zone 1 (Local)")
        if (result != null) return@withContext result

        // Zone 2: ±2 blocks (Neighbors)
        result = scanTargetBlocks((myThirdOctet - 2)..(myThirdOctet + 2), "Zone 2 (Neighbors)")
        if (result != null) return@withContext result

        // Zone 3: ±10 blocks (Deep)
        result = scanTargetBlocks((myThirdOctet - 10)..(myThirdOctet + 10), "Zone 3 (Deep)")
        if (result != null) return@withContext result

        // Zone 4: ±30 blocks (Wide) - Catches 0 vs 11 cases
        result = scanTargetBlocks((myThirdOctet - 30)..(myThirdOctet + 30), "Zone 4 (Wide)")
        if (result != null) return@withContext result

        // Zone 5: Common Roots (0, 1, 2)
        result = scanTargetBlocks(listOf(0, 1, 2), "Zone 5 (Roots)")
        if (result != null) return@withContext result

        // Zone 6: Full Subnet Scan (if /16 to /23)
        // Ensure we don't scan impossibly large ranges (skip if < /16)
        val prefix = linkInfo.prefixLength
        if (prefix in 16..23) {
             val size = 1 shl (24 - prefix) // Number of /24 blocks
             // Calculate start of the block range for the 3rd octet
             // e.g. /20 covers 16 blocks. mask = ~15.
             val startBlock = myThirdOctet and (size - 1).inv()
             val endBlock = startBlock + size - 1
             
             result = scanTargetBlocks(startBlock..endBlock, "Zone 6 (Full Subnet)")
             if (result != null) return@withContext result
        }

        onStatus("Server not found")
        null
    }

    /**
     * Scans a zone of IPs using a bounded channel with 60 worker coroutines
     */
    private suspend fun scanZone(
        ips: List<String>,
        port: Int,
        pairingCode: String
    ): String? = coroutineScope {
        if (ips.isEmpty()) return@coroutineScope null

        val channel = Channel<String>(capacity = Channel.BUFFERED)
        val found = AtomicReference<String?>(null)
        val cancelled = AtomicBoolean(false)

        val scanClient = OkHttpClient.Builder()
            .connectTimeout(CONNECT_TIMEOUT_MS.toLong(), TimeUnit.MILLISECONDS)
            .readTimeout(CONNECT_TIMEOUT_MS.toLong(), TimeUnit.MILLISECONDS)
            .build()

        // Launch worker pool
        val workers = List(WORKER_COUNT) {
            launch(Dispatchers.IO) {
                for (ip in channel) {
                    if (cancelled.get()) break // Stop processing immediately
                    
                    try {
                        // TCP connect check
                        Socket().use { socket ->
                            socket.connect(InetSocketAddress(ip, port), CONNECT_TIMEOUT_MS)
                        }
                        
                        if (cancelled.get()) break
                        
                        // Application layer handshake
                        val request = Request.Builder().url("http://$ip:$port/ping").build()
                        scanClient.newCall(request).execute().use { response ->
                            if (response.isSuccessful) {
                                val body = response.body?.string()
                                if (body != null && ApiClient.verifyPingResponse(body, pairingCode)) {
                                    if (found.compareAndSet(null, ip)) {
                                        cancelled.set(true)
                                        channel.close() // Close channel to stop feeding
                                    }
                                }
                            }
                        }
                    } catch (e: Exception) {
                        // Connection failed, continue to next IP
                    }
                }
            }
        }

        // Feed IPs to channel
        launch {
            try {
                for (ip in ips) {
                    if (cancelled.get()) break
                    channel.send(ip)
                }
            } catch (e: kotlinx.coroutines.channels.ClosedSendChannelException) {
                // Channel closed because server was found - this is expected
            } finally {
                try { channel.close() } catch (e: Exception) { }
            }
        }

        // Wait for workers to finish
        workers.forEach { it.join() }
        
        found.get()
    }

    /**
     * Verifies a single manually-entered IP
     */
    suspend fun verifyManualIp(
        ip: String,
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): Boolean = withContext(Dispatchers.IO) {
        onStatus("Verifying $ip...")
        
        try {
            // TCP connect check
            Socket().use { socket ->
                socket.connect(InetSocketAddress(ip, port), 500)
            }
            
            // Ping handshake
            val success = quickPing(ip, port, pairingCode)
            if (success) {
                onStatus("")
            } else {
                onStatus("Invalid pairing code")
            }
            success
        } catch (e: Exception) {
            onStatus("Connection failed")
            false
        }
    }
}
