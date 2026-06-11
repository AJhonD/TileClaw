package config

import (
	"os"
	"path/filepath"
	"tileclaw/internal/log"

	"github.com/spf13/viper"
)

var Config AppConfig

const DefaultConfigFile = "conf/conf.toml"

type AppConfig struct {
	App           App           `mapstructure:"app"`
	OutputConfig  OutputConfig  `mapstructure:"output"`
	TaskConfig    TaskConfig    `mapstructure:"task"`
	TileMapConfig TileMapConfig `mapstructure:"tm"`
	CfgLayers     []cfgLayer    `mapstructure:"lrs"`
}

func resolveConfigFile(confFile string) string {
	if confFile == "" {
		confFile = DefaultConfigFile
	}
	if _, err := os.Stat(confFile); err == nil {
		return confFile
	}

	log.SysLog.Warnf("config file(%s) not exist", confFile)
	for _, fallback := range []string{DefaultConfigFile, "conf.toml"} {
		if fallback == confFile {
			continue
		}
		if _, err := os.Stat(fallback); err == nil {
			log.SysLog.Warnf("use fallback config file(%s)", fallback)
			return fallback
		}
	}
	return confFile
}

// App 配置
type App struct {
	version string
	title   string
}

// OutputConfig 输出配置
type OutputConfig struct {
	Format    string `mapstructure:"format"`
	OutputDir string `mapstructure:"directory"`
}

type TaskConfig struct {
	Workers   int `mapstructure:"workers"`
	SavePipes int `mapstructure:"savepipe"`
	TimeDelay int `mapstructure:"timedelay"`
	MergeBuf  int `mapstructure:"mergebuf"`
}

// TileMapConfig 瓦片地图配置
type TileMapConfig struct {
	Name      string  `mapstructure:"name"`
	Min       int     `mapstructure:"min"`
	Max       int     `mapstructure:"max"`
	Format    string  `mapstructure:"format"`
	Schema    string  `mapstructure:"schema"`
	JSON      string  `mapstructure:"json"`
	URL       string  `mapstructure:"url"`
	CenterLon float64 `mapstructure:"center_lon"`
	CenterLat float64 `mapstructure:"center_lat"`
}

type cfgLayer struct {
	Min     int    `mapstructure:"min"`
	Max     int    `mapstructure:"max"`
	GeoJson string `mapstructure:"geojson"`
	URL     string `mapstructure:"url"`
}

// 创建默认配置
func newDefaultConfig() AppConfig {
	return AppConfig{
		App: App{
			version: "0.1.0",
			title:   "tileclaw",
		},
		OutputConfig: OutputConfig{
			Format:    "mbtiles",
			OutputDir: "output",
		},
		TaskConfig: TaskConfig{
			Workers:   10,
			SavePipes: 100,
			TimeDelay: 10,
		},
	}
}

func Init(confFile string) {
	confFile = resolveConfigFile(confFile)
	defer InitMailConfig(filepath.Dir(confFile))
	viper.SetConfigFile(confFile)
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv() // read in environment variables that match

	if err := viper.ReadInConfig(); err != nil {
		log.SysLog.Fatal("配置读取失败：", err)
	}

	Config = newDefaultConfig()

	if err := viper.Unmarshal(&Config); err != nil {
		log.SysLog.Fatal("配置解析失败：", err)
	}
}
