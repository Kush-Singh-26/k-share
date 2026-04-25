package com.kshare.android

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import com.kshare.android.api.ApiClient
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

import kotlin.coroutines.resume
import kotlin.coroutines.suspendCoroutine
import kotlinx.coroutines.suspendCancellableCoroutine

data class LinkAddressInfo(val ip: String, val prefixLength: Int)

object NetworkScanner {

    private const val WORKER_COUNT = 60
    private const val CONNECT_TIMEOUT_MS = 400

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
        try {
            val hotspotInfo = NetworkInterface.getNetworkInterfaces()?.asSequence()?.mapNotNull { iface ->
                val name = iface.name.lowercase()
                val isHotspotInterface = name.startsWith("ap") || 
                                         name.startsWith("swlan") ||
                                         name.startsWith("rndis") ||
                                         (name.startsWith("wlan") && name != "wlan0")
                
                if (isHotspotInterface && iface.isUp) {
                    iface.interfaceAddresses.firstOrNull { 
                        it.address is Inet4Address && !it.address.isLoopbackAddress 
                    }?.let { 
                        LinkAddressInfo(it.address.hostAddress ?: return@let null, it.networkPrefixLength.toInt()) 
                    }
                } else null
            }?.firstOrNull()
            
            if (hotspotInfo != null) return hotspotInfo
        } catch (e: Exception) { }

