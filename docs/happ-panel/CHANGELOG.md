# Changelog

## 3.5.0-happ.1

- добавлена персональная Happ JSON subscription с preview, QR/download,
  ETag/HEAD, статусом и принудительной регенерацией;
- добавлены manual и HTTPS remote JSON sources с CodeMirror,
  last-known-good и SSRF-защитой;
- реализован allowlist merge pipeline со стабильными tags, deduplication,
  source metadata и частичным продолжением;
- добавлены global/client direct domains, IDN normalization, import/export,
  routing preview и loop protection;
- добавлены Xray observatory/leastPing auto-selection и подтверждённые Happ
  provider headers;
- расширен существующий multi-node client workflow и cluster IP reset через
  `runtime.Runtime`;
- расширен штатный IP-limit: first/last seen, node/inbound и event journal;
- добавлен четырёхшаговый client wizard и portable bulk import preview;
- добавлены SQLite/PostgreSQL-compatible idempotent migrations;
- добавлены Xray pre-apply validation, previous binary/config и rollback;
- Xray updater и release workflows проверяют официальный SHA-256;
- добавлены unit/service/API/migration/security/fuzz/race/component/i18n tests;
- добавлены systemd/Docker packaging, install/update/backup/restore/rollback
  scripts и эксплуатационная документация.

Совместимость существующих raw, JSON, Clash и других subscription endpoints
сохранена. Публичные API расширены без удаления прежних полей.
