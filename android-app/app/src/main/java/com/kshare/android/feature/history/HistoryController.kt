package com.kshare.android.feature.history

import com.kshare.android.api.HistoryItem

class HistoryController(
    private val gateway: HistoryGateway = RealHistoryGateway
) {
    suspend fun load(serverIp: String, port: Int, pairingCode: String): List<HistoryItem> {
        return gateway.getHistory(serverIp, port, pairingCode)
    }

    suspend fun delete(serverIp: String, port: Int, id: String, pairingCode: String): Boolean {
        return gateway.deleteHistory(serverIp, port, id, pairingCode)
    }
}
