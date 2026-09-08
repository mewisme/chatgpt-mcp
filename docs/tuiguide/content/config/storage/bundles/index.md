# Configuration Bundles

Configuration bundles package portable configuration, state, and managed secrets for backup or transfer.

## Export

Export writes a sealed bundle to the selected destination. The destination may be a new path that does not exist yet.

## Import

Import reads a bundle, validates it, and requires explicit confirmation before replacing persisted state. The file picker and manual path entry feed the same validated path value.

After import, runtime state may still differ from persisted state until reload or restart applies the imported configuration.
