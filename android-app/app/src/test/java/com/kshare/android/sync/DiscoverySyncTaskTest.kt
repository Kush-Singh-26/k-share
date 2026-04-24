package com.kshare.android.sync

import com.kshare.android.api.ApiClient
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Test

class DiscoverySyncTaskTest {
    @Test
    fun refreshesCachedIpWhenLanIsAvailableAndPingSucceeds() = runBlocking {
        val settings = FakeSettings()
        val environment = FakeEnvironment(
            lanAvailable = true,
            networkId = "192.168.1",
            pingResult = ApiClient.PingResult(success = true, certHash = "hash", role = "admin")
        )
        val task = DiscoverySyncTask(settings, environment)

        val result = task.run()

        assertEquals(DiscoverySyncResult.Success, result)
        assertEquals("192.168.1.10", settings.serverIp)
    }

    @Test
    fun skipsWhenLanIsMissing() = runBlocking {
        val settings = FakeSettings()
        val environment = FakeEnvironment(lanAvailable = false)
        val task = DiscoverySyncTask(settings, environment)

        val result = task.run()

        assertEquals(DiscoverySyncResult.Success, result)
        assertEquals("", settings.serverIp)
    }

    private data class FakeSettings(
        override var serverIp: String = "",
        override var serverPort: String = "26260",
        override val pairingCode: String = "1234",
        private val cached: Map<String, String> = mapOf("192.168.1" to "192.168.1.10")
    ) : DiscoverySyncSettings {
        override fun getLastServerIp(networkId: String): String? = cached[networkId]
    }

    private class FakeEnvironment(
        private val lanAvailable: Boolean,
        private val networkId: String? = null,
        private val pingResult: ApiClient.PingResult = ApiClient.PingResult(success = false, certHash = null, role = null)
    ) : DiscoverySyncEnvironment {
        override fun hasValidLan(): Boolean = lanAvailable
        override fun getNetworkId(): String? = networkId
        override suspend fun ping(serverIp: String, port: Int, pairingCode: String): ApiClient.PingResult = pingResult
    }
}
