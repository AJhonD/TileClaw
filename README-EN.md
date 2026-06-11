# TileClaw - Map Tiles Downloader

> A fast map tile downloader supporting Gaode, Google, Tianditu, Mapbox, OSM, and more.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Features

- **Multi-source** — Gaode, Google, Tianditu, Mapbox, OSM, Baidu and custom tile services
- **Precise download** — GeoJSON boundary filtering, download only specified areas
- **Multi-layer concurrency** — Download multiple zoom levels simultaneously
- **Resumable** — Auto-skip already downloaded tiles after unexpected exit
- **Dual output** — MBTiles database or directory-based file storage
- **Vector tiles** — PBF format support
- **Cross-platform** — Pure Go, CGO-free, one-command cross-compilation
- **Auto-retry** — Exponential backoff retry for network resilience

## Quick Start

### Build

```bash
task package:linux    # Linux amd64
task package:windows  # Windows amd64
task package:mac-arm  # macOS arm64

# Or build directly
go build -trimpath -ldflags="-s -w" -o tileclaw .
```

### Configuration

Edit `conf/conf.toml`:

```toml
[task]
    workers = 2
    timedelay = 100

[output]
    format = "mbtiles"
    directory = "output"

[tm]
    name = "Gaode Satellite"
    min = 0
    max = 15
    format = "jpg"
    schema = "xyz"
    url = "http://webst01.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}"

[[lrs]]
    min = 0
    max = 15
    geojson = "./geojson/china.geojson"
```

### Run

```bash
./tileclaw -c conf/conf.toml

# Background
nohup ./tileclaw -c conf/conf.toml > /dev/null 2>&1 &
```

## CLI Options

| Option | Description |
|--------|-------------|
| `-c` | Config file path, default `conf/conf.toml` |
| `-test-mail` | Send a test email to verify mail notification config |
| `-convert-shp` | Convert a .shp/.zip file to GeoJSON and exit |
| `-convert-out` | Output path for the converted GeoJSON file |
| `--force-polygon` | Force PolyLine to Polygon conversion (auto-closes open rings) |

### SHP to GeoJSON Conversion

Supports `.shp` and `.zip` (containing .shp) input. Automatically detects geometry types and outputs GeoJSON.

```bash
# Basic conversion
./tileclaw -convert-shp boundary.zip -convert-out ./geojson/boundary.geojson

# Force line-to-polygon conversion (useful for boundary line data)
./tileclaw -convert-shp boundary.zip -convert-out ./geojson/boundary.geojson --force-polygon
```

The tool prints geometry type summary during conversion, e.g. `Shapefile geometry: PolyLine x10`. Closed rings are automatically detected and converted to Polygon. Use `--force-polygon` to force polygon output for open lines by auto-closing them.

### Multi-URL Download

Each `[[lrs]]` block can specify its own `url`. If omitted, the global `[tm].url` is used. Different zoom ranges or regions can fetch from different tile sources.

```toml
# Global default URL
[tm]
    url = "http://webrd0{1-4}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}"

# Low zoom levels: global boundary, default URL
[[lrs]]
    min = 0
    max = 6
    geojson = "./geojson/global.geojson"

# High zoom levels: China region, different tile source
[[lrs]]
    min = 7
    max = 16
    geojson = "./geojson/china.geojson"
    url = "http://mt0.google.com/vt/lyrs=y&x={x}&y={y}&z={z}"

# Same zoom level, different regions with different URLs
[[lrs]]
    min = 7
    max = 16
    geojson = "./geojson/china_ten_dash_line.geojson"
    url = "http://webst0{1-4}.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}"
```

> URL supports `{1-4}` numeric range and `{a-c}` letter range for subdomain rotation, e.g. `webst0{1-4}` randomly picks from `webst01`/`webst02`/`webst03`/`webst04`.

## Boundary Data Sources

Download areas are controlled by the GeoJSON boundary files configured in `conf/conf.toml`. You can obtain administrative boundary data from:

