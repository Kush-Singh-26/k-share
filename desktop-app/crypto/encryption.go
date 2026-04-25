package crypto

// DEPRECATED: This file previously contained EncryptData, DecryptData, EncryptStream,
// and DecryptStream functions. They were orphaned code — never called by the server,
// desktop app, or Android client. The current protocol uses TLS + certificate pinning
// for transport security, not application-layer encryption.
//
// If end-to-end encryption is needed in the future, implement it as a proper
// protocol extension with versioning, not as standalone unused utilities.
