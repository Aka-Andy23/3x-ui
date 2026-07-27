# Threat model

## Активы и границы доверия

Критичны admin session, node credentials, subscription IDs, UUID клиентов,
Xray configs, база и backup. Недоверенные входы: browser/API payload,
external JSON/URL/DNS/redirect, client names, bulk import и remote node reply.

| Угроза | Контроль |
|---|---|
| IDOR между клиентами | admin API resolve по record/email; public endpoint только по уникальному sub ID |
| Утечка подписки | HTTPS URI, private cache, отсутствие admin/node secrets в JSON и логах |
| Подбор ссылок | криптографический sub ID штатного 3x-ui, rate limit 120/min/IP |
| SSRF/DNS rebinding | HTTPS-only, запрет special IP, all-answer validation, pinned dial, redirect recheck |
| Злонамеренный JSON | 16 MiB, depth/node/string limits, allowlist секций/protocols, server-section strip |
| XSS | React escaping, JSON preview/code editor, нет HTML injection из external source |
| SQL injection | GORM parameters, enum/range validation, без SQL-конкатенации из payload |
| Command injection | Xray вызывается аргументами без shell; URL/JSON не исполняются |
| Path traversal | фиксированные storage paths; restore tar entries проверяются до extraction |
| CSRF/replay | существующая authenticated panel middleware; state API не используется public endpoint |
| Race/lost update | DB transaction, atomic last-known-good, conditional remote cache, race tests |
| Невалидный Xray | обязательный `run -test`, previous config/binary и rollback |
| Privilege escalation | systemd hardening, Docker `no-new-privileges`, drop capabilities |
| Secret logging | sanitized remote errors; UI/toast/metadata не получают credentials |
| Resource exhaustion | body/parser/fetch limits, timeout, bounded request limiter/cache updates |

## SSRF policy

Запрещены localhost, loopback, RFC1918, link-local, multicast, CGNAT,
documentation/special-use blocks, IPv4-mapped IPv6 и известные metadata IP.
Проверяются все DNS answers, выбранный dial address и каждый redirect.
HTTP proxy окружения не используется. Ответ принимается только как JSON с
допустимым Content-Type и ограниченным размером.

Это не заменяет egress firewall. В production рекомендуется разрешить панели
исходящий TCP/443 только к необходимым сетям через отдельный proxy/ACL.

## Остаточные риски

- distributed update нескольких узлов не может быть атомарным без общего
  coordinator; применяется compensating rollback;
- публичная subscription URL остаётся bearer-like секретом до ротации;
- allowlist не доказывает семантическую безопасность каждого будущего поля
  Xray; новые протоколы должны добавляться с тестами;
- CDN/reverse proxy может скрыть IP подписчика, если trusted proxy настроен
  неверно;
- npm audit baseline содержит upstream high advisories, перечисленные в
  `KNOWN_LIMITATIONS.md`;
- egress DNS/HTTPS оператор всё ещё может видеть destination metadata.

## Операционные требования

Панель должна быть за HTTPS, MFA/SSO perimeter при наличии, сетевым ACL и
регулярным backup. Не включайте node credentials, admin tokens или полные
subscription URLs в отчёты. После утечки ротируйте admin session, node secret
или client sub ID в зависимости от затронутого актива.
