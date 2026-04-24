package com.kshare.android.data

import com.google.gson.Gson
import com.google.gson.reflect.TypeToken

data class ServerInfo(
    val ip: String,
    val port: Int,
    val authCode: String,
    val lastSeen: Long = System.currentTimeMillis()
)

interface SettingsStore {
    fun getString(key: String, defaultValue: String?): String?
    fun putString(key: String, value: String?)
    fun remove(key: String)
}

class InMemorySettingsStore : SettingsStore {
    private val values = mutableMapOf<String, String?>()

    override fun getString(key: String, defaultValue: String?): String? {
        return values[key] ?: defaultValue
    }

    override fun putString(key: String, value: String?) {
        values[key] = value
    }

    override fun remove(key: String) {
        values.remove(key)
    }
}

class SettingsRepository(private val store: SettingsStore) {
    private val gson = Gson()

    private companion object {
        const val SERVER_IP = "server_ip"
        const val SERVER_PORT = "server_port"
        const val PAIRING_CODE = "pairing_code"
        const val DARK_MODE = "dark_mode"
        const val DOWNLOAD_URI = "download_uri"
        const val KNOWN_SERVERS = "known_servers"
        const val NETWORK_IP_CACHE = "network_ip_cache"
    }

    var serverIp: String
        get() = store.getString(SERVER_IP, "") ?: ""
        set(value) = store.putString(SERVER_IP, value)

    var serverPort: String
        get() = store.getString(SERVER_PORT, "") ?: ""
        set(value) = store.putString(SERVER_PORT, value)

    var pairingCode: String
        get() = store.getString(PAIRING_CODE, "") ?: ""
        set(value) = store.putString(PAIRING_CODE, value)

    var darkMode: String
        get() = store.getString(DARK_MODE, "system") ?: "system"
        set(value) = store.putString(DARK_MODE, value)

    var downloadUri: String
        get() = store.getString(DOWNLOAD_URI, "") ?: ""
        set(value) = store.putString(DOWNLOAD_URI, value)

    fun getKnownServers(): Map<String, ServerInfo> {
        val json = store.getString(KNOWN_SERVERS, "{}") ?: "{}"
        return parseKnownServers(json)
    }

    fun saveServer(certHash: String, ip: String, port: Int, authCode: String) {
        val servers = getKnownServers().toMutableMap()
        servers[certHash] = ServerInfo(ip, port, authCode, System.currentTimeMillis())
        store.putString(KNOWN_SERVERS, encodeKnownServers(servers))
    }

    fun removeServer(certHash: String) {
        val servers = getKnownServers().toMutableMap()
        if (servers.remove(certHash) != null) {
            store.putString(KNOWN_SERVERS, encodeKnownServers(servers))
        }
    }

    fun getLastServerIp(networkId: String): String? {
        val cache = store.getString(NETWORK_IP_CACHE, null) ?: return null
        return parseNetworkCache(cache)[networkId]
    }

    fun setLastServerIp(networkId: String, ip: String) {
        val cache = parseNetworkCache(store.getString(NETWORK_IP_CACHE, null))
        cache[networkId] = ip
        store.putString(NETWORK_IP_CACHE, encodeNetworkCache(cache))
    }

    fun clearServerIpCache() {
        store.remove(NETWORK_IP_CACHE)
    }

    fun getAllSavedIps(): Map<String, String> {
        val cache = store.getString(NETWORK_IP_CACHE, null) ?: return emptyMap()
        return parseNetworkCache(cache)
    }

    fun removeServerIp(networkId: String) {
        val cache = parseNetworkCache(store.getString(NETWORK_IP_CACHE, null))
        if (cache.remove(networkId) != null) {
            store.putString(NETWORK_IP_CACHE, encodeNetworkCache(cache))
        }
    }

    private fun parseKnownServers(json: String): Map<String, ServerInfo> {
        try {
            val type = object : TypeToken<Map<String, ServerInfo>>() {}.type
            return gson.fromJson<Map<String, ServerInfo>>(json, type) ?: emptyMap()
        } catch (_: Exception) {
            return emptyMap()
        }
    }

    private fun encodeKnownServers(servers: Map<String, ServerInfo>): String {
        return gson.toJson(servers)
    }

    private fun parseNetworkCache(json: String?): MutableMap<String, String> {
        if (json.isNullOrBlank()) return mutableMapOf()
        return try {
            val type = object : TypeToken<Map<String, String>>() {}.type
            val parsed = gson.fromJson<Map<String, String>>(json, type) ?: emptyMap()
            parsed.toMutableMap()
        } catch (_: Exception) {
            mutableMapOf()
        }
    }

    private fun encodeNetworkCache(cache: Map<String, String>): String {
        return gson.toJson(cache)
    }
}
