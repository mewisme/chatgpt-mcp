# RTK Command Wrapper

The official `rtk` plugin integrates RTK Token Killer as a host-backed command wrapper. It can use a compatible RTK already present on `PATH`, or explicitly install a manifest-pinned portable RTK binary into plugin-owned local data without republishing RTK in the ChatGPT MCP registry.

All RTK-specific integration details live in `plugin.json`. The manifest declares `rtk`/`rtk.exe`, checks `rtk --version`, `rtk gain`, and a `rtk rewrite "git status"` probe, then declares the runtime rewrite command and accepted exit codes. ChatGPT MCP core only provides the generic host-plugin harness.

If RTK is missing or a declared check fails, plugin installation stops before state is written and prints the install recommendations from the manifest. These commands follow the RTK upstream documentation:

```text
brew install rtk-ai/tap/rtk
curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/master/install.sh | sh
cargo install --git https://github.com/rtk-ai/rtk --branch master rtk
```

On native Windows the manifest recommends `winget install rtk-ai.rtk` first, with the documented explicit-Git Cargo command as an alternative. Portable-local installation is pinned to upstream RTK `v0.49.0`; every supported portable asset has its exact SHA-256 embedded in the signed plugin manifest, so the portable payload selected by `rtk@1.0.0` cannot drift when upstream publishes a newer release. A separately managed global RTK installation remains under the operator's package-manager lifecycle.

At runtime the generic host-wrapper harness invokes the command template declared by the manifest. RTK maps this to `rtk rewrite <command>`. Unsupported commands pass through unchanged. Rewritten commands retain the original command as the security projection, so core guard, workspace, and approval policy are evaluated against the unwrapped operation.
