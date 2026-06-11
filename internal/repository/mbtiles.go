package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"tileclaw/internal/tile"
	"time"

	"github.com/paulmach/orb/maptile"
	_ "modernc.org/sqlite"
)

type MBTilesRepository struct {
	db      *sql.DB
	writeMu sync.Mutex
}

func OpenMBTiles(path string, metadata map[string]string, minZoom, maxZoom int) (*MBTilesRepository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	repo := &MBTilesRepository{db: db}
	if err := repo.optimizeConnection(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.setupSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.SaveMetadata(metadata, minZoom, maxZoom); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (repo *MBTilesRepository) Save(tile tile.Tile) error {
	_, err := repo.db.Exec(
		"insert or ignore into tiles (zoom_level, tile_column, tile_row, tile_data) values (?, ?, ?, ?)",
		tile.T.Z,
		tile.T.X,
		tile.FlipY(),
		tile.C,
	)
	return err
}

func (repo *MBTilesRepository) SaveBatch(tiles []tile.Tile) error {
	repo.writeMu.Lock()
	defer repo.writeMu.Unlock()

	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = repo.saveBatchLocked(tiles)
		if err == nil {
			return nil
		}
		if isBusy(err) {
			time.Sleep(time.Duration(50<<attempt) * time.Millisecond)
			continue
		}
		return err
	}
	return fmt.Errorf("batch commit retry exhausted: %w", err)
}

func (repo *MBTilesRepository) saveBatchLocked(tiles []tile.Tile) error {
	tx, err := repo.db.Begin()
	if err != nil {
		return fmt.Errorf("begin batch tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("insert or ignore into tiles (zoom_level, tile_column, tile_row, tile_data) values (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare batch insert: %w", err)
	}
	defer stmt.Close()

	for _, tile := range tiles {
		if _, err := stmt.Exec(tile.T.Z, tile.T.X, tile.FlipY(), tile.C); err != nil {
			return fmt.Errorf("save tile %v: %w", tile.T, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch tx: %w", err)
	}
	return nil
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "database is locked") || strings.Contains(s, "SQLITE_BUSY")
}

func (repo *MBTilesRepository) TileExists(t maptile.Tile) (bool, error) {
	var count int
	err := repo.db.QueryRow(
		"SELECT COUNT(*) FROM tiles WHERE zoom_level=? AND tile_column=? AND tile_row=?",
		t.Z,
		t.X,
		flipY(t),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (repo *MBTilesRepository) SaveMetadata(items map[string]string, minZoom, maxZoom int) error {
	var oldMin, oldMax string
	_ = repo.db.QueryRow("SELECT value FROM metadata WHERE name='minzoom'").Scan(&oldMin)
	_ = repo.db.QueryRow("SELECT value FROM metadata WHERE name='maxzoom'").Scan(&oldMax)

	if v, err := strconv.Atoi(oldMin); err == nil && v < minZoom {
		items["minzoom"] = oldMin
	}
	if v, err := strconv.Atoi(oldMax); err == nil && v > maxZoom {
		items["maxzoom"] = oldMax
	}

	for name, value := range items {
		_, err := repo.db.Exec("insert or replace into metadata (name, value) values (?, ?)", name, value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (repo *MBTilesRepository) Optimize() error {
	if _, err := repo.db.Exec("ANALYZE;"); err != nil {
		return err
	}
	_, err := repo.db.Exec("VACUUM;")
	return err
}

func (repo *MBTilesRepository) Close() error {
	if _, err := repo.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = repo.db.Close()
		return err
	}
	return repo.db.Close()
}

func (repo *MBTilesRepository) optimizeConnection() error {
	if _, err := repo.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	_, err := repo.db.Exec("PRAGMA busy_timeout=5000")
	return err
}

func (repo *MBTilesRepository) setupSchema() error {
	statements := []string{
		"create table if not exists tiles (zoom_level integer, tile_column integer, tile_row integer, tile_data blob);",
		"create table if not exists metadata (name text, value text);",
		"create unique index if not exists name on metadata (name);",
		"create unique index if not exists tile_index on tiles(zoom_level, tile_column, tile_row);",
	}
	for _, statement := range statements {
		if _, err := repo.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func flipY(t maptile.Tile) uint32 {
	return uint32(1<<t.Z) - 1 - t.Y
}
