# Log Filter Fields

The Logs filter editor has **Range** and **Filters** sections. Applying a filter validates the complete combination before replacing the active log query; invalid combinations keep the draft open.

## Range

### Tail

Non-negative integer controlling how many historical entries are requested. `0` is valid. Non-numeric or negative values are rejected.

### All sessions

When enabled, logs are queried across sessions rather than one specific session. It cannot be combined with a non-empty **Session** field.

### Session

Optional runtime session identifier. Leave empty for the default/current query behavior. It must be empty when **All sessions** is enabled.

### Since

Optional lower time bound. Accepts relative durations such as `30m` or an RFC3339 timestamp, according to the logs query parser.

### Until

Optional upper time bound expressed as RFC3339. The combined Since/Until range is validated by the logs query builder.

## Filters

### Visibility

Controls event visibility detail.

- **Normal** uses default visibility.
- **Verbose** includes verbose events.
- **Debug** includes debug visibility.

### Minimum level

Minimum log level filter: **All**, **Debug**, **Info**, **Warn**, or **Error**. All maps to no minimum-level restriction.

### Components

Comma-separated component filter, for example `SERVER,TOOL`.

### Workspace

Workspace filter accepting a workspace ID or registered path.

### Tool

Optional tool-name filter.

### Status

Optional event/status filter.

### Source

Optional source filter.

### Event glob

Glob pattern matched against event names.

### Grep

Free-text grep filter applied by the log query surface.
