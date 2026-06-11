# TileClaw - 地图瓦片下载器

> 一个极速地图瓦片下载工具，支持高德、谷歌、天地图、Mapbox、OSM 等主流地图服务。

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## 特性

- **多源支持** — 兼容高德、谷歌、天地图、Mapbox、OSM、百度等瓦片服务
- **精准下载** — 支持 GeoJSON 轮廓范围过滤，只下载指定区域
- **多层级并发** — 多个缩放层级同时下载，充分利用带宽
- **断点续传** — 异常退出后重启自动跳过已下载瓦片
- **双存储模式** — 支持 MBTiles 数据库和目录文件两种输出
- **矢量瓦片** — 支持 PBF 矢量瓦片下载
- **跨平台编译** — 纯 Go 实现，CGO 禁用，一条命令交叉编译
- **请求重试** — 自动重试 + 指数退避，应对网络波动

## 快速开始

### 编译

```bash
# 安装 go-task 后，一键打包
task package:linux    # Linux amd64
task package:windows  # Windows amd64
task package:mac-arm  # macOS arm64

# 或直接编译
go build -trimpath -ldflags="-s -w" -o tileclaw .
```

### 配置

编辑 `conf/conf.toml`：

```toml
[task]
    workers = 2        # 并发下载线程数
    timedelay = 100    # 请求间隔 (ms)

[output]
    format = "mbtiles"
    directory = "output"

[tm]
    name = "高德卫星图"
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

### 运行

```bash
./tileclaw -c conf/conf.toml

# 后台运行
nohup ./tileclaw -c conf/conf.toml > /dev/null 2>&1 &
```

## 命令行参数

| 参数 | 说明 |
|------|------|
| `-c` | 指定配置文件，默认 `conf/conf.toml` |
| `-test-mail` | 发送一封测试邮件，用于验证邮件通知配置 |
| `-convert-shp` | 转换 SHP/ZIP 为 GeoJSON 并退出 |
| `-convert-out` | 指定转换后的 GeoJSON 输出路径 |
| `--force-polygon` | 配合 `-convert-shp`，强制 PolyLine 转 Polygon（自动补闭合） |

### SHP 转 GeoJSON

支持 `.shp` 和 `.zip`（包含 .shp）格式，自动识别几何类型并输出 GeoJSON。

```bash
# 基本转换
./tileclaw -convert-shp boundary.zip -convert-out ./geojson/boundary.geojson

# 强制线转面（适用于边界线数据）
./tileclaw -convert-shp boundary.zip -convert-out ./geojson/boundary.geojson --force-polygon
```

转换时会打印几何类型统计，如 `Shapefile geometry: PolyLine x10`。工具会自动检测闭合环并转为 Polygon；如果线条未闭合，可用 `--force-polygon` 强制转为面（自动补闭合）。

### 多 URL 下载

每个 `[[lrs]]` 块可以指定独立的 `url`，未指定则使用全局 `[tm].url`。不同层级范围或不同区域可使用不同的瓦片源。

```toml
# 全局默认 URL
[tm]
    url = "http://webrd0{1-4}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}"

# 低层级用全球边界 + 默认 URL
[[lrs]]
    min = 0
    max = 6
    geojson = "./geojson/global.geojson"

# 高层级中国区域用另一个瓦片源
[[lrs]]
    min = 7
    max = 16
    geojson = "./geojson/china.geojson"
    url = "http://mt0.google.com/vt/lyrs=y&x={x}&y={y}&z={z}"

# 同一层级不同区域也可以用不同 URL
[[lrs]]
    min = 7
    max = 16
    geojson = "./geojson/china_ten_dash_line.geojson"
    url = "http://webst0{1-4}.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}"
