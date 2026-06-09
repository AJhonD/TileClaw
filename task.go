package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	"github.com/spf13/viper"
	"github.com/teris-io/shortid"
	pb "gopkg.in/cheggaaa/pb.v1"
)

// MBTileVersion mbtiles版本号
const MBTileVersion = "1.2"

var httpClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// Task 下载任务
type Task struct {
	ID                 string
	Name               string
	Description        string
	File               string
	Min                int
	Max                int
	Layers             []Layer
	TileMap            TileMap
	Total              int64
	Current            int64
	Bar                *pb.ProgressBar
	db                 *sql.DB
	workerCount        int
	savePipeSize       int
	timeDelay          int
	bufSize            int
	tileWG             sync.WaitGroup
	abort, pause, play chan struct{}
	workers            chan maptile.Tile
	savingpipe         chan Tile
	tileSet            Set
	outformat          string
}

// NewTask 创建下载任务
func NewTask(layers []Layer, m TileMap) *Task {
	if len(layers) == 0 {
		return nil
	}
	id, _ := shortid.Generate()

	task := Task{
		ID:      id,
		Name:    m.Name,
		Layers:  layers,
		Min:     m.Min,
		Max:     m.Max,
		TileMap: m,
	}

	for i := 0; i < len(layers); i++ {
		if layers[i].URL == "" {
			layers[i].URL = m.URL
		}
		layers[i].Count = tilecover.CollectionCount(layers[i].Collection, maptile.Zoom(layers[i].Zoom))
		sysLog.Printf("zoom: %d, tiles: %d \n", layers[i].Zoom, layers[i].Count)
		task.Total += layers[i].Count
	}
	task.abort = make(chan struct{})
	task.pause = make(chan struct{})
	task.play = make(chan struct{})

	task.workerCount = viper.GetInt("task.workers")
	task.savePipeSize = viper.GetInt("task.savepipe")
	task.timeDelay = viper.GetInt("task.timedelay")
	task.workers = make(chan maptile.Tile, task.workerCount)
	task.savingpipe = make(chan Tile, task.savePipeSize)
	task.bufSize = viper.GetInt("task.mergebuf")
	task.tileSet = Set{M: make(maptile.Set)}

	task.outformat = viper.GetString("output.format")
	return &task
}

// Bound 范围
func (task *Task) Bound() orb.Bound {
	bound := orb.Bound{Min: orb.Point{1, 1}, Max: orb.Point{-1, -1}}
	for _, layer := range task.Layers {
		for _, g := range layer.Collection {
			bound = bound.Union(g.Bound())
		}
	}
	return bound
}

// Center 中心点
func (task *Task) Center() orb.Point {
	layer := task.Layers[len(task.Layers)-1]
	bound := orb.Bound{Min: orb.Point{1, 1}, Max: orb.Point{-1, -1}}
	for _, g := range layer.Collection {
		bound = bound.Union(g.Bound())
	}
	return bound.Center()
}

// MetaItems 输出
func (task *Task) MetaItems() map[string]string {
	b := task.Bound()
	var cx, cy float64
	if task.TileMap.CenterLon != 0 || task.TileMap.CenterLat != 0 {
		cx = task.TileMap.CenterLon
		cy = task.TileMap.CenterLat
	} else {
		c := task.Center()
		cx = c.X()
		cy = c.Y()
	}
	data := map[string]string{
		"id":          task.ID,
		"name":        task.Name,
		"description": task.Description,
		"attribution": `<a href="http://www.atlasdata.cn/" target="_blank">&copy; MapCloud</a>`,
		"basename":    task.TileMap.Name,
		"format":      task.TileMap.Format,
		"type":        task.TileMap.Schema,
		"pixel_scale": strconv.Itoa(TileSize),
		"version":     MBTileVersion,
		"bounds":      fmt.Sprintf(`%f,%f,%f,%f`, b.Left(), b.Bottom(), b.Right(), b.Top()),
		"center":      fmt.Sprintf(`%f,%f,%d`, cx, cy, (task.Min+task.Max)/2),
		"minzoom":     strconv.Itoa(task.Min),
		"maxzoom":     strconv.Itoa(task.Max),
	}
	if task.TileMap.JSON != "" {
		data["json"] = task.TileMap.JSON
	}
	return data
}

