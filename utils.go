package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
)

func saveToMBTile(tile Tile, db *sql.DB) error {
	_, err := db.Exec("insert into tiles (zoom_level, tile_column, tile_row, tile_data) values (?, ?, ?, ?);", tile.T.Z, tile.T.X, tile.flipY(), tile.C)
	if err != nil {
		return err
	}
	return nil
}

func saveBatchToMBTile(tiles []Tile, db *sql.DB) {
	tx, err := db.Begin()
	if err != nil {
		sysLog.Errorf("batch tx begin error: %s", err)
		return
	}
	stmt, err := tx.Prepare("insert or ignore into tiles (zoom_level, tile_column, tile_row, tile_data) values (?, ?, ?, ?)")
	if err != nil {
		sysLog.Errorf("batch prepare error: %s", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()
	for _, tile := range tiles {
		_, err := stmt.Exec(tile.T.Z, tile.T.X, tile.flipY(), tile.C)
		if err != nil {
			sysLog.Warnf("batch save %v tile error: %s", tile.T, err)
		}
	}
	err = tx.Commit()
	if err != nil {
		sysLog.Errorf("batch commit error: %s", err)
		tx.Rollback()
	}
}

func saveToFiles(tile Tile, task *Task) error {
	dir := filepath.Join(task.File, fmt.Sprintf(`%d`, tile.T.Z), fmt.Sprintf(`%d`, tile.T.X))
	os.MkdirAll(dir, os.ModePerm)
	fileName := filepath.Join(dir, fmt.Sprintf(`%d.%s`, tile.T.Y, task.TileMap.Format))
	err := os.WriteFile(fileName, tile.C, os.ModePerm)
	if err != nil {
		return err
	}
	progLog.Println(fileName)
	return nil
}

func optimizeConnection(db *sql.DB) error {
	_, err := db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		return err
	}
	_, err = db.Exec("PRAGMA busy_timeout=5000")
	if err != nil {
		return err
	}
	return nil
}

func optimizeDatabase(db *sql.DB) error {
	_, err := db.Exec("ANALYZE;")
	if err != nil {
		return err
	}

	_, err = db.Exec("VACUUM;")
	if err != nil {
		return err
	}

	return nil
}

func loadFeature(path string) *geojson.Feature {
	data, err := os.ReadFile(path)
	if err != nil {
		sysLog.Fatalf("unable to read file: %v", err)
	}

	f, err := geojson.UnmarshalFeature(data)
	if err == nil {
		return f
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err == nil {
		if len(fc.Features) != 1 {
			sysLog.Fatalf("must have 1 feature: %v", len(fc.Features))
		}
		return fc.Features[0]
	}

	g, err := geojson.UnmarshalGeometry(data)
	if err != nil {
		sysLog.Fatalf("unable to unmarshal feature: %v", err)
	}

	return geojson.NewFeature(g.Geometry())
}

func loadFeatureCollection(path string) *geojson.FeatureCollection {
	data, err := os.ReadFile(path)
	if err != nil {
		sysLog.Fatalf("unable to read file: %v", err)
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		sysLog.Fatalf("unable to unmarshal feature: %v", err)
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

func loadCollection(path string) orb.Collection {
	if v, ok := collectionCache.Load(path); ok {
		return v.(orb.Collection)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		sysLog.Fatalf("unable to read file: %v", err)
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		sysLog.Fatalf("unable to unmarshal feature: %v", err)
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
		sysLog.Fatalf("error marshalling json: %v", err)
	}

	err = os.WriteFile("failure_"+name+".geojson", data, 0644)
	if err != nil {
		sysLog.Fatalf("write file failure: %v", err)
	}
}

// output gets called if there is a test failure for debugging.
func output2(name string, r *geojson.FeatureCollection, wg *sync.WaitGroup) {
	defer wg.Done()
	data, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		sysLog.Fatalf("error marshalling json: %v", err)
	}

	err = os.WriteFile(name+".geojson", data, 0644)
	if err != nil {
		sysLog.Fatalf("write file failure: %v", err)
	}
}

func getZoomCount(g orb.Geometry, minz int, maxz int) map[int]int64 {

	info := make(map[int]int64)
	for z := minz; z <= maxz; z++ {
		info[z] = tilecover.GeometryCount(g, maptile.Zoom(z))
	}
	return info
}

func getTileFilePath(task *Task, t maptile.Tile) string {
	dir := filepath.Join(task.File, fmt.Sprintf(`%d`, t.Z), fmt.Sprintf(`%d`, t.X))
	fileName := fmt.Sprintf(`%d.%s`, t.Y, task.TileMap.Format)
	return filepath.Join(dir, fileName)
}
