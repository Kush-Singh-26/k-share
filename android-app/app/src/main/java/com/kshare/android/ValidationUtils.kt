package com.kshare.android

object ValidationUtils {
    private val IP_REGEX = Regex("^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$")

    fun isValidIp(ip: String): Boolean {
        if (ip.isBlank()) return false
        if (ip == "localhost" || ip == "127.0.0.1") return true
        return IP_REGEX.matches(ip)
    }

    fun isValidPort(port: String): Boolean {
        val portNum = port.toIntOrNull() ?: return false
        return portNum in 1..65535
    }

    fun sanitizePort(port: String): String {
        val portNum = port.toIntOrNull() ?: return Constants.DEFAULT_SERVER_PORT_STRING
        return if (portNum in 1..65535) portNum.toString() else Constants.DEFAULT_SERVER_PORT_STRING
    }
}
