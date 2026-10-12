// Package buildinfo owns the application version, independently of Wails/UI.
package buildinfo

import (
	"embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

//go:embed VERSION
var source embed.FS

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Version [3]uint16

// Parse accepts stable SemVer within Windows executable version bounds.
// Prereleases/build metadata are not part of the v1 stable update channel.
func Parse(value string) (Version, error) {
	var result Version
	match := stableVersion.FindStringSubmatch(value)
	if match == nil {
		return result, fmt.Errorf("Некорректная версия приложения")
	}
	for i := range result {
		n, err := strconv.ParseUint(match[i+1], 10, 16)
		if err != nil {
			return Version{}, fmt.Errorf("Версия превышает Windows version bounds")
		}
		result[i] = uint16(n)
	}
	return result, nil
}
func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }
func (v Version) Compare(other Version) int {
	for i := range v {
		if v[i] < other[i] {
			return -1
		}
		if v[i] > other[i] {
			return 1
		}
	}
	return 0
}
func Current() string { b, _ := source.ReadFile("VERSION"); return strings.TrimSpace(string(b)) }
