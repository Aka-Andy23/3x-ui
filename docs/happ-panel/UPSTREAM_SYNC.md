# Синхронизация с upstream

Fork основан на tag `v3.5.0`, commit
`4e928a1ce0945a6e956aa63365034ec24d2b1387`.

## Порядок обновления

```bash
git remote add upstream https://github.com/MHSanaei/3x-ui.git
git fetch upstream --tags
git switch feature/happ-multinode-panel
git switch -c sync/upstream-vNEXT
git rebase upstream/vNEXT
```

Разрешайте конфликты по owner-слоям:

1. models/AutoMigrate;
2. `ClientService` и `runtime.Runtime`;
3. `internal/sub`;
4. Xray process/update;
5. OpenAPI generation;
6. React/i18n;
7. packaging/workflows.

Не переносите вручную `internal/web/dist`: после merge выполните штатные
frontend generate/build. Не удаляйте новые upstream subscription formats,
node fields или IP-limit branches. Сначала сопоставьте owner и расширьте
общий pipeline.

## Обязательная проверка после rebase

```bash
go test ./... -count=1
go vet ./...
go test -race ./internal/sub ./internal/web/service ./internal/util/... -count=1
cd frontend
npm ci
npm run gen
npm run lint
npm exec tsc -- --noEmit
npm test -- --run
npm run build
```

Затем проверьте:

- повторную и legacy migration SQLite/PostgreSQL;
- real Xray `run -test` для transport matrix и leastPing;
- GET/HEAD/ETag/Happ headers;
- SSRF special ranges/DNS rebinding/redirect;
- недоступный узел и compensating rollback;
- IP-limit reset/rename/delete;
- locale parity и OpenAPI operation count.

Новый Xray release обновляйте только после проверки официального `.dgst`,
compatibility matrix и rollback. Не используйте prerelease без отдельного
обоснования.
