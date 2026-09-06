# wkmigrate

Offline migration from unmodified WuKongIM `v2.2.5-20260422`
(`a888f89533d0e7d1b2030e06504ca97f1ad891d4`) into a fresh native v3 cluster.
The source server does not need an upgrade. Linux and macOS source locking is supported.

```sh
GOWORK=off go build -o ./bin/wkmigrate ./cmd/wkmigrate
./bin/wkmigrate --help
```

The phases are `prepare`, `export`, `import`, and `verify`. Every phase takes an
immutable `--plan` and a `--workspace`; the last three require `--archive`.
Stop all source writes and processes before preparation. Import all target
nodes before startup; run independent offline verification before first startup.
Use a v3 binary built from the same implementation as the migration tool.

Read the [operator runbook](../../docs/superpowers/runbooks/v2-to-v3-migration.md)
for the plan schema, commands, restart boundaries, supported mappings and cutover.
The [local acceptance report](../../docs/superpowers/reports/2026-09-06-v2-to-v3-migration.md)
records functional coverage and the deferred 100 GiB / four-hour performance target.
