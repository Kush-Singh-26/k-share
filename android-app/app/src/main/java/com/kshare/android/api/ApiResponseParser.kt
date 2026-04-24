package com.kshare.android.api

import com.google.gson.Gson
import com.google.gson.reflect.TypeToken

object ApiResponseParser {
    private val gson = Gson()

    fun parsePingResponse(body: String, certHash: String? = null): ApiClient.PingResult {
        val type = object : TypeToken<Map<String, Any?>>() {}.type
        val json = runCatching {
            gson.fromJson<Map<String, Any?>>(body, type)
        }.getOrNull()

        if (json != null && json.isNotEmpty()) {
            val success = json["status"]?.toString() == "ok"
            val name = json["name"]?.toString()?.takeIf { it.isNotBlank() }
            val role = json["role"]?.toString()?.takeIf { it.isNotBlank() }
            return ApiClient.PingResult(success, certHash, name, role)
        }

        return ApiClient.PingResult(body.contains("ok", ignoreCase = true), certHash)
    }

    fun parseFiles(body: String): List<RemoteFile> {
        val type = object : TypeToken<List<RemoteFile>>() {}.type
        return runCatching { gson.fromJson<List<RemoteFile>>(body, type) }
            .getOrNull() ?: emptyList()
    }

    fun parseHistory(body: String): List<HistoryItem> {
        val type = object : TypeToken<List<HistoryItem>>() {}.type
        return runCatching { gson.fromJson<List<HistoryItem>>(body, type) }
            .getOrNull() ?: emptyList()
    }

    fun httpErrorMessage(code: Int): String = "HTTP $code"
}
