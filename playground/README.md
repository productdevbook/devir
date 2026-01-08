# Devir Playground

Test ortamı - daemon mode ve servis tiplerini test etmek için.

## Servis Tipleri

Bu playground 4 farklı servis tipini gösterir:

| Tip | Açıklama | Örnek |
|-----|----------|-------|
| `service` (default) | Sürekli çalışan process | `web` - Python HTTP server |
| `oneshot` | Bir kere çalışıp biten | `setup` - Başlangıç script'i |
| `interval` | Belirli aralıklarla çalışan | `health` - Her 5 saniyede health check |
| `http` | HTTP isteği yapan | `api-check` - httpbin.org GET request |

## Status Sembolleri

- `●` Running - Servis aktif çalışıyor
- `✓` Completed - Oneshot başarıyla tamamlandı
- `✗` Failed - Servis hata verdi
- `◐` Waiting - Interval servisi sonraki çalışmayı bekliyor
- `○` Stopped - Servis durmuş

## Kurulum

```bash
# 1. Devir'i build et
cd ..
make build

# 2. MCP config oluştur (Claude Code için)
cd playground
cp .mcp.json.example .mcp.json

# 3. .mcp.json içindeki path'i düzenle
```

## Kullanım

```bash
# TUI başlat
../devir

# MCP modunda bağlan (Claude Code için)
../devir --mcp
```

## devir.yaml Örneği

```yaml
services:
  # Long-running (default)
  web:
    dir: service1
    cmd: python3 -m http.server 8080
    port: 8080
    color: blue

  # Oneshot
  setup:
    type: oneshot
    dir: .
    cmd: echo "Setup complete!"
    color: yellow

  # Interval
  health:
    type: interval
    interval: 5s
    dir: .
    cmd: curl -sf http://localhost:8080 && echo "OK"
    color: green

  # HTTP
  api-check:
    type: http
    url: https://httpbin.org/get
    method: GET
    color: magenta
```

## Test Senaryoları

### Senaryo 1: Farklı Servis Tipleri
1. `../devir` başlat
2. `setup` oneshot'ın `✓` ile tamamlandığını gör
3. `web` servisinin `●` ile çalıştığını gör
4. `health` servisinin `◐` ile beklediğini, her 5 saniyede çalıştığını gör
5. `api-check` HTTP servisinin `✓` ile tamamlandığını gör

### Senaryo 2: TUI → TUI
1. Terminal 1: `../devir`
2. Terminal 2: `../devir`
3. İki terminal de aynı logları ve statusları görmeli

### Senaryo 3: MCP Status
1. Claude Code'da `devir_status` çağır
2. Tüm servislerin type, status, runCount bilgilerini gör
