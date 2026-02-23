# Security Policy

## Reporting a vulnerability

If you discover a security vulnerability in OpenConductor, please report it
responsibly. **Do not open a public issue.**

Email: **security@openconductor.dev**

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We will acknowledge your report within 48 hours and aim to release a fix
within 7 days for critical issues.

## Scope

OpenConductor manages AI coding agent processes and terminal I/O. Security-relevant
areas include:

- **PTY management** -- ensuring agent processes are properly sandboxed
- **Configuration files** -- API keys are referenced by environment variable name,
  never stored directly in config
- **Telegram integration** -- bot token handling and message routing
- **LLM API calls** -- terminal content sent to L2 classifiers

## Supported versions

We apply security fixes to the latest release only. There is no long-term
support for older versions at this time.
