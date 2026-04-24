package com.kshare.android.feature.main

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MainStateReducerTest {
    @Test
    fun initialStateUsesSavedSettingsAndDefaults() {
        val settings = object : MainSettingsSource {
            override val serverIp = "192.168.1.20"
            override val serverPort = ""
            override val darkMode = "dark"
        }

        val state = MainStateReducer.initialState(settings)

        assertEquals("192.168.1.20", state.serverIp)
        assertEquals("26260", state.serverPort)
        assertEquals("dark", state.themeMode)
    }

    @Test
    fun reducerCopiesStateFields() {
        val initial = MainUiState()
        val withIp = MainStateReducer.withServerIp(initial, "10.0.0.5")
        val withPort = MainStateReducer.withServerPort(withIp, "26261")
        val refreshing = MainStateReducer.setRefreshing(withPort, true)

        assertEquals("10.0.0.5", refreshing.serverIp)
        assertEquals("26261", refreshing.serverPort)
        assertTrue(refreshing.isRefreshing)
        assertFalse(refreshing.showTrustDialog)
    }
}