- [shengshixian.com](https://www.shengshixian.com/)
- [Ruiduobao Map Data](https://map.ruiduobao.com/)

Province-level or national boundary data is usually enough for tile downloading. County, township, and village-level datasets can be very large and are usually unnecessary for nationwide downloads. For public map products, use properly licensed and compliant boundary data.

## Tile URL Reference

### Gaode (Amap)

Tile format: `jpg`/`png`, CRS: GCJ-02, no API key required.

| Layer | Format | URL |
|-------|--------|-----|
| Satellite | `jpg` | `http://webst01.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}` |
| Streets | `png` | `http://webrd01.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}` |

> **Notes**
> - `style=6`: satellite imagery without labels
> - `style=8`: standard street map with place/road names and POIs; opaque white background, **cannot overlay on satellite**
> - Gaode has no transparent label layer; use Tianditu for satellite + label overlay
> - Replace `webst01` with `webst02`~`webst04` to distribute requests
> - Replace `webrd01` with `webrd02`~`webrd04` to distribute requests

### Google Maps

Tile format: `jpg`/`png`, CRS: WGS-84 by default, GCJ-02 with `gl=CN`.

| Layer | Format | URL |
|-------|--------|-----|
| Satellite | `jpg` | `http://mt0.google.com/vt/lyrs=s&x={x}&y={y}&z={z}` |
| Satellite + Labels | `jpg` | `http://mt0.google.com/vt/lyrs=y&x={x}&y={y}&z={z}` |
| Transparent Labels | `png` | `http://mt0.google.com/vt/lyrs=h&x={x}&y={y}&z={z}` |
| Streets | `png` | `http://mt0.google.com/vt/lyrs=m&x={x}&y={y}&z={z}` |
| Terrain | `jpg` | `http://mt0.google.com/vt/lyrs=t&x={x}&y={y}&z={z}` |
| Streets + Labels | `png` | `http://mt0.google.com/vt/lyrs=r&x={x}&y={y}&z={z}` |

> **Notes**
> - Requires VPN/proxy to access from China
> - Add `&gl=CN` for GCJ-02 offset when downloading China region
> - Replace `mt0` with `mt1`~`mt3` to distribute requests
> - `lyrs=h` is a transparent label layer, overlay on `lyrs=s` satellite

### Tianditu

Tile format: `jpg`/`png`, CRS: GCJ-02, **requires API key** ([register here](https://console.tianditu.gov.cn/)).

| Layer | Format | URL |
|-------|--------|-----|
| Satellite `img_w` | `jpg` | `https://t0.tianditu.gov.cn/DataServer?T=img_w&x={x}&y={y}&l={z}&tk=your_key` |
| Satellite Labels `cia_w` | `png` | `https://t0.tianditu.gov.cn/DataServer?T=cia_w&x={x}&y={y}&l={z}&tk=your_key` |
| Vector Base `vec_w` | `png` | `https://t0.tianditu.gov.cn/DataServer?T=vec_w&x={x}&y={y}&l={z}&tk=your_key` |
| Vector Labels `cva_w` | `png` | `https://t0.tianditu.gov.cn/DataServer?T=cva_w&x={x}&y={y}&l={z}&tk=your_key` |

> **Notes**
> - `cia_w` and `cva_w` are transparent label layers, overlay on `img_w`/`vec_w`
> - Tianditu has 429 rate limiting; TileClaw has built-in retry + interval control
> - Replace `t0` with `t1`~`t6` to distribute requests
> - Use responsibly — avoid excessive high-frequency downloads

### Mapbox

Tile format: `jpg`/`png`/`pbf`, CRS: WGS-84, **requires access token** ([register here](https://account.mapbox.com/)).

| Layer | Format | URL |
|-------|--------|-----|
| Satellite | `jpg` | `https://api.mapbox.com/v4/mapbox.satellite/{z}/{x}/{y}.jpg?access_token=your_token` |
| Streets | `png` | `https://api.mapbox.com/v4/mapbox.streets/{z}/{x}/{y}.png?access_token=your_token` |
| Light | `png` | `https://api.mapbox.com/v4/mapbox.light/{z}/{x}/{y}.png?access_token=your_token` |
| Dark | `png` | `https://api.mapbox.com/v4/mapbox.dark/{z}/{x}/{y}.png?access_token=your_token` |
| Terrain | `png` | `https://api.mapbox.com/v4/mapbox.mapbox-terrain-v2/{z}/{x}/{y}.png?access_token=your_token` |

> **Notes**
> - Free tier: 50,000 tile requests/month; watch quota for large downloads
> - Vector tiles (PBF) use a different API endpoint, not covered here
> - Set `schema` to `xyz`; Mapbox uses standard XYZ numbering

### OpenStreetMap

Tile format: `png`, CRS: WGS-84, no API key required.

| Layer | Format | URL |
|-------|--------|-----|
| Standard | `png` | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` |
| Humanitarian | `png` | `https://a.tile.openstreetmap.fr/hot/{z}/{x}/{y}.png` |

> **Notes**
> - OSM has a strict [tile usage policy](https://operations.osmfoundation.org/policies/tiles/); bulk downloading is prohibited
> - Recommended for small areas only, or use a third-party mirror
> - Single server, no CDN — not suitable for large-scale downloads

### Baidu Maps

Tile format: `jpg`/`png`, CRS: BD-09, no API key required.

| Layer | Format | URL |
|-------|--------|-----|
| Satellite | `jpg` | `http://shangetu0.map.bdimg.com/it/u=x={x};y={y};z={z};v=009;type=sate&fm=46` |
| Streets | `png` | `http://online1.map.bdimg.com/onlinelabel/?qt=tile&x={x}&y={y}&z={z}&styles=pl` |

> **Notes**
> - **Baidu uses BD-09 CRS and a custom tile numbering scheme**, incompatible with standard XYZ/TMS
> - The URL templates above are for reference only; direct use may result in misaligned tiles
> - Coordinate conversion (BD-09 → WGS-84) is needed for proper integration
> - Baidu tile row numbering starts from top-left, unlike TMS (bottom-left)

### Coordinate Reference Systems

| CRS | Used By | Description |
|-----|---------|-------------|
| WGS-84 | Google (non-offset), OSM, Mapbox | International standard |
| GCJ-02 (Mars) | Gaode, Tianditu, Google (`gl=CN`) | Chinese national encryption offset |
| BD-09 | Baidu | Baidu's additional encryption on top of GCJ-02 |

> Tiles from different CRS cannot be directly overlaid. Ensure alignment before stacking layers.

## Serving with Tileserver-GL

Place the generated `.mbtiles` files in tileserver-gl's `data/` directory.

## Credits

This project is based on [atlasdatatech/tiler](https://github.com/atlasdatatech/tiler.git). Huge thanks to the original author for open-sourcing this great tool.

Key improvements over the original:
- Pure Go SQLite driver (`modernc.org/sqlite`) for CGO-free cross-compilation
- HTTP connection pooling with automatic retry
- Multi-layer concurrent downloading
- Resumable downloads with deterministic output paths
- Structured logging with panic recovery
- Pre-built cross-platform packaging via Taskfile

## Disclaimer

This project is intended only for learning, research, and personal technical verification. Do not use it in ways that violate map service terms, data licenses, laws, regulations, or map review requirements. Users are responsible for verifying the authorization and compliance of tile services, boundary datasets, administrative division data, and generated outputs. Any risk or liability arising from use of this project is borne by the user.

## License

MIT
