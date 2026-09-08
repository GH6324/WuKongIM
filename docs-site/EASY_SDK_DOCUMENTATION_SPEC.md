# WuKongEasySDK documentation specification

## Reader and outcome

An application developer familiar with their platform but new to WuKongIM can
prepare credentials, install one exact SDK version, connect two users, observe
both message directions, and release the connection. Explain Channel, Payload,
and send results before relying on them. The application supplies a trusted
backend and UI; EasySDK handles online messaging, not offline synchronization.

## Published structure

Keep the existing `/sdk/easy` routes and Chinese/English parity. The overview
helps choose a platform. `examples` prepares a development single-node cluster,
credentials with matching device categories, and eight runnable entry points.
Each platform quickstart follows prepare, install, connect/listen, exchange,
cleanup, and troubleshooting, in that order.

Use one default installation path. Put alternative installations after these
tasks. Preserve each platform's real singleton, callback, timeout, and cleanup
semantics. Runnable official examples may complement application integration
snippets; identify application-supplied functions and where snippets belong.
Show expected output and wait for both peers before sending. Bound any display
buffers; they are not persistent history or durable deduplication.

## Tutorial versions

| Platform | Version | Default distribution |
| --- | --- | --- |
| iOS | 1.1.1 | Swift Package Manager |
| Android | 1.0.5 | Maven Central |
| Flutter | 1.1.0 | pub.dev |
| Web | 2.0.5 | npm |
| Rust | 0.1.0 | crates.io |
| C# | 1.0.0 | NuGet |
| C++ | 0.1.0 | WuKongIM vcpkg Git registry |
| Python | 0.1.0 | PyPI |

Check examples against their released package or pinned source. Source examples
are not package-consumer verification. Web 2.0.5 includes the handshake-failure
reconnect repair; keep its independent package validation separate from
historical Web 2.0.4 four-platform results.

## Validation reference

The original client, server, harness, artifact checksums, environment, failed
observations and bounded outcomes are preserved in
`docs/superpowers/reports/2026-09-08-easysdk-validation-history.md`, with links to
immutable originals. Link that engineering reference from public pages rather
than embedding CI commands or retrospective reports in the first-message path.
Retain actionable version requirements and known recovery limitations in the
platform tutorial, including C# cluster recovery and Python membership fixes.
Installation pins needed by vcpkg or optional source builds remain explicit.

## Checks

`lib/easy-sdk-tutorial-contract.test.ts` checks paired learning order, platform
APIs, installs, cleanup, bounded examples, and provenance separation.
`lib/easy-sdk-cpp-contract.test.ts` checks JSON manifests, CMake wiring,
archive prerequisites, and callback constraints. Run the normal documentation
verification gate for navigation, links, lint, MDX/types, static export and
machine-readable outputs. Record compilation separately from a live message run.
