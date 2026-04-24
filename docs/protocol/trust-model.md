# Trust Model

K-Share uses trust-on-first-use for server certificates.

## Hashing

- The certificate fingerprint is derived from the server leaf certificate.
- The fingerprint is stored as a SHA-256 hex string.

## First use

1. The client discovers or connects to a server.
2. The client reads the server certificate hash.
3. If the hash is unknown, the client prompts the user to trust it.
4. If the user accepts, the hash is stored in the local trust store.

## Trusted use

- If a stored certificate hash matches, the client treats the server as trusted.
- Trusted certificates allow normal file, clipboard, and history operations.

## Rotation and mismatch

- If the server certificate changes, the stored hash no longer matches.
- The client must treat that as a trust event, not as a silent reconnect.
- A new certificate may be accepted only after user confirmation.

## Trust store locations

- Desktop trust metadata is stored in the desktop config file.
- Android trust metadata is stored in the app settings store.

## Related rules

- Pairing codes and certificate trust are separate concerns.
- A valid pairing code does not replace certificate trust.
- A trusted certificate does not bypass role checks on the server.

