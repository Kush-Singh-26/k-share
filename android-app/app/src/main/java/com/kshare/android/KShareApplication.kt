package com.kshare.android

import android.app.Application
import com.kshare.android.api.ApiClient

class KShareApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        ApiClient.init(this)
        ThumbnailCache.init(this)
    }
}
