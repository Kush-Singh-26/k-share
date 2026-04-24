package com.kshare.android.connection

import com.kshare.android.api.ApiClient
import com.kshare.android.data.ServerInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ConnectionCoordinatorTest {
    @Test
    fun trustedServerSetsCachedStateAndReturnsConnected() {
        val settings = FakeConnectionSettings().apply {
            pairingCode = "1234"
            saveServer("hash1", "192.168.1.10", 26260, "1234")
        }
        val coordinator = ConnectionCoordinator(
            settings,
            pingGateway = FakePingGateway(ApiClient.PingResult(true, "hash1", role = "guest")),
            discoveryGateway = FakeDiscoveryGateway(networkId = "192.168.1")
        )

        val outcome = runBlockingTest {
            coordinator.verifyConnection("192.168.1.10", 26260, silent = false)
        }

        assertTrue(outcome is ConnectionOutcome.Connected)
        assertEquals("192.168.1.10", settings.serverIp)
        assertEquals("192.168.1.10", settings.getLastServerIp("192.168.1"))
    }

    @Test
    fun unknownServerRequestsTrustWhenNotSilent() {
        val settings = FakeConnectionSettings().apply {
            pairingCode = "1234"
        }
        val coordinator = ConnectionCoordinator(
            settings,
            pingGateway = FakePingGateway(ApiClient.PingResult(true, "hash-new", role = null)),
            discoveryGateway = FakeDiscoveryGateway()
        )

        val outcome = runBlockingTest {
            coordinator.verifyConnection("192.168.1.11", 26260, silent = false)
        }

        assertTrue(outcome is ConnectionOutcome.TrustRequired)
    }

    @Test
    fun discoveryFallsBackWhenCachedIpFails() {
        val settings = FakeConnectionSettings().apply {
            pairingCode = "1234"
            setLastServerIp("192.168.1", "192.168.1.20")
            saveServer("hash2", "192.168.1.21", 26260, "1234")
        }
        val discovery = FakeDiscoveryGateway(
            hasLan = true,
            networkId = "192.168.1",
            foundIp = "192.168.1.21"
        )
        val pingGateway = FakePingGateway(
            ApiClient.PingResult(false, "hash2", role = null),
            ApiClient.PingResult(true, "hash2", role = null)
        )
        val coordinator = ConnectionCoordinator(
            settings,
            pingGateway = pingGateway,
            discoveryGateway = discovery
        )

        val outcome = runBlockingTest {
            coordinator.discoverConnection(26260, silent = false) {}
        }

        assertTrue(outcome is ConnectionOutcome.Connected)
        assertEquals(2, pingGateway.requests.size)
    }

    private class FakePingGateway(
        private val results: List<ApiClient.PingResult>
    ) : PingGateway {
        private var index = 0
        val requests = mutableListOf<Pair<String, Int>>()

        constructor(vararg results: ApiClient.PingResult) : this(results.toList())

        override suspend fun ping(serverIp: String, port: Int, pairingCode: String): ApiClient.PingResult {
            requests += serverIp to port
            return results.getOrElse(index++) { results.last() }
        }
    }

    private class FakeDiscoveryGateway(
        private val hasLan: Boolean = true,
        private val networkId: String? = null,
        private val foundIp: String? = null
    ) : DiscoveryGateway {
        var pingRequests = 0

        override fun hasValidLan(): Boolean = hasLan
        override fun getNetworkId(): String? = networkId
        override suspend fun findServer(port: Int, pairingCode: String, onStatus: (String) -> Unit): String? {
            onStatus("searching")
            return foundIp
        }
    }

    private class FakeConnectionSettings : ConnectionSettings {
        override var pairingCode: String = ""
        override var serverIp: String = ""
        override var serverPort: String = "26260"
        private val knownServers = mutableMapOf<String, ServerInfo>()
        private val cache = mutableMapOf<String, String>()

        override fun getKnownServers(): Map<String, ServerInfo> = knownServers.toMap()

        override fun saveServer(certHash: String, ip: String, port: Int, authCode: String) {
            knownServers[certHash] = ServerInfo(ip, port, authCode)
        }

        override fun getLastServerIp(networkId: String): String? = cache[networkId]

        override fun setLastServerIp(networkId: String, ip: String) {
            cache[networkId] = ip
        }
    }

    private fun <T> runBlockingTest(block: suspend () -> T): T {
        return kotlinx.coroutines.runBlocking { block() }
    }

}
