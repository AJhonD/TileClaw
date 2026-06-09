package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	log "github.com/sirupsen/logrus"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/shiena/ansicolor"
	"github.com/spf13/viper"
	_ "modernc.org/sqlite"
)

var (
	hf       bool
	cf       string
	testMail bool
	sysLog   = log.New()
	progLog  = log.New()
)

func initLogging() {
	formatter := &nested.Formatter{
		HideKeys:        true,
		ShowFullLevel:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	}

	// 系统日志 → tileclaw.log + stdout
	sysLog.SetFormatter(formatter)
	sysLog.SetLevel(log.InfoLevel)
	sysFile, err := os.OpenFile("tileclaw.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		sysLog.SetOutput(io.MultiWriter(ansicolor.NewAnsiColorWriter(os.Stdout), sysFile))
	} else {
		sysLog.SetOutput(ansicolor.NewAnsiColorWriter(os.Stdout))
	}

	// 进度日志 → download.log + stdout (无颜色，量大)
	progLog.SetFormatter(formatter)
	progLog.SetLevel(log.InfoLevel)
	progFile, err := os.OpenFile("download.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		progLog.SetOutput(io.MultiWriter(os.Stdout, progFile))
	} else {
		progLog.SetOutput(os.Stdout)
	}
}

func init() {
	flag.BoolVar(&hf, "h", false, "this help")
	flag.StringVar(&cf, "c", "conf.toml", "set config `file`")
	flag.BoolVar(&testMail, "test-mail", false, "send a test email and exit")
	flag.Usage = usage
}
func usage() {
	fmt.Fprintf(os.Stderr, `TileClaw v0.2.0 Usage: tileclaw [-h] [-c filename]`)
	flag.PrintDefaults()
}

// initConf 初始化配置
func initConf(cfgFile string) {
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		sysLog.Warnf("config file(%s) not exist", cfgFile)
	}
	viper.SetConfigType("toml")
	viper.SetConfigFile(cfgFile)
	viper.AutomaticEnv() // read in environment variables that match
	err := viper.ReadInConfig()
	if err != nil {
		sysLog.Warnf("read config file(%s) error, details: %s", viper.ConfigFileUsed(), err)
	}
	viper.SetDefault("app.version", "v0.2.0")
	viper.SetDefault("app.title", "TileClaw")
	viper.SetDefault("output.format", "mbtiles")
	viper.SetDefault("output.directory", "output")
	viper.SetDefault("task.workers", 4)
	viper.SetDefault("task.savepipe", 1)
	viper.SetDefault("task.timedelay", 0)
}

type TileData struct {
	Z    int
	X    int
	Y    int
	Flag bool
}

// 插入瓦片数据
func insertTiles(db *sql.Tx, tiles []TileData) error {
	// 准备插入语句
	stmt, err := db.Prepare("INSERT INTO tiles (z, x, y,flag) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	// 执行插入
	for _, tile := range tiles {
		_, err = stmt.Exec(tile.Z, tile.X, tile.Y, tile.Flag)
		if err != nil {
			return err
		}
	}
	return nil
}

func testDbTask() {

	db, err := sql.Open("sqlite", "./tiles.db")
	if err != nil {
		sysLog.Fatal(err)
	}
	defer db.Close()

	createTableSQL := `CREATE TABLE IF NOT EXISTS tiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		z INTEGER,
		x INTEGER,
		y INTEGER,
		flag BOOLEAN
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		sysLog.Fatal(err)
	}

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		sysLog.Fatal(err)
	}
	defer tx.Rollback() // 如果提交事务前发生错误，则回滚事务

	batchSize := 1 << 10
	// 插入数据
	tileBatch := make([]TileData, 0, batchSize)
	total := 0
	i := 0
	for z := 0; z <= 12; z++ {
		numTiles := 1 << uint(z) // 计算每个缩放级别的瓦片数量
		total += numTiles * numTiles
		sysLog.Printf("级别%d,瓦片数量：%d\n", z, numTiles*numTiles)
		sysLog.Printf("总瓦片数量：%d\n", total)
		for x := 0; x < numTiles; x++ {
			for y := 0; y < numTiles; y++ {
				tile := TileData{Z: z, X: x, Y: y, Flag: false}
				tileBatch = append(tileBatch, tile)
				// 批量插入
				if len(tileBatch) >= batchSize {
					err := insertTiles(tx, tileBatch) // 使用事务执行批量插入操作
					if err != nil {
						sysLog.Fatal(err)
					}
					tileBatch = nil // 清空批次
				}
				i++
				if i > 99999999 {
					goto last
				}
			}

		}
	}
last:
	// 处理剩余的批次
	if len(tileBatch) > 0 {
		err := insertTiles(tx, tileBatch) // 使用事务执行批量插入操作
		if err != nil {
			sysLog.Fatal(err)
		}
	}
	// 提交事务
	err = tx.Commit()
	if err != nil {
		sysLog.Fatal(err)
	}
}

func main() {

	defer func() {
		if r := recover(); r != nil {
			sysLog.Errorf("程序异常退出 (panic): %v\n堆栈信息:\n%s", r, debug.Stack())
			notifyPanic(fmt.Sprintf("%v", r))
			os.Exit(1)
		}
	}()

	flag.Parse()
	if hf {
		flag.Usage()
		return
	}

	if cf == "" {
		cf = "conf.toml"
	}
	initLogging()
	initConf(cf)
	loadMailConfig()

	if testMail {
		sendMail("[TileClaw] Test Email", "This is a test email from TileClaw.\n\nIf you see this, your mail config is working correctly.\n\n-- TileClaw")
		sysLog.Println("test email sent, check your inbox")
		return
	}
	start := time.Now()
	tm := TileMap{
		Name:      viper.GetString("tm.name"),
		Min:       viper.GetInt("tm.min"),
		Max:       viper.GetInt("tm.max"),
		Format:    viper.GetString("tm.format"),
		Schema:    viper.GetString("tm.schema"),
		JSON:      viper.GetString("tm.json"),
		URL:       viper.GetString("tm.url"),
		CenterLon: viper.GetFloat64("tm.center_lon"),
		CenterLat: viper.GetFloat64("tm.center_lat"),
	}
	type cfgLayer struct {
		Min     int
		Max     int
		Geojson string
		URL     string
	}
	var cfgLrs []cfgLayer
	err := viper.UnmarshalKey("lrs", &cfgLrs)
	if err != nil {
		sysLog.Fatal("lrs配置错误")
	}
	var layers []Layer
	for _, lrs := range cfgLrs {
		for z := lrs.Min; z <= lrs.Max; z++ {
			c := loadCollection(lrs.Geojson)
			layer := Layer{
				URL:        lrs.URL,
				Zoom:       z,
				Collection: c,
			}
			layers = append(layers, layer)
		}
	}
	task := NewTask(layers, tm)
	sysLog.Printf("start download map tilers, workerCount: {%d}\r\n", task.workerCount)
	task.Download()
	secs := time.Since(start).Seconds()
	sysLog.Printf("\n%.3fs finished...", secs)
}
