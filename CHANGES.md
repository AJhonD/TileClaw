### 变更说明

#### 2026-06-03

- 数据库驱动从 `go-sqlite3` 切换为 `modernc.org/sqlite`，支持 CGO 禁用交叉编译
- HTTP 连接池复用（MaxIdleConns: 100, MaxIdleConnsPerHost: 20）
- 瓦片请求添加重试机制（3 次，指数退避 1s/2s/4s）
- 瓦片数据完整性校验（最小字节数 256，PBF 格式例外）
- MBTiles 批量写入（每 200 个瓦片一个事务）
- GeoJSON 文件加载添加缓存（`sync.Map`），避免重复解析
- 多图层并发下载（goroutine + WaitGroup）
- 断点续传：输出目录模式下跳过已存在的瓦片文件，MBTiles 模式下跳过已入库的瓦片
- 输出路径确定性：不再包含随机 shortid，重启后可续传
- MBTiles 索引创建使用 `IF NOT EXISTS`，防止重复运行报错
- 元数据 `json` 字段仅非空时写入，避免 tileserver-gl JSON 解析错误
- 程序入口添加 panic recovery，异常时输出堆栈到日志
- 日志同时输出到文件和控制台
- Taskfile.yml 多平台编译打包（linux/windows/darwin, amd64/arm64）
- Linux/macOS 打包产物自动 `chmod +x`
- 编译使用 `-trimpath`，堆栈信息不暴露本地路径
- `mkdir -p` 防止重复打包时目录已存在报错

#### 2022-10-01

- 添加 `timedelay` 限速参数
> 最小延迟时间`timedelay`，单位`ms`，用于请求限速，最准确的控制是将`works`设置为`1`，限速是为最准确最小间隔，若`works > 1`则`timedelay`只是个调和参数