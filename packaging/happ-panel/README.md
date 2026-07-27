# 3x-ui Happ multi-node panel

Пакет содержит готовую Linux/amd64-сборку форка 3x-ui, Xray-core, systemd unit,
Docker-конфигурацию и безопасные сценарии обслуживания.

## Состав

- `bin/x-ui` — панель со встроенным production frontend;
- `bin/xray-linux-amd64` — зафиксированный Xray-core;
- `bin/*.dat` — GeoIP и GeoSite выбранного Xray release;
- `scripts/` — install, update, backup, restore и rollback;
- `docs/` — руководство, архитектура, threat model и матрица совместимости;
- `docker-compose.yml`, `Dockerfile`, `.env.example` — контейнерный запуск;
- `SHA256SUMS` — контрольные суммы содержимого пакета.

## Установка через systemd

Требуются Linux amd64, root, systemd и доступный TCP-порт панели.

Быстрая установка напрямую из публичного GitHub-репозитория:

```bash
curl --fail --location --proto '=https' --tlsv1.2 \
  --output install-xray-panel.sh \
  https://raw.githubusercontent.com/Aka-Andy23/3x-ui/main/packaging/happ-panel/install-xray-panel.sh
chmod 700 install-xray-panel.sh
sudo ./install-xray-panel.sh --yes
```

Установщик самостоятельно скачивает архив релиза и отдельный SHA-256-файл,
проверяет контрольную сумму и только после этого запускает установку с backup
и автоматическим rollback.

Для установки из заранее загруженного пакета:

```bash
sha256sum -c SHA256SUMS
sudo ./scripts/install.sh
sudo systemctl status x-ui.service
```

Скрипт проверяет пакет и Xray, создаёт backup существующей установки,
валидирует текущий Xray config и автоматически возвращает предыдущие файлы
при ошибке.

## Docker Compose

```bash
cp .env.example .env
chmod 600 .env
docker compose config
docker compose up -d --build panel
```

Для PostgreSQL задайте стойкий `POSTGRES_PASSWORD`, заполните
`XUI_DB_DSN` и запустите профиль:

```bash
docker compose --profile postgres up -d --build
```

Не публикуйте порт панели без TLS и сетевого ACL. Для production рекомендуется
bind на loopback или внутренний адрес и reverse proxy с HTTPS.

## Обслуживание

```bash
sudo ./scripts/backup.sh
sudo ./scripts/update.sh
sudo ./scripts/restore.sh /var/backups/x-ui/x-ui-backup-TIMESTAMP.tar.gz
sudo ./scripts/rollback.sh /var/backups/x-ui/x-ui-backup-TIMESTAMP.tar.gz
```

Каждый сценарий использует `set -Eeuo pipefail`, пишет live-log и отчёт,
выполняет preflight/selfcheck и показывает пути к log, report и backup.

Перед restore сначала копируйте архив backup и его `.sha256` на отдельный
носитель. PostgreSQL backup/restore включается через `XUI_DB_TYPE=postgres`,
`XUI_DB_DSN` и требует `pg_dump`/`pg_restore`.

## Безопасность

- секреты задаются только через защищённый `.env` или окружение systemd;
- Provider ID Happ не является административным токеном, но не должен
  использоваться как пароль;
- ссылки подписок содержат bearer-like `subscription ID`: передавайте их
  только по HTTPS и перевыпускайте при утечке;
- внешние JSON URL проходят HTTPS-only SSRF-фильтр и сохраняют last-known-good;
- перед применением серверного Xray config выполняется `xray run -test`.

Полные инструкции находятся в `docs/ADMIN_GUIDE.md`.
