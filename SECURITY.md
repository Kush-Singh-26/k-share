# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------|
| 1.x     | :white_check_mark: |

Older versions are not supported. Please upgrade to the latest stable release.

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly.

### Do Not Report via Public Issues

**DO NOT** create public GitHub issues for security vulnerabilities. Public issues notify attackers and put users at risk.

### How to Report

1. **Email**: Send a direct message to the maintainer via GitHub
2. **GitHub Security Advisories**: Use [GitHub's private vulnerability reporting](https://github.com/Kush-Singh-26/k-share/security/advisories/new)

Include in your report:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fixes (optional)

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 7 days
- **Fix timeline**: Depends on severity and complexity

### Scope

The following are in scope:

- Server authentication bypasses
- Certificate trust circumvention
- Data exposure between users
- Unauthenticated file access
- Clipboard data leakage
- Denial of service

### Out of Scope

- Social engineering attacks
- Physical access attacks
- User device compromise
- Network-level attacks (DNS, ARP, etc.)

## Security Model

K-Share uses a trust-on-first-use (TOFU) model for server certificates. Users must verify they are connecting to the correct server on first connection.

See [Trust Model](./docs/protocol/trust-model.md) for details.

### Recommendations for Production

- Run K-Share on trusted networks only
- Verify the server certificate on first connection
- Use strong pairing codes
- Review the trust store periodically

## Disclosure

We follow coordinated disclosure. Credit will be given in the security advisory once the fix is released.

## Thanks

Thank you for helping keep K-Share and its users safe.