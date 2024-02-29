package service

import (
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

// CompareProjectImage compares 2 projects and return image names which only exists in old, new respectively.
//
// If any of image is not tagged, it will resolve by adding `:latest`.
func CompareProjectImage(old, new *types.Project) (onlyInOld, addedInNew []string) {
OLD_SERVICE:
	for _, oldService := range old.AllServices() {
		for _, newService := range new.AllServices() {
			if fallbackLatest(newService.Image) == fallbackLatest(oldService.Image) {
				continue OLD_SERVICE
			}
		}
		onlyInOld = append(onlyInOld, fallbackLatest(oldService.Image))
	}
NEW_SERVICE:
	for _, newService := range new.AllServices() {
		for _, oldService := range old.AllServices() {
			if fallbackLatest(oldService.Image) == fallbackLatest(newService.Image) {
				continue NEW_SERVICE
			}
		}
		addedInNew = append(addedInNew, fallbackLatest(newService.Name))
	}
	return onlyInOld, addedInNew
}

// fallbackLatest fills latest tag if i has no tag specified.
func fallbackLatest(i string) string {
	if strings.Contains(i, ":") {
		return i
	}
	return i + ":latest"
}
