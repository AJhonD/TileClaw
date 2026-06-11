package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"tileclaw/internal/app"
	"tileclaw/internal/config"
	"tileclaw/internal/converter"
	"tileclaw/internal/log"
	"tileclaw/internal/mail"
)

var (
	cf               string
	testMail         bool
	convertShpInput  string
	convertShpOut    string
	convertForcePoly bool
)

func main() {
	defer handlePanic()
	flag.Parse()
	handleConvertShp()
	config.Init(cf)
	handleTestMail()
	app.RunDownloadTask()
}

func init() {
	flag.Func("h", "show help", func(s string) error {
		_, _ = fmt.Fprintf(os.Stderr, "TileClaw v0.2.0 Usage: tileclaw [-h] [-c filename] [-test-mail]\n\n")
		flag.PrintDefaults()
		os.Exit(0)
		return nil
	})
	flag.StringVar(&cf, "c", config.DefaultConfigFile, "set config `file`")
	flag.BoolVar(&testMail, "test-mail", false, "send a test email and exit")
	flag.StringVar(&convertShpInput, "convert-shp", "", "convert .shp or .zip containing shapefile to GeoJSON and exit")
	flag.StringVar(&convertShpOut, "convert-out", "", "set converted GeoJSON output `file`")
	flag.BoolVar(&convertForcePoly, "force-polygon", false, "force PolyLine to Polygon (auto-close open rings)")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, `TileClaw v0.2.0 Usage: tileclaw [-h] [-c filename]`)
		flag.PrintDefaults()
		os.Exit(0)
	}
}

func handlePanic() {
	if r := recover(); r != nil {
		log.SysLog.Errorf("program panic: %v\nstack:\n%s", r, debug.Stack())
		mail.NotifyPanic(fmt.Sprintf("%v", r))
		os.Exit(1)
	}
}

// test mail
func handleTestMail() {
	if testMail {
		mail.SendMail("[TileClaw] Test Email", "This is a test email from TileClaw.\n\nIf you see this, your mail config is working correctly.\n\n-- TileClaw")
		log.SysLog.Println("test email sent, check your inbox")
		os.Exit(0)
	}
}

// convert shapefile
func handleConvertShp() {
	if convertShpInput != "" {
		output, err := converter.ConvertShapefileToGeoJSON(convertShpInput, convertShpOut, convertForcePoly)
		if err != nil {
			log.SysLog.Fatalf("convert shapefile failed: %s", err)
		}
		log.SysLog.Printf("converted shapefile to geojson: %s", output)
		os.Exit(0)
	}
}
