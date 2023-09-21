package mariner2

import (
	"github.com/azure/dalec/frontend"
	"github.com/moby/buildkit/frontend/subrequests/targets"
)

const (
	targetKey = "mariner2"
)

func RegisterTargets() {
	frontend.RegisterTarget(targetKey, targets.Target{
		Name:        "toolkitroot",
		Description: "Outputs an rpm buildroot suitable for passing to the mariner2 build toolkit.",
	}, handleToolkitRoot)

	frontend.RegisterTarget(targetKey, targets.Target{
		Name:        "rpm",
		Description: "Builds an rpm and src.rpm for mariner2.",
	}, handleRPM)

	frontend.RegisterTarget(targetKey, targets.Target{
		Name:        "container",
		Description: "Builds a container with the RPM installed.",
		Default:     true,
	}, handleContainer)
}
