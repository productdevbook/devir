# Devir Playground

Test ortamı - daemon mode'u test etmek için.

## Kurulum

```bash
# 1. Devir'i build et
cd ..
make build

# 2. MCP config oluştur (Claude Code için)
cd playground
cp .mcp.json.example .mcp.json

# 3. .mcp.json içindeki path'i düzenle:
#    "<ABSOLUTE_PATH_TO_PLAYGROUND>" → "$(pwd)"
#    Örn: /Users/username/projects/devir/playground
```

## Kullanım

```bash
# TUI başlat (ilk terminal)
../devir

# İkinci terminalde aynı daemon'a bağlan
../devir

# MCP modunda bağlan (Claude Code için)
../devir --mcp
```

## Test Senaryoları

### Senaryo 1: TUI → TUI
1. Terminal 1: `../devir`
2. Terminal 2: `../devir`
3. İki terminal de aynı logları görmeli

### Senaryo 2: TUI → MCP
1. Terminal 1: `../devir`
2. Terminal 2: `../devir --mcp` (veya Claude Code üzerinden)
3. MCP'den restart yapınca TUI'da görünmeli

### Senaryo 3: MCP → TUI
1. Claude Code: `devir_start` çağır
2. Terminal: `../devir`
3. Çalışan servisleri ve logları görmeli
