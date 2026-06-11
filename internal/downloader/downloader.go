package downloader

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"tileclaw/internal/log"
	"tileclaw/internal/tile"
	"time"

	"github.com/paulmach/orb/maptile"
)

var (
	numRangeRe    = regexp.MustCompile(`\{(\d+)-(\d+)\}`)
	letterRangeRe = regexp.MustCompile(`\{([a-z])-([a-z])\}`)
)

type Fetcher struct {
	client    *http.Client
	format    string
	timeDelay int
}

type Result struct {
	Tile     tile.Tile
	URL      string
	Size     int
	Duration time.Duration
}

func New(format string, timeDelay int) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		format:    format,
		timeDelay: timeDelay,
	}
}

func (f *Fetcher) Fetch(mt maptile.Tile, templateURL string) (Result, error) {
	start := time.Now()
	tileURL := prepareTileURL(mt, templateURL)
	req, err := http.NewRequest(http.MethodGet, tileURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://map.tianditu.gov.cn")

	if f.timeDelay > 0 {
		time.Sleep(time.Duration(f.timeDelay) * time.Millisecond)
	}

	body, err := f.fetchBody(req, tileURL, mt)
	if err != nil {
		return Result{}, err
	}

	td := tile.Tile{T: mt, C: body}
	if f.format == tile.PBF {
		td.C = gzipBytes(body)
	}

	return Result{
		Tile:     td,
		URL:      tileURL,
		Size:     len(body),
		Duration: time.Since(start),
	}, nil
}

func (f *Fetcher) fetchBody(req *http.Request, tileURL string, mt maptile.Tile) ([]byte, error) {
	var body []byte
	var statusCode int
	minBodySize := 256
	if f.format == tile.PBF || f.format == tile.PNG {
		minBodySize = 1
	}

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := f.client.Do(req)
		if err != nil {
			log.SysLog.Warnf("fetch %s attempt %d error: %s", tileURL, attempt+1, err)
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}
		statusCode = resp.StatusCode
		if resp.StatusCode == http.StatusOK {
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.SysLog.Warnf("read %s attempt %d error: %s", tileURL, attempt+1, err)
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			if len(body) < minBodySize {
				log.SysLog.Warnf("tile %v too small (%d bytes), retry", mt, len(body))
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			break
		}
		resp.Body.Close()
		if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusTooManyRequests {
			log.SysLog.Warnf("fetch %s status %d attempt %d", tileURL, resp.StatusCode, attempt+1)
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}
		break
	}

	if statusCode != http.StatusOK || len(body) < minBodySize {
		if statusCode == http.StatusOK && len(body) < minBodySize {
			return nil, fmt.Errorf("tile %v too small after retries (%d bytes)", mt, len(body))
		}
		return nil, fmt.Errorf("fetch %v tile error, status code: %d", mt, statusCode)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("nil tile %v", mt)
	}
	return body, nil
}

func gzipBytes(body []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		log.SysLog.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.SysLog.Fatal(err)
	}
	return buf.Bytes()
}

func prepareTileURL(t maptile.Tile, url string) string {
	url = strings.Replace(url, "{x}", strconv.Itoa(int(t.X)), -1)
	url = strings.Replace(url, "{y}", strconv.Itoa(int(t.Y)), -1)
	maxY := int(math.Pow(2, float64(t.Z))) - 1
	url = strings.Replace(url, "{-y}", strconv.Itoa(maxY-int(t.Y)), -1)
	url = strings.Replace(url, "{z}", strconv.Itoa(int(t.Z)), -1)
	url = numRangeRe.ReplaceAllStringFunc(url, func(m string) string {
		parts := numRangeRe.FindStringSubmatch(m)
		if len(parts) == 3 {
			lo, _ := strconv.Atoi(parts[1])
			hi, _ := strconv.Atoi(parts[2])
			return strconv.Itoa(lo + rand.IntN(hi-lo+1))
		}
		return m
	})
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
