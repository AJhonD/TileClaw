package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"tileclaw/internal/log"
	"tileclaw/internal/repository"
	"tileclaw/internal/tile"
	"tileclaw/internal/util"

	"github.com/paulmach/orb/maptile"
)

const mbtilesBatchSize = 200

type TileStore interface {
	Exists(maptile.Tile) (bool, error)
	Save(tile.Tile) error
	Close() error
}

type FileStore struct {
	rootDir string
	format  string
}

func NewFileStore(rootDir, format string) (*FileStore, error) {
	if err := os.MkdirAll(rootDir, os.ModePerm); err != nil {
		return nil, err
	}
	return &FileStore{
		rootDir: rootDir,
		format:  format,
	}, nil
}

func (store *FileStore) Exists(t maptile.Tile) (bool, error) {
	_, err := os.Stat(util.TileFilePath(store.rootDir, store.format, t))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (store *FileStore) Save(t tile.Tile) error {
	return util.SaveTileFile(t, store.rootDir, store.format)
}

func (store *FileStore) Close() error {
	return nil
}

type MBTilesStore struct {
	repo *repository.MBTilesRepository
	ch   chan tile.Tile
	wg   sync.WaitGroup
}

func NewMBTilesStore(path string, metadata map[string]string, minZoom, maxZoom, pipeSize int) (*MBTilesStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return nil, err
	}
	repo, err := repository.OpenMBTiles(path, metadata, minZoom, maxZoom)
	if err != nil {
		return nil, err
	}

	store := &MBTilesStore{
		repo: repo,
		ch:   make(chan tile.Tile, pipeSize),
	}
	store.wg.Add(1)
	go store.writeLoop()
	return store, nil
}

func DefaultMBTilesPath(outputDir, name string) string {
	return filepath.Join(outputDir, fmt.Sprintf("%s.mbtiles", name))
}

func (store *MBTilesStore) Exists(t maptile.Tile) (bool, error) {
	return store.repo.TileExists(t)
}

func (store *MBTilesStore) Save(t tile.Tile) error {
	store.ch <- t
	return nil
}

func (store *MBTilesStore) Close() error {
	close(store.ch)
	store.wg.Wait()
	return store.repo.Close()
}

func (store *MBTilesStore) writeLoop() {
	defer store.wg.Done()
	batch := make([]tile.Tile, 0, mbtilesBatchSize)
	for t := range store.ch {
		batch = append(batch, t)
		if len(batch) >= mbtilesBatchSize {
			store.saveBatch(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		store.saveBatch(batch)
	}
}

func (store *MBTilesStore) saveBatch(batch []tile.Tile) {
	if err := store.repo.SaveBatch(batch); err != nil {
		log.SysLog.Errorf("batch save mbtiles error: %s", err)
	}
}
