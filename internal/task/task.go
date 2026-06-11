package task

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"tileclaw/internal/downloader"
	"tileclaw/internal/log"
	"tileclaw/internal/mail"
	_map "tileclaw/internal/map"
	"tileclaw/internal/storage"
	"tileclaw/internal/tile"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	"github.com/teris-io/shortid"
	pb "gopkg.in/cheggaaa/pb.v1"
)

const MBTileVersion = "1.2"

type Options struct {
	WorkerCount     int
	SavePipeSize    int
	TimeDelay       int
	BufferSize      int
	OutputFormat    string
	OutputDirectory string
}

type Task struct {
	ID                 string
	Name               string
	Description        string
	File               string
	Min                int
	Max                int
	Layers             []tile.Layer
	TileMap            _map.TileMap
	Total              int64
	Current            int64
	Bar                *pb.ProgressBar
	workerCount        int
	savePipeSize       int
	bufSize            int
	tileWG             sync.WaitGroup
	abort, pause, play chan struct{}
	workers            chan maptile.Tile
	store              storage.TileStore
	fetcher            *downloader.Fetcher
	outformat          string
	outputDir          string
}

func NewTask(layers []tile.Layer, m _map.TileMap, options Options) *Task {
	if len(layers) == 0 {
		return nil
	}
	id, _ := shortid.Generate()

	task := Task{
		ID:        id,
		Name:      m.Name,
		Layers:    layers,
		Min:       m.Min,
		Max:       m.Max,
		TileMap:   m,
		outformat: options.OutputFormat,
		outputDir: options.OutputDirectory,
	}

	for i := 0; i < len(layers); i++ {
		if layers[i].URL == "" {
			layers[i].URL = m.URL
		}
		layers[i].Count = tilecover.CollectionCount(layers[i].Collection, maptile.Zoom(layers[i].Zoom))
		log.SysLog.Printf("zoom: %d, tiles: %d \n", layers[i].Zoom, layers[i].Count)
		task.Total += layers[i].Count
	}

	task.abort = make(chan struct{})
	task.pause = make(chan struct{})
	task.play = make(chan struct{})
	task.workerCount = options.WorkerCount
	task.savePipeSize = options.SavePipeSize
	task.bufSize = options.BufferSize
	task.workers = make(chan maptile.Tile, task.workerCount)
	task.fetcher = downloader.New(m.Format, options.TimeDelay)
	return &task
}

func (task *Task) WorkerCount() int {
	return task.workerCount
}

func (task *Task) Bound() orb.Bound {
	bound := orb.Bound{Min: orb.Point{1, 1}, Max: orb.Point{-1, -1}}
	for _, layer := range task.Layers {
		for _, g := range layer.Collection {
			bound = bound.Union(g.Bound())
		}
	}
	return bound
}

func (task *Task) Center() orb.Point {
	layer := task.Layers[len(task.Layers)-1]
	bound := orb.Bound{Min: orb.Point{1, 1}, Max: orb.Point{-1, -1}}
	for _, g := range layer.Collection {
		bound = bound.Union(g.Bound())
	}
	return bound.Center()
}

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
		"pixel_scale": strconv.Itoa(tile.TileSize),
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

func (task *Task) setupStore() error {
	if task.outformat == "mbtiles" {
		if task.File == "" {
			task.File = storage.DefaultMBTilesPath(task.outputDir, task.Name)
		}
		store, err := storage.NewMBTilesStore(task.File, task.MetaItems(), task.Min, task.Max, task.savePipeSize)
		if err != nil {
			return err
		}
		task.store = store
		return nil
	}

	if task.File == "" {
		task.File = filepath.Join(task.outputDir, task.Name)
	}
	store, err := storage.NewFileStore(task.File, task.TileMap.Format)
	if err != nil {
		return err
	}
	task.store = store
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

func (task *Task) tileExists(mt maptile.Tile) bool {
	exists, err := task.store.Exists(mt)
	if err != nil {
		log.SysLog.Warnf("check existing tile (z:%d, x:%d, y:%d) error: %s", mt.Z, mt.X, mt.Y, err)
		return false
	}
	return exists
}

func (task *Task) tileFetcher(mt maptile.Tile, url string) {
	defer task.tileWG.Done()
	defer func() {
		<-task.workers
	}()

	if task.tileExists(mt) {
		log.SysLog.Debugf("skip existing tile (z:%d, x:%d, y:%d)", mt.Z, mt.X, mt.Y)
		return
	}

	result, err := task.fetcher.Fetch(mt, url)
	if err != nil {
		log.SysLog.Errorf("%s", err)
		mail.CountError()
		return
	}
	if err := task.store.Save(result.Tile); err != nil {
		log.SysLog.Errorf("save tile %v error: %s", mt, err)
		mail.CountError()
		return
	}

	log.ProgLog.Infof("tile(z:%d, x:%d, y:%d), %dms , %.2f kb, %s ...\n", mt.Z, mt.X, mt.Y, result.Duration.Milliseconds(), float32(result.Size)/1024.0, result.URL)
}

func (task *Task) downloadLayer(layer tile.Layer) {
	bar := pb.New64(layer.Count).Prefix(fmt.Sprintf("Zoom %d : ", layer.Zoom)).Postfix("\n")
	bar.Start()

	tilelist := make(chan maptile.Tile, task.bufSize)
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
			log.SysLog.Infof("Task %s got canceled.", task.ID)
			close(tilelist)
		case <-task.pause:
			log.SysLog.Infof("Task %s suspended.", task.ID)
			select {
			case <-task.play:
				log.SysLog.Infof("Task %s go on.", task.ID)
			case <-task.abort:
				log.SysLog.Infof("Task %s got canceled.", task.ID)
				close(tilelist)
			}
		}
	}
	bar.FinishPrint(fmt.Sprintf("Zoom %d dispatch finished ~", layer.Zoom))
}

func (task *Task) Download() {
	downloadStart := time.Now()
	task.Bar = pb.New64(task.Total).Prefix("Task : ").Postfix("\n")
	task.Bar.Start()

	if err := task.setupStore(); err != nil {
		log.SysLog.Fatalf("init storage error: %s", err)
	}

	done := make(chan struct{})
	go task.timingPrintingProgress(done)

	var dWg sync.WaitGroup
	for _, layer := range task.Layers {
		dWg.Add(1)
		go func(layer tile.Layer) {
			defer dWg.Done()
			task.downloadLayer(layer)
		}(layer)
	}
	dWg.Wait()
	task.tileWG.Wait()
	close(done)

	if err := task.store.Close(); err != nil {
		log.SysLog.Errorf("close storage error: %v", err)
	}

	task.Bar.FinishPrint(fmt.Sprintf("Task %s finished ~", task.ID))
	elapsed := time.Since(downloadStart)
	mail.NotifyComplete(task.Current, task.Total, elapsed)
}

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
			log.SysLog.Infof("Progress: %d / %d (%.2f%%), elapsed %s, ETA %s", current, task.Total, pct, elapsed, eta)
			prevCurrent = current
			prevTime = now
		case <-done:
			return
		}
	}
}