```

> URL 支持 `{1-4}` 数字范围和 `{a-c}` 字母范围实现子域名轮询，如 `webst0{1-4}` → `webst01`/`webst02`/`webst03`/`webst04` 随机选取。

## 边界数据来源

下载范围由配置中的 GeoJSON 边界文件决定。可以从以下网站获取省、市、县等行政区划边界数据，并按需转换为 GeoJSON：

- [省市县边界数据](https://www.shengshixian.com/)
- [锐多宝地图数据下载](https://map.ruiduobao.com/)

建议优先使用省级或全国边界数据。县级、乡镇级、村级数据文件较大，通常不适合作为全国瓦片下载范围。公开发布地图成果时，请使用合规、授权、符合审图要求的数据源。

## 地图 URL 参考

### 高德地图

瓦片格式：`jpg`，坐标系：火星坐标 (GCJ-02)，无需 API Key。

| 图层 | 类型 | URL |
|------|------|-----|
| 卫星图 | `jpg` | `http://webst01.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}` |
| 电子地图 | `png` | `http://webrd01.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}` |

> **说明**
> - `style=6`：卫星影像，不含地名标注
> - `style=8`：标准街道图，含地名/路名/POI，有白色底图，**不能叠加到卫星图上**
> - 高德没有透明标注层，如需卫星图 + 标注叠加请使用天地图
> - `webst01` 可换为 `webst02` ~ `webst04` 分散请求
> - `webrd01` 可换为 `webrd02` ~ `webrd04` 分散请求

### 谷歌地图

瓦片格式：`jpg`/`png`，坐标系：默认 WGS-84，`gl=CN` 时使用火星坐标。

| 图层 | 类型 | URL |
|------|------|-----|
| 卫星图 | `jpg` | `http://mt0.google.com/vt/lyrs=s&x={x}&y={y}&z={z}` |
| 影像含标注 | `jpg` | `http://mt0.google.com/vt/lyrs=y&x={x}&y={y}&z={z}` |
| 透明街道标注 | `png` | `http://mt0.google.com/vt/lyrs=h&x={x}&y={y}&z={z}` |
| 街道图 | `png` | `http://mt0.google.com/vt/lyrs=m&x={x}&y={y}&z={z}` |
| 地形图 | `jpg` | `http://mt0.google.com/vt/lyrs=t&x={x}&y={y}&z={z}` |
| 街道图 (含标注) | `png` | `http://mt0.google.com/vt/lyrs=r&x={x}&y={y}&z={z}` |

> **说明**
> - 国内访问需科学上网
> - 下载国内区域加 `&gl=CN` 可获得火星坐标偏移数据
> - `mt0` 可换为 `mt1` ~ `mt3` 分散请求
> - `lyrs=h` 是透明标注层，可叠加到 `lyrs=s` 卫星图上

### 天地图

