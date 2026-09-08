This directory stages Prometheus binaries before building cmd/wukongim.
The Dockerfile downloads the official Linux amd64 or arm64 release selected by
docker/prometheus/toolchain.env and verifies its pinned SHA256 before staging
only the target platform. The final image also carries the upstream license
and notice. Local development scripts may build assets from source instead.

Generated files are named prometheus-<goos>-<goarch> and are embedded into the
final wukongim binary through go:embed. They are intentionally ignored by git.
Docker also ignores developer-generated assets, so a local file cannot replace
the verified build input. Enabling prometheus with an empty binary_path extracts
the matching asset under the configured Prometheus data directory and starts it
as an app-managed child process.
