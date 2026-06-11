package app

import (
	"tileclaw/internal/config"
	"tileclaw/internal/log"
	_map "tileclaw/internal/map"
	"tileclaw/internal/task"
	"tileclaw/internal/tile"
	"tileclaw/internal/util"
	"time"

	"github.com/jinzhu/copier"
)

func RunDownloadTask() {
	start := time.Now()

	tm := _map.TileMap{}
	tmc := config.Config.TileMapConfig
	if err := copier.Copy(&tm, &tmc); err != nil {
		log.SysLog.Fatal("config convert error: ", err)
	}

	cfgLayers := config.Config.CfgLayers
	var layers []tile.Layer
	for _, lrs := range cfgLayers {
		for z := lrs.Min; z <= lrs.Max; z++ {
			layer := tile.Layer{
				URL:        lrs.URL,
				Zoom:       z,
				Collection: util.LoadCollection(lrs.GeoJson),
			}
			layers = append(layers, layer)
		}
	}

	downloadTask := task.NewTask(layers, tm, optionsFromConfig(config.Config))
	if downloadTask == nil {
		log.SysLog.Warningf("The task is nil, stopped")
		return
	}
	log.SysLog.Printf("start download map tilers, workerCount: {%d}\r\n", downloadTask.WorkerCount())

	downloadTask.Download()

	secs := time.Since(start).Seconds()
	log.SysLog.Printf("\n%.3fs finished...", secs)
}

func optionsFromConfig(cfg config.AppConfig) task.Options {
	return task.Options{
		WorkerCount:     cfg.TaskConfig.Workers,
		SavePipeSize:    cfg.TaskConfig.SavePipes,
		TimeDelay:       cfg.TaskConfig.TimeDelay,
		BufferSize:      cfg.TaskConfig.MergeBuf,
		OutputFormat:    cfg.OutputConfig.Format,
		OutputDirectory: cfg.OutputConfig.OutputDir,
	}
}
