package com.kush.kshare

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters

class SyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val settings = SettingsManager(applicationContext)
        
        // Check if we have a valid LAN
        if (!NetworkScanner.hasValidLan(applicationContext)) {
            return Result.success()
        }
        
        val networkId = NetworkScanner.getNetworkId(applicationContext) ?: return Result.success()
        val cachedIp = settings.getLastServerIp(networkId) ?: return Result.success()
        val port = settings.serverPort.toIntOrNull() ?: 26260
        
        return try {
            // Quick ping to verify cached IP is still valid
            val isOnline = NetworkScanner.quickPing(cachedIp, port, settings.pairingCode)
            if (isOnline) {
                settings.serverIp = cachedIp
            }
            Result.success()
        } catch (e: Exception) {
            Result.retry()
        }
    }
}
