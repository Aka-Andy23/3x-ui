# Xray-core source and integrity

- Release: `v26.7.11`
- Commit shown by binary: `50231eaff98c`
- Go runtime shown by binary: `go1.26.5 linux/amd64`
- Official asset:
  `https://github.com/XTLS/Xray-core/releases/download/v26.7.11/Xray-linux-64.zip`
- Official digest:
  `https://github.com/XTLS/Xray-core/releases/download/v26.7.11/Xray-linux-64.zip.dgst`
- ZIP SHA-256:
  `aa11c3685c71da0ffc71e511db50404609e7e963bb914b048f59a6a00af8930e`
- Extracted binary SHA-256:
  `5200ed9b358cf380b2d9f1fe28c7e56220c0159adcd86a64592246d8257a043c`

При обновлении скачивайте ZIP и `.dgst` только из одного release, проверяйте
SHA-256 до extraction, затем выполняйте:

```bash
xray version
xray run -test -c /etc/x-ui/config.json
```

Updater панели сохраняет предыдущий binary/config и возвращает их при ошибке
валидации или restart.
