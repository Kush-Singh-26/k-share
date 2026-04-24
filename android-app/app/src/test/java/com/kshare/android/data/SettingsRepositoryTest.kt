package com.kshare.android.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SettingsRepositoryTest {
    @Test
    fun defaultsAreReturnedWhenStoreIsEmpty() {
        val repo = SettingsRepository(InMemorySettingsStore())

        assertEquals("", repo.serverIp)
        assertEquals("", repo.serverPort)
        assertEquals("", repo.pairingCode)
        assertEquals("system", repo.darkMode)
        assertEquals("", repo.downloadUri)
        assertEquals(emptyMap<String, ServerInfo>(), repo.getKnownServers())
        assertEquals(emptyMap<String, String>(), repo.getAllSavedIps())
        assertNull(repo.getLastServerIp("192.168.1"))
    }

    @Test
    fun persistsKnownServersAndNetworkCache() {
        val store = InMemorySettingsStore()
        val repo = SettingsRepository(store)

        repo.serverIp = "192.168.1.10"
        repo.serverPort = "26260"
        repo.pairingCode = "1234"
        repo.darkMode = "dark"
        repo.downloadUri = "content://downloads/tree"

        repo.saveServer("hash1", "192.168.1.10", 26260, "1234")
        repo.setLastServerIp("192.168.1", "192.168.1.10")

        assertEquals("192.168.1.10", repo.serverIp)
        assertEquals("26260", repo.serverPort)
        assertEquals("1234", repo.pairingCode)
        assertEquals("dark", repo.darkMode)
        assertEquals("content://downloads/tree", repo.downloadUri)

        val known = repo.getKnownServers()
        assertEquals(1, known.size)
        assertEquals("192.168.1.10", known["hash1"]?.ip)
        assertEquals(26260, known["hash1"]?.port)
        assertEquals("1234", known["hash1"]?.authCode)
        assertEquals("192.168.1.10", repo.getLastServerIp("192.168.1"))
        assertEquals(mapOf("192.168.1" to "192.168.1.10"), repo.getAllSavedIps())

        repo.removeServer("hash1")
        repo.removeServerIp("192.168.1")

        assertEquals(emptyMap<String, ServerInfo>(), repo.getKnownServers())
        assertEquals(emptyMap<String, String>(), repo.getAllSavedIps())
    }

    @Test
    fun malformedJsonFallsBackToEmptyState() {
        val store = InMemorySettingsStore()
        store.putString("known_servers", "{not-json")
        store.putString("network_ip_cache", "{broken")

        val repo = SettingsRepository(store)

        assertEquals(emptyMap<String, ServerInfo>(), repo.getKnownServers())
        assertEquals(emptyMap<String, String>(), repo.getAllSavedIps())
    }
}
