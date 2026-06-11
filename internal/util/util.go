package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"tileclaw/internal/log"
	"tileclaw/internal/tile"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
)

func SaveTileFile(tile tile.Tile, rootDir, format string) error {
	dir := filepath.Join(rootDir, fmt.Sprintf(`%d`, tile.T.Z), fmt.Sprintf(`%d`, tile.T.X))
	os.MkdirAll(dir, os.ModePerm)
	fileName := filepath.Join(dir, fmt.Sprintf(`%d.%s`, tile.T.Y, format))
	err := os.WriteFile(fileName, tile.C, os.ModePerm)
	if err != nil {
		return err
	}
	log.ProgLog.Println(fileName)
	return nil
}

func loadFeature(path string) *geojson.Feature {
	data, err := os.ReadFile(path)
	if err != nil {
		log.SysLog.Fatalf("unable to read file: %v", err)
	}

	f, err := geojson.UnmarshalFeature(data)
	if err == nil {
		return f
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err == nil {
		if len(fc.Features) != 1 {
			log.SysLog.Fatalf("must have 1 feature: %v", len(fc.Features))
		}
		return fc.Features[0]
	}

	g, err := geojson.UnmarshalGeometry(data)
	if err != nil {
		log.SysLog.Fatalf("unable to unmarshal feature: %v", err)
	}

	return geojson.NewFeature(g.Geometry())
}

func loadFeatureCollection(path string) *geojson.FeatureCollection {
	data, err := os.ReadFile(path)
	if err != nil {
		log.SysLog.Fatalf("unable to read file: %v", err)
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		log.SysLog.Fatalf("unable to unmarshal feature: %v", err)
	}

	count := 0
	for i := range fc.Features {
		if fc.Features[i].Properties["name"] != "original" {
			fc.Features[count] = fc.Features[i]
			count++
		}
	}
	fc.Features = fc.Features[:count]

	return fc
}

var collectionCache sync.Map

func LoadCollection(path string) orb.Collection {
	if v, ok := collectionCache.Load(path); ok {
		return v.(orb.Collection)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.SysLog.Fatalf("unable to read file: %v", err)
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		log.SysLog.Fatalf("unable to unmarshal feature: %v", err)
	}

	var collection orb.Collection
	for _, f := range fc.Features {
		collection = append(collection, f.Geometry)
	}

	collectionCache.Store(path, collection)
	return collection
}

// output gets called if there is a test failure for debugging.
func output(name string, r *geojson.FeatureCollection) {
	f := loadFeature("./data/" + name + ".geojson")
	if f.Properties == nil {
		f.Properties = make(geojson.Properties)
	}

	f.Properties["fill"] = "#FF0000"
	f.Properties["fill-opacity"] = "0.5"
	f.Properties["stroke"] = "#FF0000"
	f.Properties["name"] = "original"
	r.Append(f)

	data, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		log.SysLog.Fatalf("error marshalling json: %v", err)
	}

	err = os.WriteFile("failure_"+name+".geojson", data, 0644)
	if err != nil {
		log.SysLog.Fatalf("write file failure: %v", err)
	}
}

// output gets called if there is a test failure for debugging.
func output2(name string, r *geojson.FeatureCollection, wg *sync.WaitGroup) {
	defer wg.Done()
	data, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		log.SysLog.Fatalf("error marshalling json: %v", err)
	}

	err = os.WriteFile(name+".geojson", data, 0644)
	if err != nil {
		log.SysLog.Fatalf("write file failure: %v", err)
	}
}

func getZoomCount(g orb.Geometry, minz int, maxz int) map[int]int64 {

	info := make(map[int]int64)
	for z := minz; z <= maxz; z++ {
		info[z] = tilecover.GeometryCount(g, maptile.Zoom(z))
	}
	return info
}

func TileFilePath(rootDir, format string, t maptile.Tile) string {
	dir := filepath.Join(rootDir, fmt.Sprintf(`%d`, t.Z), fmt.Sprintf(`%d`, t.X))
	fileName := fmt.Sprintf(`%d.%s`, t.Y, format)
	return filepath.Join(dir, fileName)
}
