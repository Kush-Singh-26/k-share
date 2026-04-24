package com.kshare.android.api

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ApiResponseParserTest {
    @Test
    fun parsesJsonPingResponse() {
        val result = ApiResponseParser.parsePingResponse(
            """{"status":"ok","name":"K-Share","role":"guest"}""",
            certHash = "hash"
        )

        assertTrue(result.success)
        assertEquals("K-Share", result.name)
        assertEquals("guest", result.role)
        assertEquals("hash", result.certHash)
    }

    @Test
    fun fallsBackToTextPingResponse() {
        val result = ApiResponseParser.parsePingResponse("ok")

        assertTrue(result.success)
        assertEquals(null, result.name)
        assertEquals(null, result.role)
    }

    @Test
    fun parsesFilesHistoryAndHttpErrors() {
        val files = ApiResponseParser.parseFiles(
            """[{"name":"a.txt","isDirectory":false,"size":10,"modTime":"today"}]"""
        )
        val history = ApiResponseParser.parseHistory(
            """[{"id":"1","text":"hello","timestamp":"now"}]"""
        )

        assertEquals(1, files.size)
        assertEquals("a.txt", files[0].name)
        assertEquals(1, history.size)
        assertEquals("hello", history[0].text)
        assertEquals("HTTP 401", ApiResponseParser.httpErrorMessage(401))
    }
}
