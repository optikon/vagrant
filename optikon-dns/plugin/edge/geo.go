package edge

import "math"

const (
	earthRaidusKm = 6371
	radianScalar  = math.Pi / 180.0
)

// Point represents a point on the planet.
type Point struct {
	Lon float64 `json:"lon"`
	Lat float64 `json:"lat"`
}

// NewPoint returns a new Point for the given lon, lat coordinates.
func NewPoint(lon, lat float64) Point {
	return Point{
		Lon: lon,
		Lat: lat,
	}
}

// GreatCircleDistance calculates the shortest path between two coordinates
// on the surface of the Earth, using the Haversine formula
// (https://en.wikipedia.org/wiki/Haversine_formula).
func (p1 Point) GreatCircleDistance(p2 Point) float64 {
	dLat := (p2.Lat - p1.Lat) * radianScalar
	dLon := (p2.Lon - p1.Lon) * radianScalar

	lat1 := p1.Lat * radianScalar
	lat2 := p2.Lat * radianScalar

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)

	a := sinDLat*sinDLat + sinDLon*sinDLon*math.Cos(lat1)*math.Cos(lat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRaidusKm * c
}