瓦片格式：`jpg`/`png`，坐标系：火星坐标 (GCJ-02)，**需注册获取 tk**（[注册地址](https://console.tianditu.gov.cn/)）。

| 图层 | 类型 | URL |
|------|------|-----|
| 卫星影像 `img_w` | `jpg` | `https://t0.tianditu.gov.cn/DataServer?T=img_w&x={x}&y={y}&l={z}&tk=你的key` |
| 影像标注 `cia_w` | `png` | `https://t0.tianditu.gov.cn/DataServer?T=cia_w&x={x}&y={y}&l={z}&tk=你的key` |
| 矢量底图 `vec_w` | `png` | `https://t0.tianditu.gov.cn/DataServer?T=vec_w&x={x}&y={y}&l={z}&tk=你的key` |
| 矢量标注 `cva_w` | `png` | `https://t0.tianditu.gov.cn/DataServer?T=cva_w&x={x}&y={y}&l={z}&tk=你的key` |

> **说明**
> - `cia_w` 和 `cva_w` 是透明标注层，可叠加到 `img_w` / `vec_w` 上
> - 天地图有 429 限速，工具已内置重试 + 间隔控制
> - `t0` 可换为 `t1` ~ `t6` 分散请求
> - 请合理使用，不要高频大量下载

### Mapbox

瓦片格式：`jpg`/`png`/`pbf`，坐标系：WGS-84，**需注册获取 access_token**（[注册地址](https://account.mapbox.com/)）。

| 图层 | 类型 | URL |
|------|------|-----|
| 卫星图 | `jpg` | `https://api.mapbox.com/v4/mapbox.satellite/{z}/{x}/{y}.jpg?access_token=你的token` |
| 街道图 | `png` | `https://api.mapbox.com/v4/mapbox.streets/{z}/{x}/{y}.png?access_token=你的token` |
| 浅色风格 | `png` | `https://api.mapbox.com/v4/mapbox.light/{z}/{x}/{y}.png?access_token=你的token` |
| 深色风格 | `png` | `https://api.mapbox.com/v4/mapbox.dark/{z}/{x}/{y}.png?access_token=你的token` |
| 地形图 | `png` | `https://api.mapbox.com/v4/mapbox.mapbox-terrain-v2/{z}/{x}/{y}.png?access_token=你的token` |

> **说明**
> - Mapbox 免费额度每月 50,000 次请求，大规模下载需注意配额
> - 矢量瓦片（PBF）需使用 Mapbox Vector Tiles API，格式不同于栅格瓦片
> - `schema` 设为 `xyz`，Mapbox 使用标准 XYZ 编号

### OpenStreetMap

瓦片格式：`png`，坐标系：WGS-84，无需 API Key。

| 图层 | 类型 | URL |
|------|------|-----|
| 标准地图 | `png` | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` |
| 人道主义风格 | `png` | `https://a.tile.openstreetmap.fr/hot/{z}/{x}/{y}.png` |

> **说明**
> - OSM 瓦片服务有严格的[使用策略](https://operations.osmfoundation.org/policies/tiles/)，禁止大量下载
> - 推荐仅下载小范围区域，或使用其他 OSM 镜像源
> - `tile.openstreetmap.org` 单点服务器，无 CDN，不建议大规模使用

### 百度地图

瓦片格式：`jpg`/`png`，坐标系：百度坐标 (BD-09)，无需 API Key。

| 图层 | 类型 | URL |
|------|------|-----|
| 卫星图 | `jpg` | `http://shangetu0.map.bdimg.com/it/u=x={x};y={y};z={z};v=009;type=sate&fm=46` |
| 电子地图 | `png` | `http://online1.map.bdimg.com/onlinelabel/?qt=tile&x={x}&y={y}&z={z}&styles=pl` |

> **说明**
> - **百度使用 BD-09 坐标系和自定义瓦片编号方案**，与标准 XYZ/TMS 不兼容
> - 以上 URL 模板仅作参考，直接使用可能无法对齐瓦片位置
> - 如需下载百度地图，建议在处理端做坐标转换（BD-09 → WGS-84）
> - 百度地图的瓦片行号从左上角开始，与 TMS 方案（左下角）不同

### 坐标系说明

| 坐标系 | 使用方 | 说明 |
|--------|--------|------|
| WGS-84 | Google(无偏移), OSM, Mapbox | 国际标准坐标系 |
| GCJ-02 (火星坐标) | 高德, 天地图, Google(`gl=CN`) | 国测局加密坐标系 |
| BD-09 | 百度 | 百度在 GCJ-02 基础上二次加密 |

> 不同坐标系的瓦片无法直接叠加使用，需注意对齐。

## 与 Tileserver-GL 配合使用

将生成的 `.mbtiles` 文件放入 tileserver-gl 的 `data/` 目录即可直接服务。

## 致谢

本项目基于 [atlasdatatech/tiler](https://github.com/atlasdatatech/tiler.git) 二次开发，感谢原作者的开源贡献。

主要改进：
- 数据库驱动切换为纯 Go 实现，支持 CGO 禁用交叉编译
- HTTP 连接池复用 + 请求重试机制
- 多图层并发下载，大幅提升下载速度
- 断点续传，异常退出不丢失进度
- 确定性输出路径，重启可继续
- 完善的日志输出与 panic 恢复

## 免责声明

本项目仅用于学习、研究和个人技术验证。请勿将本工具用于违反地图服务条款、数据授权协议、法律法规或审图要求的用途。使用者应自行确认瓦片服务、边界数据、行政区划数据及生成成果的授权和合规性。因使用本项目造成的任何风险或责任，由使用者自行承担。

## License

MIT
