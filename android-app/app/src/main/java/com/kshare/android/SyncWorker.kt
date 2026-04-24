package com.kshare.android

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.kshare.android.sync.AndroidDiscoverySyncEnvironment
import com.kshare.android.sync.DiscoverySyncTask
import com.kshare.android.sync.DiscoverySyncResult

class SyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val task = DiscoverySyncTask(
            settings = SettingsManager(applicationContext),
            environment = AndroidDiscoverySyncEnvironment(applicationContext)
        )
        return when (task.run()) {
            DiscoverySyncResult.Success -> Result.success()
            DiscoverySyncResult.Retry -> Result.retry()
        }
    }
}
