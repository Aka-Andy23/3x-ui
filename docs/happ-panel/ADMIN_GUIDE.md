# Руководство администратора

## Первичная настройка

1. Установите пакет и откройте панель только через HTTPS.
2. Смените административные учётные данные и ограничьте доступ к панели ACL.
3. В настройках подписок включите Happ JSON, задайте публичный HTTPS URI.
4. Для автоматического выбора укажите Provider ID Happ: ровно 8 латинских
   букв или цифр.
5. Проверьте версии панели и Xray на странице состояния.

JSON-ссылка клиента имеет вид `<публичный URI>/<subscription ID>`. Она
поддерживает `GET`, `HEAD`, ETag/304, `Last-Modified`, private cache и
Happ-заголовки. Считайте `subscription ID` секретной ссылкой.

## Создание клиента

Wizard состоит из четырёх шагов:

1. данные клиента, срок, трафик, статус и число одновременных IP;
2. выбор нескольких inbound с группировкой по узлу и протоколу;
3. Happ, внешние JSON, прямые домены и автоматический выбор;
4. итоговая проверка перед сохранением.

Поля bundle предварительно валидируются. Локальная запись, профиль, прямые
домены и внешние источники создаются в транзакции. Привязки на удалённых
узлах выполняются через существующий `runtime.Runtime` с компенсирующим
rollback. При сетевом разрыве операция возвращает ошибку и не удаляет
ранее существовавшие данные.

Повторный выбор того же клиента и inbound не создаёт дубликат. Массовый
импорт поддерживает preview, построчную валидацию, пропуск дублей и скачивание
отчёта.

## Внешние конфигурации

В карточке клиента доступны:

- ручной Xray JSON с CodeMirror, форматированием, строкой/позицией ошибки,
  приоритетом и порядком;
- удалённый JSON по HTTPS с timeout, лимитом размера, redirect limit,
  ручным refresh и статусом последней попытки.

Импортируется только allowlist клиентских секций. `api`, серверные `inbounds`,
произвольный `log` и неизвестные верхнеуровневые секции не переносятся.
Ошибка источника не ломает локальную подписку: используется атомарно
сохранённый last-known-good, а предупреждение попадает в preview.

Remote fetch запрещает localhost, loopback, private, link-local, multicast,
CGNAT, documentation/special ranges и metadata endpoints. DNS и каждый
redirect проверяются повторно; прокси окружения отключены; TLS не ниже 1.2.

## Прямые домены

Страница «Прямые домены» принимает домены, URL, пробелы, запятые и переносы.
Схема, порт, путь, query и fragment удаляются; Unicode IDN хранится в Punycode.
Доступны комментарии, поиск, включение, bulk import, export и routing preview.

Глобальные include применяются ко всем JSON. Клиентские exclude отменяют
совпадающие глобальные правила, а клиентские include добавляют правила.
Маршруты инфраструктуры панели, узлов, DNS и VPN endpoints вставляются раньше
пользовательских правил для защиты от routing loop.

## Автовыбор

Кнопка создаёт Xray client JSON с `observatory`,
`routing.balancers[].strategy.type=leastPing` и явным `fallbackTag`, который
никогда не равен `direct`. Участвуют только выбранные, включённые,
непросроченные подключения доступных узлов.

Измерение выполняется клиентским Xray, а не панелью. Нулевые, отрицательные,
timeout и отсутствующие результаты не считаются рабочими средствами Xray
observatory. Если кандидатов нет, генератор возвращает «Нет доступных
подключений».

Happ получает подтверждённые headers:

- `subscription-autoconnect: 1`;
- `subscription-autoconnect-type: lowestdelay`;
- `subscription-ping-onopen-enabled: 1`;
- `providerid: <8 символов>`.

Поля switch threshold/debounce отсутствуют в подтверждённом публичном
контракте Happ/Xray и показываются как неподдерживаемые, без имитации.

## IP limit

Поле «Одновременных IP»: `0` отключает лимит, положительное значение задаёт
максимум активных уникальных IP. Используется штатный IP-limit/Fail2ban 3x-ui.
Карточка показывает IP, first/last seen, узел, inbound и события ограничения.
Сброс распространяется по узлам через `runtime.Runtime`.

При reverse proxy/CDN проверьте настройку trusted proxy и источник реального
адреса до включения лимита. NAT закономерно объединяет нескольких
пользователей в один внешний IP. Правила Fail2ban не должны охватывать SSH,
порт панели, localhost и межузловые адреса.

## Узлы

Управление использует штатные модели Node, NodeService и `runtime.Runtime`.
Проверка состояния показывает panel/Xray version, latency, CPU, RAM, disk,
network, last sync/error. TLS pinning и mTLS настраиваются в существующей
карточке узла. Недоступность узла не очищает его клиентов или inbound.

## Backup и обновление

```bash
sudo /usr/local/x-ui/scripts/backup.sh
sudo /usr/local/x-ui/scripts/update.sh
sudo /usr/local/x-ui/scripts/restore.sh /path/to/backup.tar.gz
sudo /usr/local/x-ui/scripts/rollback.sh /path/to/backup.tar.gz
```

Перед обновлением сохраните backup вне сервера. Для SQLite сервис
кратковременно останавливается для согласованного snapshot. Для PostgreSQL
используются `pg_dump --format=custom` и `pg_restore`.

## Диагностика

- Проверка Xray: `/usr/local/x-ui/bin/xray-linux-amd64 run -test -c /etc/x-ui/config.json`.
- Состояние: `systemctl status x-ui.service`.
- Журнал: `journalctl -u x-ui.service`.
- Maintenance logs: `/var/log/x-ui-maintenance`.

Не помещайте полные subscription URLs, node credentials и admin tokens в
тикеты или диагностические архивы.