// SetupMBTileTables 初始化配置MBTile库
func (task *Task) SetupMBTileTables() error {

	if task.File == "" {
		outdir := viper.GetString("output.directory")
		os.MkdirAll(outdir, os.ModePerm)
		task.File = filepath.Join(outdir, fmt.Sprintf("%s.mbtiles", task.Name))
	}
	db, err := sql.Open("sqlite", task.File)
	if err != nil {
		return err
	}

	err = optimizeConnection(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("create table if not exists tiles (zoom_level integer, tile_column integer, tile_row integer, tile_data blob);")
	if err != nil {
		return err
	}

	_, err = db.Exec("create table if not exists metadata (name text, value text);")
	if err != nil {
		return err
	}

	_, err = db.Exec("create unique index if not exists name on metadata (name);")
	if err != nil {
		return err
	}

	_, err = db.Exec("create unique index if not exists tile_index on tiles(zoom_level, tile_column, tile_row);")
	if err != nil {
		return err
	}

	// Load metadata, merging minzoom/maxzoom with existing values
	items := task.MetaItems()
	var oldMin, oldMax string
	_ = db.QueryRow("SELECT value FROM metadata WHERE name='minzoom'").Scan(&oldMin)
	_ = db.QueryRow("SELECT value FROM metadata WHERE name='maxzoom'").Scan(&oldMax)
	if v, _ := strconv.Atoi(oldMin); v < task.Min {
		items["minzoom"] = oldMin
	}
	if v, _ := strconv.Atoi(oldMax); v > task.Max {
		items["maxzoom"] = oldMax
	}
	for name, value := range items {
		_, err := db.Exec("insert or replace into metadata (name, value) values (?, ?)", name, value)
		if err != nil {
			return err
		}
	}

	task.db = db //保存任务的库连接
	return nil
}

func (task *Task) abortFun() {
	task.abort <- struct{}{}
}

func (task *Task) pauseFun() {
	task.pause <- struct{}{}
}

func (task *Task) playFun() {
	task.play <- struct{}{}
}

// SavePipe 保存瓦片管道
func (task *Task) savePipe() {
	batch := make([]Tile, 0, 200)
	for tile := range task.savingpipe {
		batch = append(batch, tile)
		if len(batch) >= 200 {
			saveBatchToMBTile(batch, task.db)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		saveBatchToMBTile(batch, task.db)
	}
}

// SaveTile 保存瓦片
func (task *Task) saveTile(tile Tile) error {
	err := saveToFiles(tile, task)
	if err != nil {
		sysLog.Errorf("create %v tile file error ~ %s", tile.T, err)
	}
	return nil
}

// tileFetcher 瓦片加载器
func (task *Task) tileFetcher(mt maptile.Tile, url string) {
	start := time.Now()
	defer task.tileWG.Done()
	defer func() {
		<-task.workers
	}()

	// 断点续传：跳过已下载的瓦片
	if task.outformat != "mbtiles" {
		path := getTileFilePath(task, mt)
		if _, err := os.Stat(path); err == nil {
			sysLog.Debugf("skip existing tile (z:%d, x:%d, y:%d)", mt.Z, mt.X, mt.Y)
			return
		}
	} else {
		zpower := uint32(math.Pow(2.0, float64(mt.Z)))
		flippedY := zpower - 1 - mt.Y
		var count int
		err := task.db.QueryRow("SELECT COUNT(*) FROM tiles WHERE zoom_level=? AND tile_column=? AND tile_row=?",
			mt.Z, mt.X, flippedY).Scan(&count)
		if err == nil && count > 0 {
			sysLog.Debugf("skip existing tile (z:%d, x:%d, y:%d)", mt.Z, mt.X, mt.Y)
			return
		}
	}

	prep := func(t maptile.Tile, url string) string {
		url = strings.Replace(url, "{x}", strconv.Itoa(int(t.X)), -1)
		url = strings.Replace(url, "{y}", strconv.Itoa(int(t.Y)), -1)
		maxY := int(math.Pow(2, float64(t.Z))) - 1
		url = strings.Replace(url, "{-y}", strconv.Itoa(maxY-int(t.Y)), -1)
		url = strings.Replace(url, "{z}", strconv.Itoa(int(t.Z)), -1)
		// 子域名范围: {1-4} 随机数字, {a-c} 随机字母
		numRangeRe := regexp.MustCompile(`\{(\d+)-(\d+)\}`)
		url = numRangeRe.ReplaceAllStringFunc(url, func(m string) string {
			parts := numRangeRe.FindStringSubmatch(m)
			if len(parts) == 3 {
				lo, _ := strconv.Atoi(parts[1])
				hi, _ := strconv.Atoi(parts[2])
				return strconv.Itoa(lo + rand.IntN(hi-lo+1))
			}
			return m
		})
		letterRangeRe := regexp.MustCompile(`\{([a-z])-([a-z])\}`)
		url = letterRangeRe.ReplaceAllStringFunc(url, func(m string) string {
			parts := letterRangeRe.FindStringSubmatch(m)
			if len(parts) == 3 {
				lo := int(parts[1][0])
				hi := int(parts[2][0])
				return string(rune(lo + rand.IntN(hi-lo+1)))
			}
			return m
		})
		return url
	}
	tileURL := prep(mt, url)

	req, err := http.NewRequest("GET", tileURL, nil)
	if err != nil {
		sysLog.Errorf("create request error: %s", err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://map.tianditu.gov.cn")

	if task.timeDelay > 0 {
		time.Sleep(time.Duration(task.timeDelay) * time.Millisecond)
	}

	var body []byte
	var statusCode int
	minBodySize := 256
	if task.TileMap.Format == PBF || task.TileMap.Format == PNG {
		minBodySize = 1
	}

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := httpClient.Do(req)
		if err != nil {
			sysLog.Warnf("fetch %s attempt %d error: %s", tileURL, attempt+1, err)
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}
		statusCode = resp.StatusCode
		if resp.StatusCode == 200 {
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				sysLog.Warnf("read %s attempt %d error: %s", tileURL, attempt+1, err)
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			if len(body) < minBodySize {
				sysLog.Warnf("tile %v too small (%d bytes), retry", mt, len(body))
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			break
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			sysLog.Warnf("fetch %s status %d attempt %d", tileURL, resp.StatusCode, attempt+1)
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}
		break // 4xx not retried
	}
	if statusCode != 200 || len(body) < minBodySize {
		if statusCode == 200 && len(body) < minBodySize {
			sysLog.Errorf("tile %v too small after retries (%d bytes)", mt, len(body))
			countError()
		} else {
			sysLog.Errorf("fetch %v tile error, status code: %d ~", mt, statusCode)
			countError()
		}
		return
	}
	if len(body) == 0 {
		sysLog.Warnf("nil tile %v ~", mt)
		countError()
		return
	}
	// tiledata
	td := Tile{
		T: mt,
		C: body,
	}

	if task.TileMap.Format == PBF {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, err = zw.Write(body)
		if err != nil {
			sysLog.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			sysLog.Fatal(err)
		}
		td.C = buf.Bytes()
	}

	//enable savingpipe
	if task.outformat == "mbtiles" {
		task.savingpipe <- td
	} else {
		task.saveTile(td)
	}

	cost := time.Since(start).Milliseconds()
	progLog.Infof("tile(z:%d, x:%d, y:%d), %dms , %.2f kb, %s ...\n", mt.Z, mt.X, mt.Y, cost, float32(len(body))/1024.0, tileURL)
}

// DownloadZoom 下载指定层级
func (task *Task) downloadLayer(layer Layer) {
	bar := pb.New64(layer.Count).Prefix(fmt.Sprintf("Zoom %d : ", layer.Zoom)).Postfix("\n")
	bar.Start()

	var tilelist = make(chan maptile.Tile, task.bufSize)

	go tilecover.CollectionChannel(layer.Collection, maptile.Zoom(layer.Zoom), tilelist)

	for tile := range tilelist {
		select {
		case task.workers <- tile:
			bar.Increment()
			task.Bar.Increment()
			atomic.AddInt64(&task.Current, 1)
			task.tileWG.Add(1)
			go task.tileFetcher(tile, layer.URL)
		case <-task.abort:
			sysLog.Infof("Task %s got canceled.", task.ID)
			close(tilelist)
		case <-task.pause:
			sysLog.Infof("Task %s suspended.", task.ID)
			select {
			case <-task.play:
				sysLog.Infof("Task %s go on.", task.ID)
			case <-task.abort:
				sysLog.Infof("Task %s got canceled.", task.ID)
				close(tilelist)
			}
		}
	}
	bar.FinishPrint(fmt.Sprintf("Zoom %d dispatch finished ~", layer.Zoom))
}

// Download 开启下载任务
func (task *Task) Download() {
	downloadStart := time.Now()
	task.Bar = pb.New64(task.Total).Prefix("Task : ").Postfix("\n")
	task.Bar.Start()
	// 不用的格式创建存储介质
	if task.outformat == "mbtiles" {
		if err := task.SetupMBTileTables(); err != nil {
			sysLog.Fatalf("init mbtiles error: %s", err)
		}
	} else {
		if task.File == "" {
			outDir := viper.GetString("output.directory")
			task.File = filepath.Join(outDir, task.Name)
		}
		err := os.MkdirAll(task.File, os.ModePerm)
		if err != nil {
			sysLog.Fatalf("create output directory error: %s", err)
			return
		}
	}
	// 保存管道
	go task.savePipe()

	// 定期进度日志，30s 一条，方便后台运行时查看
	done := make(chan struct{})
	go task.timingPrintingProgress(done)

	var dWg sync.WaitGroup
	for _, layer := range task.Layers {
		dWg.Add(1)
		go func(layer Layer) {
			defer dWg.Done()
			task.downloadLayer(layer)
		}(layer)
	}
	dWg.Wait()
	task.tileWG.Wait()
	close(done)
	close(task.savingpipe)
	if task.db != nil {
		_, err := task.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		if err != nil {
			sysLog.Errorf("wal checkpoint 失败: %v", err)
		}
		// 2. 关闭数据库（必须拿 error）
		err = task.db.Close()
		if err != nil {
			sysLog.Errorf("关闭数据库失败: %v", err)
		}
		task.db = nil
	}
	task.Bar.FinishPrint(fmt.Sprintf("Task %s finished ~", task.ID))
	elapsed := time.Since(downloadStart)
	notifyComplete(task.Current, task.Total, elapsed)
}

// 定时进度打印
func (task *Task) timingPrintingProgress(done chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	start := time.Now()
	prevCurrent := int64(0)
	prevTime := start
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			current := atomic.LoadInt64(&task.Current)
			elapsed := now.Sub(start).Truncate(time.Second)
			pct := float64(current) / float64(task.Total) * 100
			var eta string
			interval := now.Sub(prevTime).Seconds()
			delta := current - prevCurrent
			if delta > 0 && interval > 0 {
				rate := float64(delta) / interval
				remaining := time.Duration(float64(task.Total-current)/rate) * time.Second
				eta = remaining.Truncate(time.Second).String()
			} else {
				eta = "-"
			}
			sysLog.Infof("Progress: %d / %d (%.2f%%), elapsed %s, ETA %s", current, task.Total, pct, elapsed, eta)
			prevCurrent = current
			prevTime = now
		case <-done:
			return
		}
	}
}
