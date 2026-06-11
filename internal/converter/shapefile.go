package converter

import (
	"archive/zip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"tileclaw/internal/log"
)

type featureCollection struct {
	Type     string    `json:"type"`
	Name     string    `json:"name,omitempty"`
	Features []feature `json:"features"`
}

type feature struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Geometry   geometry               `json:"geometry"`
}

type geometry struct {
	Type        string      `json:"type"`
	Coordinates interface{} `json:"coordinates"`
}

func ConvertShapefileToGeoJSON(inputPath, outputPath string, forcePolygon bool) (string, error) {
	shp, name, err := readShapefile(inputPath)
	if err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = defaultOutputPath(name)
	}

	features, err := parseShapefile(shp, forcePolygon)
	if err != nil {
		return "", err
	}

	data := featureCollection{
		Type:     "FeatureCollection",
		Name:     strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath)),
		Features: features,
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
		return "", err
	}
	out, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(outputPath, out, 0644); err != nil {
		return "", err
	}
	return outputPath, nil
}

func readShapefile(inputPath string) ([]byte, string, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".shp" {
		data, err := os.ReadFile(inputPath)
		return data, filepath.Base(inputPath), err
	}
	if ext != ".zip" {
		return nil, "", fmt.Errorf("unsupported input type %q, expected .shp or .zip", ext)
	}

	reader, err := zip.OpenReader(inputPath)
	if err != nil {
		return nil, "", err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if strings.EqualFold(filepath.Ext(file.Name), ".shp") {
			rc, err := file.Open()
			if err != nil {
				return nil, "", err
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			return data, filepath.Base(file.Name), err
		}
	}
	return nil, "", fmt.Errorf("zip does not contain a .shp file")
}

func parseShapefile(data []byte, forcePolygon bool) ([]feature, error) {
	if len(data) < 100 {
		return nil, fmt.Errorf("invalid shapefile: file too small")
	}

	typeCount := map[int]int{}
	var features []feature
	offset := 100
	for offset+8 <= len(data) {
		recordNumber := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		contentWords := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8

		contentLen := contentWords * 2
		if offset+contentLen > len(data) {
			return nil, fmt.Errorf("invalid shapefile: record %d exceeds file size", recordNumber)
		}
		content := data[offset : offset+contentLen]
		offset += contentLen
		if len(content) < 4 {
			continue
		}

		shapeType := int(binary.LittleEndian.Uint32(content[0:4]))
		typeCount[shapeType]++

		geom, ok, err := parseShape(content, forcePolygon)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", recordNumber, err)
		}
		if !ok {
			continue
		}

		features = append(features, feature{
			Type: "Feature",
			Properties: map[string]interface{}{
				"record": recordNumber,
			},
			Geometry: geom,
		})
	}

	logTypeSummary(typeCount)
	return features, nil
}

func logTypeSummary(typeCount map[int]int) {
	if len(typeCount) == 0 {
		return
	}
	parts := make([]string, 0, len(typeCount))
	for t, c := range typeCount {
		parts = append(parts, fmt.Sprintf("%s x%d", shapeTypeName(t), c))
	}
	log.SysLog.Infof("Shapefile geometry: %s", strings.Join(parts, ", "))
}

