# Configuration

Config is a typed configuration center built from the application's configuration schema. It groups fields into operational domains instead of presenting one giant flat form.

## Domains

The dashboard groups settings into areas such as Runtime & Network, Access & Security, Shell & Execution, Features, Tunnel, and Storage & Maintenance. Opening a domain shows only fields/actions relevant to that section.

Field detail shows the schema description, current/default state, and guidance. Managed secret/hash fields are read-only and never render the underlying secret value.

## Editing typed fields

Editable fields open a routed full-page editor. String/integer input guidance is placed in placeholders. Persistent booleans use Switch controls. Select-like schema fields use choices. Validation uses the same configuration mutation/domain logic used by normal configuration operations.

`Ctrl+S` persists the field. On success, the editor commits the saved draft as its baseline before returning to the field page, so the global dirty guard does not incorrectly ask to discard a value that was just saved. On failure the editor remains open with the draft intact.

## Shell & Execution

Shell policy includes approval behavior and command policy fields such as allow/deny command patterns. Multiline command lists are edited as editor content rather than a cramped modal. Saving these fields follows the same clean-baseline behavior described above.

Command policy is only one layer of shell safety; workspace access boundaries and the runtime control-guard/approval model remain separate concerns.

## Storage & Maintenance

Storage centralizes maintenance operations:

- **Verify** checks structured config/state consistency and validity.
- **Reload** asks a running runtime to load persisted configuration.
- **Migrate** moves legacy plaintext credentials to managed secret storage.
- **Convert** changes structured config/state format among supported formats.
- **Export** creates a portable sealed configuration bundle.
- **Import** restores a bundle and uses an explicit destructive confirmation before applying it.

Import uses a file picker with manual input fallback. Export accepts a destination that may not exist yet.

## Persisted vs runtime state

When a runtime is running, Config shows whether persisted configuration matches the runtime fingerprint. A successful local mutation can make runtime sync **pending** until reload/restart applies the new persisted state. This distinction is intentional: saving a field does not pretend that an already-running process has automatically adopted every setting.