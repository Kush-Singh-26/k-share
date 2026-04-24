package com.kshare.android.sync

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ClipboardSyncManagerTest {
    @Test
    fun textPushIsDedupedAfterSuccessfulSend() = runBlocking {
        val gateway = FakeClipboardGateway()
        val manager = ClipboardSyncManager(gateway)

        assertTrue(manager.pushText("192.168.1.10", 26260, "hello", "1234"))
        assertFalse(manager.pushText("192.168.1.10", 26260, "hello", "1234"))
    }

    @Test
    fun imagePushIsDedupedAfterSuccessfulSend() = runBlocking {
        val gateway = FakeClipboardGateway()
        val manager = ClipboardSyncManager(gateway)

        val bytes = byteArrayOf(1, 2, 3)
        assertTrue(manager.pushImage("192.168.1.10", 26260, bytes, "1234"))
        assertFalse(manager.pushImage("192.168.1.10", 26260, bytes, "1234"))
    }

    private class FakeClipboardGateway : ClipboardGateway {
        override suspend fun getText(serverIp: String, port: Int, pairingCode: String, channel: String): String? = "text"
        override suspend fun postText(
            serverIp: String,
            port: Int,
            text: String,
            pairingCode: String,
            append: Boolean,
            channel: String
        ): Boolean = true

        override suspend fun getImage(serverIp: String, port: Int, pairingCode: String): ByteArray? = byteArrayOf(1, 2, 3)
        override suspend fun postImage(serverIp: String, port: Int, bytes: ByteArray, pairingCode: String): Boolean = true
    }
}
