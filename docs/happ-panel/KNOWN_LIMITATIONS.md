# Известные ограничения

1. В среде сборки отсутствовали Docker/Podman, PostgreSQL server, удалённые
   3x-ui узлы и реальные устройства Happ. Dockerfile/Compose, PostgreSQL
   paths и multi-node contracts проверены статически/автотестами, но live
   end-to-end требует инфраструктурного стенда.
2. Межузловая операция не является общей ACID-транзакцией. Используются
   idempotent calls и компенсирующий rollback; после обрыва сети оператору
   показывается ошибка и безопасный retry.
3. Happ/Xray не публикуют подтверждённые параметры hysteresis threshold или
   debounce для `lowestdelay`. Панель не имитирует их поддержку.
4. Xray принимает WebSocket/gRPC, но v26.7.11 предупреждает, что эти transports
   deprecated. Они сохранены ради совместимости существующих пользователей.
5. IP limit считает наблюдаемые внешние адреса. NAT объединяет пользователей,
   а неверный trusted proxy/CDN может исказить источник.
6. Public subscription URL является bearer-like secret. ETag/private cache
   не защищают украденную ссылку; требуется ротация subscription ID.
7. После совместимого обновления `immutable`, Swagger, `postcss` и корневого
   `brace-expansion`, `npm audit --audit-level=high` сообщает 5 high
   advisories (2 production): вложенный `brace-expansion` dev-плагина и
   React Router RSC. Совместимого non-breaking fix audit не предлагает.
   Панель не использует RSC actions; риск всё равно не считается закрытым.
8. External JSON поддерживает allowlist известных client outbounds. Неизвестные
   новые протоколы/поля пропускаются с ошибкой или warning до явного добавления.
9. Backup PostgreSQL требует локально доступных `pg_dump`/`pg_restore` той же
   major-версии, что и server.
