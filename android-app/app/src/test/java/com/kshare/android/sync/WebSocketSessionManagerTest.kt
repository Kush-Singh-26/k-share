package com.kshare.android.sync

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import okhttp3.Request
import okhttp3.Response
import okhttp3.Protocol
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class WebSocketSessionManagerTest {
    @Test
    fun connectBuildsSocketAndForwardsMessages() = runBlocking {
        val engine = FakeWebSocketEngine()
        val scope = CoroutineScope(Dispatchers.Unconfined + SupervisorJob())
        val events = mutableListOf<String>()
        val manager = WebSocketSessionManager(engine, scope, reconnectDelayMs = 1)

        manager.connect("192.168.1.10", 26260, listener = object : WebSocketSessionManager.Listener {
            override fun onOpen() {
                events += "open"
            }

            override fun onMessage(text: String) {
                events += "message:$text"
            }
        })

        engine.listener?.onOpen(engine.socket, FakeResponse().toResponse())
        engine.listener?.onMessage(engine.socket, """{"type":"clip"}""")

        assertEquals("192.168.1.10", engine.request?.url?.host)
        assertEquals(26260, engine.request?.url?.port)
        assertEquals("/ws", engine.request?.url?.encodedPath)
        assertEquals(listOf("open", "message:{\"type\":\"clip\"}"), events)
        scope.cancel()
    }

    @Test
    fun reconnectsAfterUnexpectedClose() = runBlocking {
        val engine = FakeWebSocketEngine()
        val scope = CoroutineScope(Dispatchers.Unconfined + SupervisorJob())
        val manager = WebSocketSessionManager(engine, scope, reconnectDelayMs = 1)
        var reconnected = false

        manager.connect("192.168.1.10", 26260, listener = object : WebSocketSessionManager.Listener {}, onReconnect = {
            reconnected = true
            manager.connect("192.168.1.10", 26260, listener = object : WebSocketSessionManager.Listener {})
        })
        engine.listener?.onClosed(engine.socket, 1006, "boom")
        delay(10)

        assertTrue("Should have called onReconnect", reconnected)
        assertTrue("Connect count should be at least 2, was ${engine.connectCount}", engine.connectCount >= 2)
        scope.cancel()
    }

    private class FakeWebSocketEngine : WebSocketEngine {
        var request: Request? = null
        var listener: WebSocketListener? = null
        var connectCount = 0
        val socket = object : WebSocket {
            override fun queueSize(): Long = 0
            override fun close(code: Int, reason: String?): Boolean = true
            override fun request(): Request = request!!
            override fun send(text: String): Boolean = true
            override fun cancel() {}
            override fun send(bytes: ByteString): Boolean = true
        }

        override fun newWebSocket(request: Request, listener: WebSocketListener): WebSocket {
            this.request = request
            this.listener = listener
            connectCount++
            return socket
        }
    }

    private class FakeResponse {
        fun toResponse(): Response {
            return Response.Builder()
                .request(Request.Builder().url("https://example.com").build())
                .protocol(Protocol.HTTP_1_1)
                .code(101)
                .message("Switching Protocols")
                .build()
        }
    }
}
