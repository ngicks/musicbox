package service

import (
	"slices"

	"github.com/compose-spec/compose-go/v2/types"
)

// Reverse changes dst so that its enabled services are disabled in src.
func Reverse(src *types.Project) (dst *types.Project, err error) {
	serviceNames := src.ServiceNames()

	var disabledServices []string
	for _, disabled := range src.DisabledServices {
		if !slices.Contains(serviceNames, disabled.Name) {
			// In case caller is not correctly set up src.
			disabledServices = append(disabledServices, disabled.Name)
		}
	}

	if len(disabledServices) > 0 {
		dst = EnableAll(src)
		dst, err = dst.WithSelectedServices(disabledServices, types.IgnoreDependencies)
		if err != nil {
			return nil, err
		}
	} else {
		dst = src.WithServicesDisabled(src.ServiceNames()...)
	}

	return dst, nil
}

// EnableAll adds DisabledServices to Services and set empty Services to DisabledServices.
func EnableAll(p *types.Project) *types.Project {
	cloned, _ := p.WithProfiles([]string{"*"})
	for k, v := range cloned.DisabledServices {
		cloned.Services[k] = v
	}
	cloned.DisabledServices = make(types.Services)
	return cloned
}
