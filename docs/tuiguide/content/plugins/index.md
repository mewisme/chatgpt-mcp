# Plugins

The Plugins area manages installed plugins, marketplace discovery, available updates, and trusted registries. Plugins extend local runtime behavior; they do not change core workspace or control-plane policy.

## Installed plugins

The main Plugins tab lists installed plugins. Open a row with `Enter` to view identity, version, enablement, and retained rollback versions. From detail:

- `space` enables or disables the plugin;
- `u` updates to the latest available version;
- `b` rolls back to a retained version;
- `p` prunes retained inactive versions;
- `v` re-verifies integrity and trust;
- `d` uninstalls; `D` force-uninstalls when needed.

`r` refreshes the current list or detail.

## Marketplace

Press `2` (or open **Plugin Marketplace** from Commands) to browse registry catalogs. Filter with `/`, open a row for metadata and trust details, then `i` to install. Marketplace rows are keyed by registry-qualified references such as `official/bash`.

## Updates

Press `3` for **Plugin Updates**, which lists outdated installed plugins. Open a row and update or refresh from the same detail bindings used on Installed.

## Registries

Press `4` for **Plugin Registries** to see the built-in official registry and any configured third-party registries. Press `a` to add a registry with HTTPS URL and pinned Sigstore identity (`issuer` + `repository`). Custom registries cannot replace `official`. Remove a third-party registry with `d` from its detail page.

Unqualified name resolution is only used for registries explicitly configured to allow it.

## Commands and CLI

`Ctrl+K` surfaces plugin actions when the current Plugins route has a matching resource. Canonical CLI equivalents include `cgm plugin list`, `cgm plugin search`, `cgm plugin install`, `cgm plugin update`, `cgm plugin verify`, and `cgm plugin registry add|list|remove`.
