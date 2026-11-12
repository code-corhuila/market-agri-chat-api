# market-agri-chat-api

chat-service of Marketplace Agrícola Huila: the **conversations** between a buyer and a producer about
a product, their **messages** in near real time and the agreed **purchase method** (ADR-007: Go;
ADR-010: MongoDB `chat` of `market-agri-infra-mongo`, migrated by `market-agri-chat-db`).

## Layout (Anexo C, Go)

`cmd/chat-api/` (composition root) · `internal/config/` (every limit, with its value) ·
`internal/domain/model/` · `internal/application/{port,usecase}/` · `internal/adapter/in/httpapi/`
(handlers, `errors.go`, `middleware.go`, `auth.go`) · `internal/adapter/out/` (MongoDB, catalog client,
outbox relay). The core imports no framework, driver or HTTP package.

## Configuration

`JWT_ISSUER` and `JWT_PUBLIC_KEY` (PEM on one line with literal `\n`, or `JWT_PUBLIC_KEY_FILE`) are
required; `PORT` defaults to `8080`. Limits (Anexo C 5.3.10) live in `internal/config/config.go`.

## Behaviour common to every route

- `GET /health` → `200 {"status":"UP"}` (liveness, E-60).
- One error body: `{ error, message, details: [], traceId }`, `traceId` = `X-Correlation-Id`; an
  unknown route is `404 NOT_FOUND`, a wrong method `405 METHOD_NOT_ALLOWED`, a panic `500` with a
  neutral message (detail only in the log).
- `X-Correlation-Id` reused when safe (`[A-Za-z0-9._-]{1,128}`), generated otherwise, returned and
  logged on every line (JSON, one object per line).

## Run the tests

```bash
gofmt -l . && go vet ./... && go test -race ./...
docker build -f deploy/Dockerfile .          # also runs the tests
```

## Branching

`develop`, `qa` and `main` accept no direct commit: child branch + squash PR to `develop`,
`qa-<SUBTASK-ID>` with `cherry-pick -x` and *Rebase and merge* to `qa`, `release.2.Y.Z` to `main`
with the teacher's approval (`git-conventions` in `market-agri-docs`).
