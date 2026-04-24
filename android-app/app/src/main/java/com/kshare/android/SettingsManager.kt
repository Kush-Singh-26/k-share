package com.kshare.android

import android.content.Context
import android.content.SharedPreferences
import com.kshare.android.data.ServerInfo
import com.kshare.android.data.SettingsRepository
import com.kshare.android.data.SettingsStore
import com.kshare.android.connection.ConnectionSettings
import com.kshare.android.sync.DiscoverySyncSettings
import com.kshare.android.feature.main.MainSettingsSource

class SettingsManager(context: Context) : MainSettingsSource, ConnectionSettings, DiscoverySyncSettings {
    private val repository = SettingsRepository(PreferencesStore(context.getSharedPreferences("settings", Context.MODE_PRIVATE)))

    override fun getKnownServers(): Map<String, com.kshare.android.data.ServerInfo> {
        return repository.getKnownServers()
    }

    override fun saveServer(certHash: String, ip: String, port: Int, authCode: String) {
        repository.saveServer(certHash, ip, port, authCode)
    }

    fun removeServer(certHash: String) {
        repository.removeServer(certHash)
    }

    override var serverIp: String
        get() = repository.serverIp
        set(value) {
            repository.serverIp = value
        }

    override var serverPort: String
        get() = repository.serverPort
        set(value) {
            repository.serverPort = value
        }

    override var pairingCode: String
        get() = repository.pairingCode
        set(value) {
            repository.pairingCode = value
        }

    override var darkMode: String
        get() = repository.darkMode
        set(value) {
            repository.darkMode = value
        }

    var downloadUri: String
        get() = repository.downloadUri
        set(value) {
            repository.downloadUri = value
        }

    override fun getLastServerIp(networkId: String): String? = repository.getLastServerIp(networkId)

    override fun setLastServerIp(networkId: String, ip: String) {
        repository.setLastServerIp(networkId, ip)
    }

    fun clearServerIpCache() {
        repository.clearServerIpCache()
    }

    fun getAllSavedIps(): Map<String, String> = repository.getAllSavedIps()

    fun removeServerIp(networkId: String) {
        repository.removeServerIp(networkId)
    }

    private class PreferencesStore(private val prefs: SharedPreferences) : SettingsStore {
        override fun getString(key: String, defaultValue: String?): String? = prefs.getString(key, defaultValue)
        override fun putString(key: String, value: String?) {
            // Use commit() for synchronous write to prevent data loss on rapid updates
            prefs.edit().putString(key, value).commit()
        }

        override fun remove(key: String) {
            prefs.edit().remove(key).commit()
        }
    }
}
