// NOTE: Adapted from https://gist.github.com/jgrahamc/9807839

package edge

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/miekg/dns"
)

// The domain name provided in the LOC record.
var edgeDomain = fmt.Sprintf("%s.site.", pluginName)

var (
	// Regex expressions for parsing LOC string.
	//
	// The string l will be in the following format:
	// d1 [m1 [s1]] {"N"|"S"} d2 [m2 [s2]] {"E"|"W"}
	// alt["m"] [siz["m"] [hp["m"] [vp["m"]]]]
	//
	// d1 is the latitude, d2 is the longitude, alt is the altitude,
	// siz is the size of the planet, hp and vp are the horiz and vert
	// precisions. See RFC 1876 for full detail.
	//
	// Examples:
	// 42 21 54 N 71 06 18 W -24m 30m
	// 42 21 43.952 N 71 5 6.344 W -24m 1m 200m
	// 52 14 05 N 00 08 50 E 10m
	// 2 7 19 S 116 2 25 E 10m
	// 42 21 28.764 N 71 00 51.617 W -44m 2000m
	// 59 N 10 E 15.0 30.0 2000.0 5.0
	lonLatRe      = "(\\d+)(?: (\\d+))?(?: (\\d+(?:\\.\\d+)?))?"
	otherValRe    = "(?: (-?\\d+(?:\\.\\d+)?)m?)"
	optOtherValRe = fmt.Sprintf("%s?", otherValRe)
	locReStr      = fmt.Sprintf("%s (N|S) %s (E|W) %s %s %s %s", lonLatRe, lonLatRe, otherValRe, optOtherValRe, optOtherValRe, optOtherValRe)
	locRe         = regexp.MustCompile(locReStr)
)

// Parses and removes the LOC record from the Extra fields of a DNS message.
func extractLocationRecord(r *dns.Msg) (*Point, bool) {

	// Assert that such a record actually exists.
	if len(r.Extra) == 0 {
		return nil, false
	}

	// Try to extract the last entry.
	point, err := convertLOCToPoint(r.Extra[len(r.Extra)-1])
	if err != nil {
		return nil, false
	}

	// Remove the LOC record from the back of Extra.
	r.Extra = r.Extra[:(len(r.Extra) - 1)]

	// Return the parse point.
	return point, true
}

// Inserts a LOC record into a DNS request under the Extra fields.
func insertLocationRecord(r *dns.Msg, locRR dns.RR) {

	// Add the LOC record to the *end* of the Extra fields.
	// NOTE: This is where we'll look for it when we extract it.
	r.Extra = append(r.Extra, locRR)
}

// Takes a geographic point and converts it to a DNS LOC RR record.
func convertPointToLOC(point *Point) (dns.RR, error) {

	// Start by populating a LOC struct.
	loc := new(dns.LOC)
	loc.Longitude = uint32(int(float64(dns.LOC_DEGREES)*point.Lon) + dns.LOC_PRIMEMERIDIAN)
	loc.Latitude = uint32(int(float64(dns.LOC_DEGREES)*point.Lat) + dns.LOC_EQUATOR)

	// Converts the LOC to a basic RR.
	rr, err := dns.NewRR(loc.String())
	if err != nil {
		return nil, err
	}
	loc.Header().Name = edgeDomain
	loc.Header().Class = dns.ClassINET
	loc.Header().Rrtype = dns.TypeLOC
	loc.Header().Ttl = 0
	loc.Header().Rdlength = uint16(len(loc.String()))

	return rr, nil
}

// Takes a DNS LOC record and converts it to a geographic point.
func convertLOCToPoint(loc dns.RR) (*Point, error) {

	// Assert that the RR is a LOC record.
	if loc.Header().Rrtype != dns.TypeLOC || loc.Header().Name != edgeDomain {
		return nil, errInvalidLOC
	}

	// Parse out the lon-lat in decimal degrees.
	lon, lat, err := parseLOCString(loc.String())
	if err != nil {
		return nil, err
	}

	return NewPoint(lon, lat), nil
}

// Takes a LOC string and converts it to the struct.
func parseLOCString(l string) (float64, float64, error) {

	// Use regex to parse out the parts of the string.
	parts := locRe.FindStringSubmatch(l)
	if parts == nil {
		return 0.0, 0.0, errInvalidLOC
	}

	// Quick reference for the matches:
	// parts[1] == latitude degrees
	// parts[2] == latitude minutes (optional)
	// parts[3] == latitude seconds (optional)
	// parts[4] == N or S
	// parts[5] == longitude degrees
	// parts[6] == longitude minutes (optional)
	// parts[7] == longitude seconds (optional)
	// parts[8] == E or W
	// parts[9] == altitude (NOTE: ignore this)
	// These are completely optional:
	// parts[10] == size
	// parts[11] == horizontal precision
	// parts[12] == vertical precision

	// Parse the latitude d, m, s values into decimal degrees.
	lat, ok := dmsToDD(parts[1], parts[2], parts[3], 90)
	if !ok {
		return 0.0, 0.0, errInvalidLOC
	}
	if parts[4] == "S" {
		lat = -lat
	}

	// Parse the longitude d, m, s values into decimal degrees.
	lon, ok := dmsToDD(parts[5], parts[6], parts[7], 180)
	if !ok {
		return 0.0, 0.0, errInvalidLOC
	}
	if parts[8] == "W" {
		lon = -lon
	}

	return lon, lat, nil
}

// Converts DMS (degree, minute, second) format to decimal degrees.
func dmsToDD(d, m, s string, limit uint64) (float64, bool) {

	// The resulting decimal degree will be a float.
	var result float64

	// Parse the degree value.
	val, err := strconv.ParseUint(d, 10, 8)
	if err != nil || val > limit {
		return result, false
	}
	result = float64(val)

	// Parse the minute value.
	if m != "" {
		val, err = strconv.ParseUint(m, 10, 8)
		if err != nil || val > 59 {
			return result, false
		}
		result += float64(val) / 60.0
	}

	// Parse the second value.
	if s != "" {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f > 59.999 {
			return result, false
		}
		result += f / 3600.0
	}

	return result, true
}
