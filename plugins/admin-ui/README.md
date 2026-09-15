# Admin UI plugin

Official static Admin UI bundle for ChatGPT MCP.

The plugin provides `web-ui/admin` only. The core process continues to own the admin listener, authentication, API routes, OAuth callback, activity endpoints, and security headers. This plugin contains no backend handlers and requests no runtime permissions.

The release workflow builds `web/`, packages `web/dist` as one deterministic platform-independent ZIP, computes its SHA-256 digest, and publishes the signed manifest and artifact to the official plugin marketplace.
