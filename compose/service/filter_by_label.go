package service

import "github.com/compose-spec/compose-go/v2/types"

// FindServicesByLabels selects services in p by given labels.
func FindServicesByLabels(p *types.Project, labels []string) []types.ServiceConfig {
	var matched []types.ServiceConfig
SERVICES:
	for _, s := range p.AllServices() {
		for _, label := range labels {
			_, ok := s.Labels[label]
			if !ok {
				continue SERVICES
			}
		}
		matched = append(matched, s)
	}
	return matched
}
