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
        
    var gistUrl: String
        get() = prefs.getString("gist_url", "") ?: ""
        set(value) = prefs.edit().putString("gist_url", value).apply()
        
    var gistJsonKey: String
        get() = prefs.getString("gist_json_key", "ip") ?: "ip"
        set(value) = prefs.edit().putString("gist_json_key", value).apply()
}
