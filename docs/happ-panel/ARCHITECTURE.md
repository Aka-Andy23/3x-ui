# Архитектура расширения

## Baseline и принципы

Основа: 3x-ui `v3.5.0`, commit
`4e928a1ce0945a6e956aa63365034ec24d2b1387`. Лицензия GPL-3.0 и attribution
upstream сохранены. Изменения расширяют существующие owner-слои и публичные
API обратно совместимо.

## Владельцы

| Область | Существующий owner | Расширение |
|---|---|---|
| Клиенты/inbound | `ClientService`, inbound models | bundle create, multi-select, cleanup |
| Узлы | `NodeService`, `runtime.Runtime` | выбор/фильтры, cluster IP reset |
| Подписки | `internal/sub` | Happ generator и единый merge |
| External links | `ClientExternalLink` | manual/remote JSON и last-known-good |
| Xray apply | `ServerService`, `internal/xray` | validate, backup, rollback |
| IP limit | существующий IP-limit/Fail2ban | first seen, node/inbound, events |
| UI | существующий React SPA/Ant Design | wizard, editor, direct domains |

Контроллеры выполняют bind/auth/response. Валидация, fetch, merge, транзакции
и генерация находятся в service/util слоях.

## Модель данных

Расширена `client_external_links`; добавлены:

- `client_subscription_profiles` — настройки/статус Happ и auto-selection;
- `direct_domains` — глобальные и клиентские include/exclude;
- `first_seen` в штатных IP observations.

GORM AutoMigrate используется одинаково для SQLite/PostgreSQL. Миграции
добавляют поля/таблицы без удаления данных и повторно исполняются безопасно.

## Pipeline подписки

1. `internal/sub` разрешает клиента по `subscription ID`.
2. Через штатный subscription/runtime path собираются локальные и удалённые
   inbound выбранного клиента.
3. Загружаются enabled manual JSON и last-known-good remote JSON.
4. Parser применяет allowlist, лимиты размера/глубины/узлов.
5. Merge дедуплицирует identity, создаёт стабильные tags и metadata источника.
6. Добавляются direct/infrastructure routing rules.
7. При необходимости строятся observatory и leastPing balancer.
8. JSON валидируется, хэшируется и отдаётся с ETag/Happ headers.

Отказ одного внешнего источника даёт warning и не удаляет рабочие локальные
элементы.

## Транзакции и узлы

Локальные данные создаются транзакционно. Удалённые Xray-узлы не поддерживают
общую ACID-транзакцию, поэтому операции выполняются через `runtime.Runtime`
с idempotency/deduplication и компенсирующим rollback. Сетевая неопределённость
сохраняется как ошибка для безопасного retry; данные недоступного узла не
обнуляются.

## Xray lifecycle

Перед hot apply выполняется `xray run -test -c`. Рабочий config сохраняется
как previous, binary/config копируются в backup. При неуспешном restart
восстанавливается предыдущая пара. Updater скачивает официальный release,
сверяет SHA-256 из `.dgst`, валидирует candidate version и текущий config.

## Frontend

SPA использует React, TypeScript, Ant Design, TanStack Query и CodeMirror.
Все новые пользовательские строки находятся в i18n; русский и английский
полные, остальные локали получают английский fallback с проверкой паритета.
`internal/web/dist` формируется только штатным frontend build.
