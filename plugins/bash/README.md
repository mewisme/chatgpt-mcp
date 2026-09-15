# Bash Runtime plugin

The official `bash` plugin provides the `shell/bash` capability on Windows without requiring a system Git for Windows installation.

Version `1.0.0` is built from the pinned Git for Windows PortableGit source declared in `source.json`. The release build verifies the upstream SHA-256, runs PortableGit's upstream post-install preparation in the build environment, and repackages the resulting portable tree into a deterministic ZIP. End-user plugin installation remains declarative extraction only; no installer or post-install script is executed by ChatGPT MCP.

The source `plugin.json` is a release template. Its all-zero platform digest is a required build sentinel and is replaced with the SHA-256 of the generated immutable artifact before the exact-version manifest is published.

Only `windows/amd64` is published initially. Git for Windows also ships an ARM64 package, but the plugin will not advertise that platform until the release pipeline has an ARM64 runtime smoke environment.

The packaged PortableGit tree carries its upstream license and third-party notices. See `licenses/README.md` for provenance.
