# Storage & Maintenance

Storage operations maintain persisted configuration and state without mixing those workflows into individual field editors.

## Verify and reload

**Verify** checks structured config/state consistency and configuration validity. **Reload** asks the running runtime to load the latest persisted configuration.

## Convert

Convert changes the structured config/state representation among supported formats while preserving validated configuration semantics.

## Import and export

Export creates a portable sealed bundle. Import restores a bundle and uses explicit destructive confirmation before applying it. Import supports picker-first file selection with manual path entry fallback.

More detailed bundle behavior is documented under the child topic.
