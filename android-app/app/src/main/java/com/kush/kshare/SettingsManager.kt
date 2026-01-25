package com.kush.kshare

import android.content.Context

class SettingsManager(context: Context) {
    private val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    var serverIp: String
        get() = prefs.getString("server_ip", "") ?: ""
        set(value) = prefs.edit().putString("server_ip", value).apply()

    var serverPort: String
        get() = prefs.getString("server_port", "") ?: ""
        set(value) = prefs.edit().putString("server_port", value).apply()

    var pairingCode: String
        get() = prefs.getString("pairing_code", "") ?: ""
        set(value) = prefs.edit().putString("pairing_code", value).apply()

    var darkMode: String
        get() = prefs.getString("dark_mode", "system") ?: "system"
        set(value) = prefs.edit().putString("dark_mode", value).apply()

    var downloadUri: String
        get() = prefs.getString("download_uri", "") ?: ""
        set(value) = prefs.edit().putString("download_uri", value).apply()

    // Network-IP cache for Context-Aware Auto-Connect
    private val ipCacheKey = "network_ip_cache"

    fun getLastServerIp(networkId: String): String? {
        val cache = prefs.getString(ipCacheKey, null) ?: return null
        return try {
            org.json.JSONObject(cache).optString(networkId, null)
        } catch (e: Exception) { null }
    }

    fun setLastServerIp(networkId: String, ip: String) {
        val cache = try {
            org.json.JSONObject(prefs.getString(ipCacheKey, null) ?: "{}")
        } catch (e: Exception) { org.json.JSONObject() }
        cache.put(networkId, ip)
        prefs.edit().putString(ipCacheKey, cache.toString()).apply()
    }

    fun clearServerIpCache() {
        prefs.edit().remove(ipCacheKey).apply()
    }

    fun getAllSavedIps(): Map<String, String> {
        val cache = prefs.getString(ipCacheKey, null) ?: return emptyMap()
        return try {
            val json = org.json.JSONObject(cache)
            val result = mutableMapOf<String, String>()
            json.keys().forEach { key ->
                result[key] = json.getString(key)
            }
            result
        } catch (e: Exception) { emptyMap() }
    }

    fun removeServerIp(networkId: String) {
        val cache = try {
            org.json.JSONObject(prefs.getString(ipCacheKey, null) ?: "{}")
        } catch (e: Exception) { return }
        cache.remove(networkId)
        prefs.edit().putString(ipCacheKey, cache.toString()).apply()
    }
}
