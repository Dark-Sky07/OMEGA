# OMEGA

## Exact upstream baseline

- Upstream: https://github.com/MHSanaei/3x-ui
- Tag: `v3.3.1`
- Commit: `b5ef412b8db999a6b981e1550762551d7b06540f`
- Unmodified import checkpoint: `270fdd1` (before any OMEGA changes).
- The upstream license and attribution are retained. Go module identifiers, database paths, service names, update scripts and protocol behavior are deliberately not globally renamed.

## Agreed reseller specification

Resellers manage only their own clients on administrator-approved inbounds. They cannot create inbounds or access server settings, backups, nodes, other resellers, other customers, administrator API tokens or the global WebSocket feed.

Limits are allocated bytes (not actual consumption), number of clients, and account expiry. Zero is **not unlimited** for reseller-created client quotas. Expiry timestamps use Unix milliseconds; volume uses bytes (1 GiB = 1,073,741,824 bytes). Each client's full quota counts even if disabled or depleted. Client expiry must not exceed reseller expiry. Client renewal must not reset consumed traffic. Deleting a customer releases its allocation only after successful deletion.

## Implementation status — checkpoint, not a deployable reseller feature

Implemented in this checkpoint:

- Exact upstream source imported and pushed separately.
- OMEGA visible branding in the main sidebar and login page.
- Separate additive reseller, allowed-inbound and client-ownership models.
- Pure, fail-closed validation of reseller quotas, expiry and inbound permissions, with boundary tests.

Still required before enabling reseller accounts:

- Transactional quota reservation and recovery after failed Xray/node operations; concurrent requests must not overspend.
- Account creation, password hashing, isolated authentication and session revocation.
- Backend authorization on every entry point, including legacy API and WebSocket access.
- Ownership-scoped client CRUD, renewal, traffic and subscription links.
- Administrator UI and reseller portal, with Persian translations.
- Migration, authorization, concurrency and end-to-end tests against a disposable server.
- OMEGA release/install/update channel (the inherited scripts currently install/update upstream, **not OMEGA**).

Do not install this checkpoint on a production server expecting working reseller functionality. No reseller login or management endpoints are exposed yet.

## Preservation and verification

Work is pushed only to `arena/01a09d0e-omega` in `Dark-Sky07/OMEGA`. Build products, downloaded SDKs, dependencies and credentials are not committed. Source backups are not a substitute for backups of an installed server's database and certificates.

Verification commands:

```sh
go test ./internal/reseller ./internal/database
cd frontend
npm ci
npm run typecheck
npm run build
```

The upstream module requires Go `1.26.4`; do not downgrade the upstream dependency versions to accommodate a build environment.

### Local checkpoint results

- `npm run typecheck`: passed.
- `npm run build`: passed.
- `npm test`: 509 tests passed in 31 files; the suite emitted DOM/CodeMirror environment warnings.
- Go tests: not run locally. Go is absent; attempts to obtain the exact SDK from go.dev, dl.google.com, storage.googleapis.com and proxy.golang.org failed with SSL connection errors. The separate OMEGA GitHub Actions workflow runs on the session branch; its result must be checked independently.

### GitHub validation

[OMEGA validation run 34789427947](https://github.com/Dark-Sky07/OMEGA/actions/runs/34789427947) passed for checkpoint `30beac1`:

- `go test -race ./internal/reseller`: passed on GitHub's runner using the upstream Go version.
- `go test ./internal/database`: passed, including the additive migration test.
- Frontend typecheck, tests and production build: passed.

These are foundation checks, not an end-to-end validation of a working reseller feature.
