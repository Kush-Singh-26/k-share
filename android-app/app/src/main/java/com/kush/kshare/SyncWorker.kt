package com.kush.kshare

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.kush.kshare.api.ApiClient

class SyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val prefs = applicationContext.getSharedPreferences("settings", Context.MODE_PRIVATE)
        val gistUrl = prefs.getString("gist_url", "") ?: ""
        val jsonKey = prefs.getString("gist_json_key", "ip") ?: "ip"

        if (gistUrl.isEmpty()) return Result.success()

        return try {
            // Just update the IP in shared preferences
            ApiClient.fetchIpFromGist(gistUrl, jsonKey) { ip ->
                if (ip != null) {
                    prefs.edit().putString("server_ip", ip).apply()
                }
            }
            Result.success()
        } catch (e: Exception) {
            Result.retry()
        }
    }
}
