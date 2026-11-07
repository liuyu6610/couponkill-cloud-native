# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

CouponKill is a flash-sale (秒杀) coupon system: five Go services (gateway, user, coupon, order, connector) over PostgreSQL 16, Redis and Kafka, a React 19 frontend, a Helm chart, and an independent kubebuilder Operator module. Docs, comments and commit messages are mostly Chinese; match that. The authoritative design and invariants are in `docs/ARCHITECTURE.md` — change behaviour there first.

## Commands

All Go commands run from the repo root (single module `github.com/liuyu6610/couponkill-cloud-native`, Go 1.27). The Operator under `couponkill-operator/` is a separate module: `cd` into it.

```bash
go build ./cmd/... ./internal/... ./test/...
go vet ./cmd/... ./internal/... ./test/...
go test -count=1 ./cmd/... ./internal/... ./test/...      # needs PostgreSQL, see below
go test -count=1 -run TestName ./internal/order/seckill/  # single test
```

Tests that need a database use `dbxtest` and create/drop a throwaway database per test on the server in `CK_TEST_PG_DSN` (URL form, an account that can `CREATE DATABASE`). Without it they try an embedded PostgreSQL and otherwise skip. **Always set `CK_REQUIRE_PG=true`** so a missing database fails instead of silently skipping:

```bash
export CK_TEST_PG_DSN='postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable' CK_REQUIRE_PG=true
```

End to end (real service processes, real PostgreSQL, in-memory Redis/Kafka from `cmd/devinfra`; builds, boots, tests and cleans up by itself):

```powershell
scripts\e2e-local.ps1                       # all scenarios; -TestPattern X for one; -BootOnly to just check startup
```

`test/e2e` skips unless `CK_E2E_BASE_URL` (the gateway) is set; `CK_REQUIRE_E2E=true` makes that a failure — CI must set it. Other scripts: `scripts/dev-up.ps1` (compose infra + services), `scripts/loadtest.ps1`, `scripts/pg-test.ps1`.

Frontend (`frontend/couponkill-frontend`, Node 20.19+/22.12+): `npm ci`, `npm run dev` (proxies `/api` to the gateway :8088), `npm run lint`, `npm test` (vitest), `npm run build`. Single file: `npx vitest run src/lib/apiClient.test.ts`. `frontend/couponkill-miniprogram` is an unmaintained client of the *old* API contract.

Chart: `helm lint charts/couponkill`; `cd charts/couponkill/tests/render && go test ./...` renders every values combination offline (separate Go module) and cross-checks the injected `CK_*` variables against the Go configs. Images: `build/Dockerfile` with `--build-arg SERVICE=<name>`. `make help` / `build.ps1 help` list the build targets; the kubebuilder targets are `operator-*` and `cd` into `couponkill-operator`.

## Architecture (read `docs/ARCHITECTURE.md` for the full picture)

- `cmd/<svc>/main.go` only wires: config → logx → dependencies → `app.Run`. `cmd/devinfra` is a dev-only miniredis + kfake stand-in.
- `internal/platform/*` is shared infrastructure (config, logx, httpx, authx, dbx, redisx, kafkax, idgen, limiter, app, testkit); API cheat-sheet in `internal/platform/README.md`. `internal/contracts/*` holds what services agree on: Redis key builders (`keys`), Kafka payloads (`events`), internal endpoints + typed clients (`internalapi`). Per-service layering: `handler` → `service` (defines its own dependency interfaces, so rules test without PG/Redis/Kafka) → `repo`.
- Each service owns one PostgreSQL schema (`usr`, `cpn`, `ord`, `cnx`) and runs its own embedded migrations at startup under an advisory lock. Services only touch their own schema; cross-service calls are internal HTTP (`/internal/v1/*`, `X-Internal-Token`).
- Identity: the gateway verifies the JWT (and the per-token session `ck:sess:{jti}`), strips any client-supplied identity headers, and forwards HMAC-signed `X-User-Id/Roles`; services trust only a valid signature (`httpx.RequireGatewayIdentity`). The gateway must forward `Authorization` unchanged — logout needs the jti.
- Errors are real HTTP statuses plus the envelope `{code,message,data,requestId}`; codes live in `internal/platform/apperr` and are shared with the frontend, so never change a code value.
- Seckill (order service): Lua pre-deduct → Kafka `seckill_order_create` → idempotent consumer (coupon `stock/deduct` + one-transaction order insert) → Redis request state polled by the client; a sweeper settles stuck requests. Every step is replayable; `docs/ARCHITECTURE.md` §9 lists the invariants the simulation tests prove.

## Gotchas learned the hard way

- **Windows reserved file names**: never name a file `aux.go`, `con.go`, `nul.go`, `prn.go`, `com1.go`… The Go toolchain happily reads them but Git for Windows cannot index them, so the failure only shows up at commit time.
- **Partitioned `ord.orders`**: PostgreSQL reports unique violations with the *partition's* index name, so never match on constraint names for it. Absorb expected duplicates with `ON CONFLICT (user_id, request_id) DO NOTHING` and treat any remaining unique violation as "the user already holds an active order of this coupon" (`dbx.IsUniqueViolation(err, "")`). Queries that know the user must filter by `user_id` (partition pruning).
- **`pg_locks` / `pg_stat_activity` are server-wide**: tests running in parallel packages share one server, so scope every assertion on them to `current_database()`.
- **Stock sync is min-by-default**: `stock/sync` with no `mode` (or `min`) only ever lowers stock; platform readings can be constant or stale (the mock platform always says 500) and "setting" them resets sold stock → oversell. Only an explicit `mode=set` (admin `force`) may raise it.
- **Gin route mounting**: `order.WithRoutes` hands out a root-level group that already has the gateway-identity middleware, and the reservation/notification packages register their own full path constants (`/api/v1/order/reservations`). Do not mount them under another prefix.
- **Sold-out vs conflict**: coupon `stock/deduct` returns `OutOfStock` (409) only when every bucket is empty; there are no version conflicts. Order must treat transient errors (`internalapi.IsTransient`) as "retry the same requestId", never as a reason to compensate.
- **Line endings**: this Windows checkout may show CRLF in the working tree, so `gofmt -l` lists files that are fine in the index; format-check with `tr -d '\r' < file | gofmt -l`, and write new Go files with LF.
- **Frontend contract**: the frontend reads `createdAt/updatedAt/lastActiveAt` and string ids; `test/e2e` (`TestEndpointsUsedByTheFrontend...`) and `frontend/.../contract.test.ts` pin the shapes from both sides.
- **No `-race` locally** without gcc/cgo: concurrency tests are written to be run with `-count=20`; CI adds `-race`.
- Never commit real credentials. Dev defaults for secrets exist only for `CK_ENV=dev`; `CK_ENV=prod` refuses missing/default/short secrets and demo seeds.

## Not maintained / follow-ups

`couponkill-operator` (its README and samples still describe the old service set), `frontend/couponkill-miniprogram` (old API contract), and `.cursor/`, `.kiro/`, `openspec/` (OpenSpec workflow tooling configuration, no specs). `docs/PRODUCT-POSITIONING.md` predates the Go rewrite.
