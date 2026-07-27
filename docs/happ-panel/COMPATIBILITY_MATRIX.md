# Матрица совместимости

| Компонент | Версия | Статус проверки |
|---|---:|---|
| 3x-ui baseline | v3.5.0 / `4e928a1c` | исходные backend/frontend tests и build |
| Xray-core | v26.7.11 / `50231eaff98c` | официальный SHA-256, real `run -test` |
| Go | 1.26.5 | build, test, vet, race |
| Node.js | engine 22; build env 24.14.0 | lint, typecheck, tests, Vite build |
| React | 19.2.7 | component tests/build |
| TypeScript | 6.0.3 | `tsc --noEmit` |
| Ant Design | 6.5.0 | component/build |
| Vite | 8.1.4 | production build |
| SQLite | GORM driver baseline | clean/repeat/legacy migration tests |
| PostgreSQL | GORM driver baseline | compile/SQL-shape tests; live server unavailable |
| Happ Android | current public contract на 2026-07-14 | headers/schema checked; physical import unavailable |
| Happ iOS | current public contract на 2026-07-14 | headers/schema checked; physical import unavailable |
| Happ Desktop | current public contract на 2026-07-14 | headers/schema checked; physical import unavailable |

## Xray features

Real Xray v26.7.11 validation выполнена для VLESS, REALITY, XHTTP, TLS, ECH,
WebSocket, gRPC, TCP, VLESS encryption, observatory, leastPing balancer и
fallback. WebSocket и gRPC приняты Xray, но сам Xray сообщает об их
deprecated transport status; существующая совместимость 3x-ui сохранена.

## Happ contract

Реализация следует опубликованным App Management headers Happ:
`providerid`, `subscription-autoconnect`,
`subscription-autoconnect-type=lowestdelay` и
`subscription-ping-onopen-enabled`.

Источники:

- https://www.happ.su/main/ru/dev-docs/app-management
- https://xtls.github.io/en/config/routing.html
- https://xtls.github.io/en/config/transport.html

Для выпуска рекомендуется smoke-test на реальном устройстве каждой платформы,
так как app store release может измениться независимо от панели.