        // PRIORITY 2: Check wlan0
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
        } catch (e: Exception) { }

        // PRIORITY 3: ConnectivityManager
        val cm = context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val activeNetwork = cm.activeNetwork
        val linkProps = activeNetwork?.let { cm.getLinkProperties(it) }
        
        if (linkProps != null) {
            for (linkAddr in linkProps.linkAddresses) {
                val addr = linkAddr.address
                if (addr is Inet4Address && !addr.isLoopbackAddress) {
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

    private fun getFastClient(timeoutMs: Long): OkHttpClient {
        return ApiClient.getDiscoveryClient(timeoutMs)
    }

    suspend fun quickPing(ip: String, port: Int, pairingCode: String): Boolean {
        return ApiClient.ping(ip, port).success
    }

    // ==================== 4. PRIORITY ZONE SCANNING & MDNS ====================

    private suspend fun tryMdnsDiscovery(
        context: Context,
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): String? = suspendCancellableCoroutine { cont ->
        val appContext = context.applicationContext
        val nsdManager = context.getSystemService(Context.NSD_SERVICE) as NsdManager
        val serviceType = "_kshare._tcp."
        val normalizedServiceType = serviceType.trimEnd('.')
        val wifiManager = appContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        val multicastLock = wifiManager.createMulticastLock("kshare-mdns").apply {
            setReferenceCounted(false)
            acquire()
        }
        
        var discoveryListener: NsdManager.DiscoveryListener? = null
        var resolved = false

        fun cleanup() {
            try {
                discoveryListener?.let { nsdManager.stopServiceDiscovery(it) }
            } catch (e: Exception) { }
            try {
                if (multicastLock.isHeld) multicastLock.release()
            } catch (e: Exception) { }
        }

        onStatus("mDNS: Scanning...")

        cont.invokeOnCancellation { cleanup() }

        discoveryListener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(regType: String) {}
            override fun onServiceFound(service: NsdServiceInfo) {
                val foundType = service.serviceType?.trimEnd('.')
                if (!resolved && foundType == normalizedServiceType) {
                    nsdManager.resolveService(service, object : NsdManager.ResolveListener {
                        override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {}
                        override fun onServiceResolved(serviceInfo: NsdServiceInfo) {
                            if (!resolved) {
                                resolved = true
                                val ip = serviceInfo.host.hostAddress
                                if (ip != null) {
                                    cleanup()
                                    if (cont.isActive) cont.resume(ip)
                                }
                            }
                        }
                    })
                }
            }
            override fun onServiceLost(service: NsdServiceInfo) {}
            override fun onDiscoveryStopped(serviceType: String) {}
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                cleanup()
                if (cont.isActive) cont.resume(null)
            }
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {}
        }

        try {
            nsdManager.discoverServices(serviceType, NsdManager.PROTOCOL_DNS_SD, discoveryListener)
            val timeoutJob = CoroutineScope(cont.context).launch {
                delay(2500)
                if (!resolved) {
                    resolved = true
                    cleanup()
                    if (cont.isActive) cont.resume(null)
                }
            }
            cont.invokeOnCancellation { timeoutJob.cancel(); cleanup() }
        } catch (e: Exception) {
            cleanup()
            if (cont.isActive) cont.resume(null)
        }
    }

    suspend fun findServer(
        context: Context,
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): String? = withContext(Dispatchers.IO) {
        val linkInfo = getLinkAddressInfo(context) ?: return@withContext null
        val parts = linkInfo.ip.split(".").map { it.toInt() }
        if (parts.size != 4) return@withContext null

        val myThirdOctet = parts[2]
        val basePrefix = "${parts[0]}.${parts[1]}"
        val scannedBlocks = mutableSetOf<Int>()

        val mdnsResult = tryMdnsDiscovery(context, port, pairingCode, onStatus)
        if (mdnsResult != null) return@withContext mdnsResult

        suspend fun scanTargetBlocks(blocks: Iterable<Int>, zoneLabel: String): String? {
            val targets = blocks.filter { it in 0..255 && !scannedBlocks.contains(it) }.distinct()
            if (targets.isEmpty()) return null
            scannedBlocks.addAll(targets)
            
            val min = targets.minOrNull() ?: 0
            val max = targets.maxOrNull() ?: 0
            onStatus("$zoneLabel: $basePrefix.[${if (min == max) "$min" else "$min-$max"}].*")

            val ips = targets.flatMap { block -> (1..254).map { host -> "$basePrefix.$block.$host" } }
            return scanZone(ips, port, pairingCode)
        }

        scanTargetBlocks(listOf(myThirdOctet), "Zone 1 (Local)")?.let { return@withContext it }
        scanTargetBlocks((myThirdOctet - 2)..(myThirdOctet + 2), "Zone 2 (Neighbors)")?.let { return@withContext it }
        scanTargetBlocks((myThirdOctet - 10)..(myThirdOctet + 10), "Zone 3 (Deep)")?.let { return@withContext it }
        scanTargetBlocks((myThirdOctet - 30)..(myThirdOctet + 30), "Zone 4 (Wide)")?.let { return@withContext it }
        scanTargetBlocks(listOf(0, 1, 2), "Zone 5 (Roots)")?.let { return@withContext it }

        val prefix = linkInfo.prefixLength
        if (prefix in 16..23) {
             val size = 1 shl (24 - prefix)
             val startBlock = myThirdOctet and (size - 1).inv()
             val endBlock = startBlock + size - 1
             scanTargetBlocks(startBlock..endBlock, "Zone 6 (Full Subnet)")?.let { return@withContext it }
        }

        onStatus("Not found")
        null
    }

    private suspend fun scanZone(
        ips: List<String>,
        port: Int,
        pairingCode: String
    ): String? = supervisorScope {
        if (ips.isEmpty()) return@supervisorScope null

        val channel = Channel<String>(capacity = 100)
        val found = AtomicReference<String?>(null)
        val scanClient = getFastClient(CONNECT_TIMEOUT_MS.toLong())

        val workers = List(WORKER_COUNT) {
            launch(Dispatchers.IO) {
                try {
                    for (ip in channel) {
                        try {
                            val url = "https://$ip:$port/ping"
                            val request = Request.Builder()
                                .url(url)
                                .header("Authorization", "Bearer $pairingCode")
                                .build()
                            scanClient.newCall(request).execute().use { response ->
                                if (response.isSuccessful) {
                                    val body = response.body?.string()
                                    if (body != null && body.contains("ok")) {
                                        if (found.compareAndSet(null, ip)) {
                                            channel.close()
                                            this@supervisorScope.coroutineContext.cancelChildren()
                                        }
                                    }
                                }
                            }
                        } catch (e: Exception) { }
                    }
                } catch (e: CancellationException) { }
            }
        }

        launch(Dispatchers.IO) {
            try {
                for (ip in ips) {
                    if (found.get() != null) break
                    channel.send(ip)
                }
            } catch (e: Exception) { } finally { channel.close() }
        }

        workers.forEach { 
            try { it.join() } catch (e: CancellationException) { }
        }
        
        found.get()
    }

    suspend fun verifyManualIp(
        ip: String,
        port: Int,
        pairingCode: String,
        onStatus: (String) -> Unit
    ): Boolean = withContext(Dispatchers.IO) {
        onStatus("Verifying $ip...")
        try {
            val success = quickPing(ip, port, pairingCode)
            if (success) onStatus("") else onStatus("Failed")
            success
        } catch (e: Exception) {
            onStatus("Failed")
            false
        }
    }
}
