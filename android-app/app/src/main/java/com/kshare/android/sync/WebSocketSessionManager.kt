package com.kshare.android.sync

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.atomic.AtomicLong

interface WebSocketEngine {
    fun newWebSocket(request: Request, listener: WebSocketListener): WebSocket
}

class OkHttpWebSocketEngine(private val client: OkHttpClient) : WebSocketEngine {
    override fun newWebSocket(request: Request, listener: WebSocketListener): WebSocket {
        return client.newWebSocket(request, listener)
    }
}

class WebSocketSessionManager(
    private val engine: WebSocketEngine,
    private val scope: CoroutineScope,
    private val reconnectDelayMs: Long = 5_000L
) {
    interface Listener {
        fun onOpen() {}
        fun onMessage(text: String) {}
        fun onClosed(code: Int, reason: String) {}
        fun onFailure(t: Throwable) {}
    }

    private var currentDelay = 2000L
    private val maxDelay = 64000L

    private data class Endpoint(
        val serverIp: String,
        val port: Int,
        val path: String,
        val authCode: String
    )

    private val generationCounter = AtomicLong(0L)
    @Volatile private var currentGeneration = 0L
    @Volatile private var suppressedGeneration: Long? = null
    private var reconnectJob: Job? = null
    private var endpoint: Endpoint? = null
    private var listener: Listener? = null
    private var webSocket: WebSocket? = null
    private var onReconnect: (suspend () -> Unit)? = null

    fun connect(serverIp: String, port: Int, path: String = "/ws", authCode: String, listener: Listener, onReconnect: (suspend () -> Unit)? = null) {
        this.endpoint = Endpoint(serverIp, port, path, authCode)
        this.listener = listener
        this.onReconnect = onReconnect
        reconnectJob?.cancel()
        reconnectJob = null

        val previousSocket = webSocket
        if (previousSocket != null) {
            suppressedGeneration = currentGeneration
            previousSocket.close(1000, null)
        }
        webSocket = null
        openCurrent()
    }

    fun close() {
        reconnectJob?.cancel()
        reconnectJob = null
        endpoint = null
        listener = null
        if (webSocket != null) {
            suppressedGeneration = currentGeneration
            webSocket?.close(1000, null)
        }
        webSocket = null
    }

    private fun openCurrent() {
        val ep = endpoint ?: return
        val targetListener = listener ?: return
        val generation = generationCounter.incrementAndGet()
        currentGeneration = generation
        val request = Request.Builder()
            .url("wss://${ep.serverIp}:${ep.port}${ep.path}")
            .header("Authorization", "Bearer ${ep.authCode}")
            .build()

        webSocket = engine.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                currentDelay = 2000L // Reset backoff on success
                targetListener.onOpen()
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                targetListener.onMessage(text)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                this@WebSocketSessionManager.webSocket = null
                if (consumeSuppressedDisconnect(generation)) return
                targetListener.onClosed(code, reason)
                scheduleReconnect()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                this@WebSocketSessionManager.webSocket = null
                if (consumeSuppressedDisconnect(generation)) return
                targetListener.onFailure(t)
                scheduleReconnect()
            }
        })
    }

    private fun scheduleReconnect() {
        val reconnect = onReconnect ?: return
        reconnectJob?.cancel()
        reconnectJob = scope.launch {
            delay(currentDelay)
            currentDelay = (currentDelay * 2).coerceAtMost(maxDelay)
            reconnect()
        }
    }

    private fun consumeSuppressedDisconnect(generation: Long): Boolean {
        if (suppressedGeneration == generation) {
            suppressedGeneration = null
            return true
        }
        return false
    }
}
