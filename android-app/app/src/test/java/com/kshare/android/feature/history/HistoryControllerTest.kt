package com.kshare.android.feature.history

import com.kshare.android.api.HistoryItem
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class HistoryControllerTest {
    @Test
    fun delegatesLoadAndDeleteToGateway() = runBlocking {
        val gateway = FakeHistoryGateway(
            items = listOf(HistoryItem("1", "hello", "now"))
        )
        val controller = HistoryController(gateway)

        val items = controller.load("192.168.1.10", 26260, "1234")
        val deleted = controller.delete("192.168.1.10", 26260, "1", "1234")

        assertEquals(1, items.size)
        assertTrue(deleted)
        assertEquals(listOf("load:192.168.1.10:26260:1234", "delete:192.168.1.10:26260:1:1234"), gateway.calls)
    }

    private class FakeHistoryGateway(
        private val items: List<HistoryItem>
    ) : HistoryGateway {
        val calls = mutableListOf<String>()

        override suspend fun getHistory(serverIp: String, port: Int, pairingCode: String): List<HistoryItem> {
            calls += "load:$serverIp:$port:$pairingCode"
            return items
        }

        override suspend fun deleteHistory(serverIp: String, port: Int, id: String, pairingCode: String): Boolean {
            calls += "delete:$serverIp:$port:$id:$pairingCode"
            return true
        }
    }
}
