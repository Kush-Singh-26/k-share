package com.kshare.android.feature.history

import com.kshare.android.api.ApiClient
import com.kshare.android.api.HistoryItem

interface HistoryGateway {
    suspend fun getHistory(serverIp: String, port: Int, pairingCode: String): List<HistoryItem>
    suspend fun deleteHistory(serverIp: String, port: Int, id: String, pairingCode: String): Boolean
}

object RealHistoryGateway : HistoryGateway {
    override suspend fun getHistory(serverIp: String, port: Int, pairingCode: String): List<HistoryItem> {
        return ApiClient.getHistory(serverIp, port, pairingCode)
    }

    override suspend fun deleteHistory(serverIp: String, port: Int, id: String, pairingCode: String): Boolean {
        return ApiClient.deleteHistory(serverIp, port, id, pairingCode)
    }
}