func shapeTypeName(t int) string {
	switch t {
	case 0:
		return "Null"
	case 1:
		return "Point"
	case 3:
		return "PolyLine"
	case 5:
		return "Polygon"
	case 11:
		return "PointZ"
	case 13:
		return "PolyLineZ"
	case 15:
		return "PolygonZ"
	case 21:
		return "PointM"
	case 23:
		return "PolyLineM"
	case 25:
		return "PolygonM"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

func parseShape(content []byte, forcePolygon bool) (geometry, bool, error) {
	shapeType := int(binary.LittleEndian.Uint32(content[0:4]))
	switch shapeType {
	case 0:
		return geometry{}, false, nil
	case 1, 11, 21:
		return parsePoint(content)
	case 3, 13, 23:
		return parseParts(content, false, forcePolygon)
	case 5, 15, 25:
		return parseParts(content, true, forcePolygon)
	default:
		return geometry{}, false, fmt.Errorf("unsupported shape type %d", shapeType)
	}
}

func parsePoint(content []byte) (geometry, bool, error) {
	if len(content) < 20 {
		return geometry{}, false, fmt.Errorf("point shape is too short")
	}
	x := float64FromLittleEndian(content[4:12])
	y := float64FromLittleEndian(content[12:20])
	return geometry{Type: "Point", Coordinates: []float64{x, y}}, true, nil
}

func parseParts(content []byte, polygon bool, forcePolygon bool) (geometry, bool, error) {
	if len(content) < 44 {
		return geometry{}, false, fmt.Errorf("part shape is too short")
	}

	numParts := int(binary.LittleEndian.Uint32(content[36:40]))
	numPoints := int(binary.LittleEndian.Uint32(content[40:44]))
	partsStart := 44
	pointsStart := partsStart + numParts*4
	pointsEnd := pointsStart + numPoints*16
	if numParts <= 0 || numPoints <= 0 || pointsEnd > len(content) {
		return geometry{}, false, fmt.Errorf("invalid part or point count")
	}

	parts := make([]int, numParts+1)
	for i := 0; i < numParts; i++ {
		parts[i] = int(binary.LittleEndian.Uint32(content[partsStart+i*4 : partsStart+i*4+4]))
	}
	parts[numParts] = numPoints

	points := make([][]float64, numPoints)
	pos := pointsStart
	for i := 0; i < numPoints; i++ {
		points[i] = []float64{
			float64FromLittleEndian(content[pos : pos+8]),
			float64FromLittleEndian(content[pos+8 : pos+16]),
		}
		pos += 16
	}

	lines := make([][][]float64, 0, numParts)
	for i := 0; i < numParts; i++ {
		start, end := parts[i], parts[i+1]
		if start < 0 || end > len(points) || start >= end {
			return geometry{}, false, fmt.Errorf("invalid part range")
		}
		lines = append(lines, points[start:end])
	}

	if polygon {
		if len(lines) == 1 {
			return geometry{Type: "Polygon", Coordinates: lines}, true, nil
		}
		polygons := make([][][][]float64, 0, len(lines))
		for _, line := range lines {
			polygons = append(polygons, [][][]float64{line})
		}
		return geometry{Type: "MultiPolygon", Coordinates: polygons}, true, nil
	}

	if allClosed(lines) {
		log.SysLog.Infof("PolyLine has closed rings, converting to Polygon")
		if len(lines) == 1 {
			return geometry{Type: "Polygon", Coordinates: lines}, true, nil
		}
		polygons := make([][][][]float64, 0, len(lines))
		for _, line := range lines {
			polygons = append(polygons, [][][]float64{line})
		}
		return geometry{Type: "MultiPolygon", Coordinates: polygons}, true, nil
	}

	if forcePolygon {
		log.SysLog.Infof("force converting PolyLine to Polygon (auto-closing %d ring(s))", len(lines))
		closed := closeRings(lines)
		if len(closed) == 1 {
			return geometry{Type: "Polygon", Coordinates: closed}, true, nil
		}
		polygons := make([][][][]float64, 0, len(closed))
		for _, ring := range closed {
			polygons = append(polygons, [][][]float64{ring})
		}
		return geometry{Type: "MultiPolygon", Coordinates: polygons}, true, nil
	}

	if len(lines) == 1 {
		return geometry{Type: "LineString", Coordinates: lines[0]}, true, nil
	}
	return geometry{Type: "MultiLineString", Coordinates: lines}, true, nil
}

func closeRings(lines [][][]float64) [][][]float64 {
	closed := make([][][]float64, 0, len(lines))
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		first, last := line[0], line[len(line)-1]
		if delta(first[0], last[0]) > 1e-9 || delta(first[1], last[1]) > 1e-9 {
			line = append(line, append([]float64(nil), first...))
		}
		closed = append(closed, line)
	}
	return closed
}

func allClosed(lines [][][]float64) bool {
	for _, line := range lines {
		if len(line) < 2 {
			return false
		}
		first, last := line[0], line[len(line)-1]
		if delta(first[0], last[0]) > 1e-9 || delta(first[1], last[1]) > 1e-9 {
			return false
		}
	}
	return true
}

func delta(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

func defaultOutputPath(inputName string) string {
	name := strings.TrimSuffix(filepath.Base(inputName), filepath.Ext(inputName))
	if name == "" {
		name = "converted"
	}
	return filepath.Join("geojson", name+".geojson")
}

func float64FromLittleEndian(data []byte) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(data))
}
